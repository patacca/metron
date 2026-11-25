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

# setup <case> <trial-dir>: a new fixture repository at <trial-dir>/repo with
# one commit, "seed", made under the identity eval <eval@example.invalid>.
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
  REPO=$TRIAL/repo
  STEPS=$TRIAL/steps
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

# run_omp <kind> <role> <prompt-file> <max-time>: one fresh omp process in the
# repository, its events in steps/NN.jsonl.
run_omp() {
  begin_step
  rc=0
  omp -p --no-session --mode json --max-time "$4" \
    --no-context-files --no-skills --no-rules --no-extensions \
    ${EVAL_MODEL:+--model "$EVAL_MODEL"} \
    --cwd "$REPO" "@$3" </dev/null >"$STEPS/$nn.jsonl" 2>"$STEPS/$nn.err" || rc=$?
  end_step "$1" "$2" "$rc"
}

# finish <end-state>: record the trial. diff.json and oracle.json describe
# the last commit, which is what the arm delivers; uncommitted changes are
# not part of it.
finish() {
  ended=$(now)
  seconds=$(($(date +%s) - STARTED_S))
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
