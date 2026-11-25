#!/bin/sh
# The Metron arm: the loop of docs/architecture.md, section 12, as an operator
# runs it. Do what `metron next` names; stop when the step is `none` or the
# owner is governance. Governance decisions are never automated here: after
# setup this script runs neither `metron resolve` nor `metron contract pin`.
#
#   eval/run/metron.sh <case> <trial-dir>
#
# EVAL_MODEL, if set, is passed to omp as --model. METRON_BIN, if set, is the
# metron binary to use. The trial ends at the driver's cap after 60 steps of
# the loop or EVAL_WALL_SECONDS (default 5400) of wall clock.
set -eu
. "$(dirname -- "$0")/lib.sh"

setup "$@"
use_metron
DEADLINE=$((STARTED_S + ${EVAL_WALL_SECONDS:-5400}))
adopt_metron
cd "$REPO"

# run_commit: the implementer makes the working tree clean as next describes.
run_commit() {
  p=$(step_prompt)
  cat "$EVAL_RUN/prompts/metron-commit.md" >"$p"
  { echo '```'; metron next; echo '```'; } >>"$p"
  run_omp commit-changes implementer "$p" 1800
}

# run_check_fix <check-output>: a check failed; the implementer, who owns
# run-checks, corrects the candidate.
run_check_fix() {
  p=$(step_prompt)
  cat "$EVAL_RUN/prompts/metron-checks.md" >"$p"
  { echo '```'; cat "$1"; echo '```'; echo; echo 'This is what `metron next` printed:'; echo
    echo '```'; metron next; echo '```'; } >>"$p"
  run_omp run-checks implementer "$p" 1800
}

end=driver-cap
FAILED=0
i=0
while [ "$i" -lt 60 ] && [ "$(date +%s)" -lt "$DEADLINE" ]; do
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
    if [ "$rc" -eq 1 ]; then run_check_fix "$STEPS/$nn.check.txt"; fi
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

keep_state
finish "$end"
