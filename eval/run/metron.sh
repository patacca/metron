#!/bin/sh
# The Metron arm: the loop of docs/architecture.md, section 12, as an operator
# runs it. Do what `metron next` names; stop when the step is `none` or the
# owner is governance. Governance decisions are never automated here: after
# setup this script runs neither `metron resolve` nor `metron contract pin`.
#
#   eval/run/metron.sh <case> <trial-dir>
#
# EVAL_MODEL, if set, is passed to omp as --model.
set -eu
. "$(dirname -- "$0")/lib.sh"

setup "$@"
mkdir -p "$TRIAL/bin"
(cd "$METRON_SRC" && CGO_ENABLED=0 go build -trimpath -o "$TRIAL/bin/metron" ./cmd/metron)
PATH=$TRIAL/bin:$PATH
export PATH
LOG=$TRIAL/metron.log

# The model the roles declare in their reports: EVAL_MODEL, or the one omp
# answers with by default.
MODEL=${EVAL_MODEL:-}
if [ -z "$MODEL" ]; then
  printf 'Reply with the single word OK.\n' >"$STEPS/probe.md"
  omp -p --no-session --mode json --max-time 5m --no-context-files --no-skills --no-rules --no-extensions \
    --cwd "$STEPS" "@$STEPS/probe.md" </dev/null >"$STEPS/probe.jsonl" 2>"$STEPS/probe.err" || die "omp failed: see $STEPS/probe.err"
  MODEL=$(record model <"$STEPS/probe.jsonl")
  [ -n "$MODEL" ] || die "omp reported no model: see $STEPS/probe.jsonl"
fi

cd "$REPO"
metron init >>"$LOG"
cp "$CASE_DIR/contract.yaml" .metron/contract.yaml
git add -A
git commit -q -m "chore: adopt metron"
metron contract pin -m "evaluation" >>"$LOG"

ask_next() { metron next --json >"$TRIAL/next.json" || die "metron next failed"; }
field() { record get "$1" <"$TRIAL/next.json"; }

# run_role <role> <step>: give the open packet's brief to a fresh omp process;
# retry while that packet stays open, at most twice. Sets FAILED on giving up.
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
    run_omp "$2" "$1" "$p" 30m
    ask_next
    if [ "$(field step)" != submit-report ] || [ "$(field packet)" != "$packet" ]; then
      return 0
    fi
    tries=$((tries + 1))
    if [ "$tries" -gt 2 ]; then
      FAILED=1
      return 0
    fi
  done
}

# run_commit: the implementer makes the working tree clean as next describes.
run_commit() {
  p=$(step_prompt)
  cat "$EVAL_RUN/prompts/metron-commit.md" >"$p"
  { echo '```'; metron next; echo '```'; } >>"$p"
  run_omp commit-changes implementer "$p" 30m
}

end=driver-cap
FAILED=0
i=0
while [ "$i" -lt 60 ]; do
  i=$((i + 1))
  ask_next
  step=$(field step)
  owner=$(field owner)
  if [ "$step" = none ]; then
    end=approved
    break
  fi
  if [ "$owner" = governance ]; then
    end=governance:$step
    if [ "$step" = adjudicate ]; then
      metron status --json >"$TRIAL/status.json" || die "metron status failed"
      kinds=$(record governance-kinds <"$TRIAL/status.json")
      end=governance:${kinds:-adjudicate}
    fi
    break
  fi
  case $step in
  run-checks)
    begin_step
    rc=0
    metron check >"$STEPS/$nn.check.txt" 2>&1 || rc=$?
    end_step check operator "$rc"
    [ "$rc" -le 1 ] || die "metron check exited $rc: see $STEPS/$nn.check.txt"
    ;;
  review | fix | verify | audit | submit-report)
    run_role "$owner" "$step"
    if [ "$FAILED" -eq 1 ]; then
      end=role-failure
      break
    fi
    ;;
  commit-changes) run_commit ;;
  *) die "metron next names an unknown step: $step" ;;
  esac
done

finish "$end"
cp .metron/state/ledger.jsonl "$TRIAL/ledger.jsonl"
for d in reports packets; do
  if [ -d ".metron/state/$d" ]; then cp -R ".metron/state/$d" "$TRIAL/$d"; fi
done
