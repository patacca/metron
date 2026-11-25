#!/bin/sh
# The unrestricted arm: one agent, one prompt, no governance. It reviews,
# fixes and commits until it is satisfied, then says whether the code is
# ready in VERDICT.json.
#
#   eval/run/unrestricted.sh <case> <trial-dir>
#
# EVAL_MODEL, if set, is passed to omp as --model.
set -eu
. "$(dirname -- "$0")/lib.sh"

setup "$@"
mkdir "$TRIAL/rounds"
cp "$CASE_DIR/contract.yaml" "$REPO/ACCEPTANCE.yaml"
printf 'ACCEPTANCE.yaml\nVERDICT.json\n' >>"$REPO/.git/info/exclude"

p=$(step_prompt)
cp "$EVAL_RUN/prompts/unrestricted.md" "$p"
run_omp unrestricted agent "$p" 2700

if ready=$(record verdict "$REPO/VERDICT.json" "$TRIAL/rounds/1.review.json"); then
  if [ "$ready" = ready ]; then end=approved; else end=not-ready; fi
else
  end=role-failure
fi
record baseline "$TRIAL" "$ARM" "$end" >"$TRIAL/baseline.json"
finish "$end"
