#!/bin/sh
# The bounded arm: a conventional review-fix loop. A reviewer writes
# REVIEW.json, a different fixer addresses it and commits, at most 3 rounds.
# It ends approved when a reviewer approves, and at the cap when round 3 ends
# with changes requested: a conventional loop ships then.
#
#   eval/run/bounded.sh <case> <trial-dir>
#
# EVAL_MODEL, if set, is passed to omp as --model.
set -eu
. "$(dirname -- "$0")/lib.sh"

setup "$@"
mkdir "$TRIAL/rounds"
cp "$CASE_DIR/contract.yaml" "$REPO/ACCEPTANCE.yaml"
printf 'ACCEPTANCE.yaml\nREVIEW.json\n' >>"$REPO/.git/info/exclude"

end=cap
round=0
while [ "$round" -lt 3 ]; do
  round=$((round + 1))
  rm -f "$REPO/REVIEW.json"
  p=$(step_prompt)
  cp "$EVAL_RUN/prompts/bounded-reviewer.md" "$p"
  run_omp review reviewer "$p" 30m
  if ! verdict=$(record review "$REPO/REVIEW.json" "$TRIAL/rounds/$round.review.json"); then
    end=role-failure
    break
  fi
  if [ "$verdict" = approve ]; then
    end=approved
    break
  fi
  before=$(git -C "$REPO" rev-parse HEAD)
  p=$(step_prompt)
  cp "$EVAL_RUN/prompts/bounded-fixer.md" "$p"
  run_omp fix fixer "$p" 30m
  if [ "$(git -C "$REPO" rev-parse HEAD)" = "$before" ]; then fixed=false; else fixed=true; fi
  echo "$fixed" >"$TRIAL/rounds/$round.fixed"
done

record baseline "$TRIAL" "$ARM" "$end" >"$TRIAL/baseline.json"
finish "$end"
