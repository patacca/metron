# Shared by the arm scripts in eval/run/. Source it; do not run it.
#
# It does what an operator would otherwise type by hand: set up a fixture
# repository, start one fresh omp process per step, and record the trial when
# the arm ends. Everything that reads or writes JSON goes through record.py.

EVAL_RUN=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
EVAL_DIR=$(dirname -- "$EVAL_RUN")
METRON_SRC=$(dirname -- "$EVAL_DIR")
ARM=$(basename -- "$0" .sh)

# The agents never see the operator's own git identity or settings.
GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_GLOBAL

die() {
  printf '%s: %s\n' "$ARM" "$*" >&2
  exit 1
}

now() { date -u +%Y-%m-%dT%H:%M:%SZ; }

record() { python3 "$EVAL_RUN/record.py" "$@"; }

# setup <case> <trial-dir>: a new fixture repository with one commit, "seed",
# made under the identity eval <eval@example.invalid>.
#
# While the arm runs, the repository, the prompt the agent reads and the
# metron binary live in a fresh temporary directory, WORK, so that nothing
# an agent can see (its working directory, the prompt file's name, PATH)
# leads to the trial directory and the transcripts of the other roles.
# finish moves the repository to <trial-dir>/repo.
#
# CASE_DIR holds the case's contract.yaml and truth.json. A variant names
# another case in a one-line file, base; BASE_DIR is then that case, whose
# repo/ and oracle/ the variant shares. Otherwise BASE_DIR is CASE_DIR.
setup() {
  [ $# -eq 2 ] || die "usage: $0 <case> <trial-dir>"
  CASE=$1
  CASE_DIR=$EVAL_DIR/cases/$1
  [ -f "$CASE_DIR/contract.yaml" ] || die "no case $1: $CASE_DIR/contract.yaml does not exist"
  BASE_DIR=$CASE_DIR
  if [ -f "$CASE_DIR/base" ]; then
    BASE_DIR=$EVAL_DIR/cases/$(tr -d ' \t\r\n' <"$CASE_DIR/base")
  fi
  [ -d "$BASE_DIR/repo" ] || die "case $1 has no seed: $BASE_DIR/repo does not exist"
  [ ! -e "$2" ] || die "$2 already exists: give every trial a new directory"
  mkdir -p "$2/steps"
  TRIAL=$(CDPATH='' cd -- "$2" && pwd)
  STEPS=$TRIAL/steps
  WORK=$(mktemp -d)
  REPO=$WORK/repo
  N=0
  STARTED=$(now)
  STARTED_S=$(date +%s)
  HARNESS=$(omp --version | tr / ' ')
  cp -R "$BASE_DIR/repo" "$REPO"
  git -C "$REPO" init -q
  git -C "$REPO" config user.name eval
  git -C "$REPO" config user.email eval@example.invalid
  git -C "$REPO" add -A
  git -C "$REPO" commit -q -m seed
  SEED=$(git -C "$REPO" rev-parse HEAD)
}

# step_prompt: the prompt file of the next step.
step_prompt() { printf '%s/%02d.prompt.md' "$STEPS" $((N + 1)); }

# begin_step and end_step <kind> <role> <exit-status> number a step and record
# its timing in steps.tsv. Its files are steps/$nn.*.
begin_step() {
  N=$((N + 1))
  nn=$(printf '%02d' "$N")
  t0=$(date +%s)
}
end_step() {
  printf '%s\t%s\t%s\t%s\t%s\n' "$N" "$1" "$2" $(($(date +%s) - t0)) "$3" >>"$TRIAL/steps.tsv"
}

# run_omp <kind> <role> <prompt-file> <max-seconds>: one fresh omp process in
# the repository, its events in steps/NN.jsonl. It reads the prompt from a
# copy in WORK. With DEADLINE set (seconds since the epoch), the run also
# stops then.
run_omp() {
  begin_step
  cp "$3" "$WORK/prompt.md"
  limit=$4
  if [ -n "${DEADLINE:-}" ] && [ $((DEADLINE - t0)) -lt "$limit" ]; then
    limit=$((DEADLINE - t0 > 1 ? DEADLINE - t0 : 1))
  fi
  rc=0
  omp -p --no-session --mode json --max-time "$limit" \
    --no-context-files --no-skills --no-rules --no-extensions \
    ${EVAL_MODEL:+--model "$EVAL_MODEL"} \
    --cwd "$REPO" "@$WORK/prompt.md" </dev/null >"$STEPS/$nn.jsonl" 2>"$STEPS/$nn.err" || rc=$?
  end_step "$1" "$2" "$rc"
}

# use_metron: put metron on PATH, METRON_BIN if set, else built from this
# source tree; keep its state directory in WORK, outside the repository,
# where a role's git commands cannot roll it back (docs/architecture.md,
# section 4); and set MODEL, the model the roles declare in their reports:
# EVAL_MODEL, or the one omp answers with by default.
use_metron() {
  mkdir "$WORK/bin"
  if [ -n "${METRON_BIN:-}" ]; then
    cp "$METRON_BIN" "$WORK/bin/metron"
  else
    (cd "$METRON_SRC" && CGO_ENABLED=0 go build -trimpath -o "$WORK/bin/metron" ./cmd/metron)
  fi
  PATH=$WORK/bin:$PATH
  METRON_STATE_DIR=$WORK/state
  export PATH METRON_STATE_DIR
  LOG=$TRIAL/metron.log
  MODEL=${EVAL_MODEL:-}
  if [ -z "$MODEL" ]; then
    printf 'Reply with the single word OK.\n' >"$WORK/prompt.md"
    omp -p --no-session --mode json --max-time 5m --no-context-files --no-skills --no-rules --no-extensions \
      --cwd "$WORK" "@$WORK/prompt.md" </dev/null >"$STEPS/probe.jsonl" 2>"$STEPS/probe.err" || die "omp failed: see $STEPS/probe.err"
    MODEL=$(record model <"$STEPS/probe.jsonl")
    [ -n "$MODEL" ] || die "omp reported no model: see $STEPS/probe.jsonl"
  fi
}

# adopt_metron: in the repository, initialize metron, commit the case's
# contract and pin it, as governance does once before the loop.
adopt_metron() {
  (
    cd "$REPO"
    metron init >>"$LOG"
    cp "$CASE_DIR/contract.yaml" .metron/contract.yaml
    git add -A
    git commit -q -m "chore: adopt metron"
    metron contract pin -m "evaluation" >>"$LOG"
  )
}

# keep_state: copy the ledger, packets and reports to the trial directory.
keep_state() {
  cp "$METRON_STATE_DIR/ledger.jsonl" "$TRIAL/ledger.jsonl"
  for d in reports packets; do
    if [ -d "$METRON_STATE_DIR/$d" ]; then cp -R "$METRON_STATE_DIR/$d" "$TRIAL/$d"; fi
  done
}

ask_next() { metron next --json >"$TRIAL/next.json" || die "metron next failed"; }
field() { record get "$1" <"$TRIAL/next.json"; }

# run_role <role> <step>: in the repository, give the open packet's brief to
# a fresh omp process; retry while that packet stays open, at most twice, and
# not after DEADLINE. Sets FAILED on giving up.
run_role() {
  tries=0
  packet=
  while :; do
    metron packet --format prompt -o .metron/work/prompt.md >>"$LOG" 2>&1 || die "metron packet failed: see $LOG"
    ask_next
    [ -n "$packet" ] || packet=$(field packet)
    [ -n "$packet" ] || die "no packet is open after metron packet: see $TRIAL/next.json"
    p=$(step_prompt)
    sed -e "s|{{ROLE}}|$1|g" -e "s|{{MODEL}}|$MODEL|g" -e "s|{{HARNESS}}|$HARNESS|g" "$EVAL_RUN/prompts/metron-role.md" >"$p"
    cat .metron/work/prompt.md >>"$p"
    run_omp "$2" "$1" "$p" 1800
    ask_next
    if [ "$(field step)" != submit-report ] || [ "$(field packet)" != "$packet" ]; then
      return 0
    fi
    tries=$((tries + 1))
    if [ "$tries" -gt 2 ]; then
      FAILED=1
      return 0
    fi
    [ "$(date +%s)" -lt "${DEADLINE:-$((t0 + 86400))}" ] || return 0
  done
}

# finish <end-state>: move the repository to the trial directory and record
# the trial. diff.json and oracle.json describe the last commit, which is
# what the arm delivers; uncommitted changes are not part of it.
finish() {
  ended=$(now)
  seconds=$(($(date +%s) - STARTED_S))
  cd "$TRIAL"
  mv "$REPO" "$TRIAL/repo"
  rm -rf "$WORK"
  REPO=$TRIAL/repo
  head=$(git -C "$REPO" rev-parse HEAD)
  git -C "$REPO" diff --numstat -z --no-renames "$SEED" "$head" -- . ':(exclude).metron' >"$TRIAL/numstat"
  record diff "$SEED" "$head" <"$TRIAL/numstat" >"$TRIAL/diff.json"
  rm -f "$TRIAL/numstat"
  git -C "$REPO" archive -o "$TRIAL/final.tar" "$head"
  mkdir "$TRIAL/final"
  tar -x -f "$TRIAL/final.tar" -C "$TRIAL/final"
  python3 "$EVAL_RUN/oracle.py" "$BASE_DIR" "$TRIAL/final" >"$TRIAL/oracle.json"
  rm -rf "$TRIAL/final" "$TRIAL/final.tar"
  touch "$TRIAL/steps.tsv"
  record run "$TRIAL" "$CASE" "$ARM" "$(basename -- "$TRIAL")" "$STARTED" "$ended" "$seconds" "$1" "$HARNESS" >"$TRIAL/run.json"
  printf '%s: %s %s ended %s after %ss and %s steps\n' "$ARM" "$CASE" "$TRIAL" "$1" "$seconds" "$N" >&2
}
