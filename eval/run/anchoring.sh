#!/bin/sh
# The anchoring experiment on the listings case: does a reviewer who is shown
# the ledger miss a defect that the ledger says is resolved? Two conditions
# review the same tree, on which P1 is back and P2 is fixed, with one reviewer
# step each, run as the Metron arm runs a review step.
#
#   eval/run/anchoring.sh ledger|fresh <trial-dir>
#
# The history is scripted, with no agent: pin the contract; pass the checks;
# a RED review with P1 (T0) and P2 (T1); the reference fix as one commit,
# claimed for both; the checks; a verification confirming both; then an
# ordinary commit that brings P1 back as a refactoring would, and touches the
# README. ledger reviews that state: its packet shows P1's finding resolved,
# with the verifier's evidence, and names the verified candidate, which is
# not the reviewed one. fresh reviews a new repository whose single commit has
# the same tree, under a new ledger with the same contract.
#
# EVAL_MODEL and METRON_BIN as for eval/run/metron.sh. The trial records the
# end state reviewed:<verdict>, or role-failure, under arm anchoring-<condition>.
set -eu
. "$(dirname -- "$0")/lib.sh"

[ $# -eq 2 ] || die "usage: $0 ledger|fresh <trial-dir>"
case $1 in
ledger | fresh) ARM=anchoring-$1 ;;
*) die "the condition is ledger or fresh, not $1" ;;
esac
setup listings "$2"
use_metron

helper() { python3 "$EVAL_RUN/anchoring.py" "$@"; }

expect_next() {
  ask_next
  [ "$(field step)" = "$1" ] || die "metron next names $(field step), not $1: see $TRIAL/next.json"
}

# scripted <role> <fill> [commit]: answer the open packet with a scripted report.
scripted() {
  metron packet --format skeleton -o .metron/work/report.json >>"$LOG" 2>&1
  helper "$2" .metron/work/report.json ${3:+"$3"}
  metron ingest .metron/work/report.json >>"$LOG" 2>&1 || die "metron refused the scripted $1 report: see $LOG"
  rm .metron/work/report.json
}

# build_history: the scripted campaign up to the review under test, in REPO
# under METRON_STATE_DIR.
build_history() {
  adopt_metron
  cd "$REPO"
  expect_next run-checks
  metron check >>"$LOG"
  expect_next review
  scripted reviewer review
  expect_next fix
  metron packet >>"$LOG" 2>&1
  git apply "$BASE_DIR/oracle/reference_fix.patch"
  git add -A
  git commit -q -m "fix: update known listings in place and read prices exactly"
  scripted implementer fix "$(git rev-parse HEAD)"
  expect_next run-checks
  metron check >>"$LOG"
  expect_next verify
  scripted verifier verify
  helper regress "$REPO"
  git add -A
  git commit -q -m "refactor(store): write new and known listings with one statement"
  mkdir "$TRIAL/selfcheck"
  git archive HEAD | tar -x -C "$TRIAL/selfcheck"
  python3 "$EVAL_RUN/oracle.py" "$BASE_DIR" "$TRIAL/selfcheck" | helper selfcheck
  rm -rf "$TRIAL/selfcheck"
  expect_next run-checks
  metron check >>"$LOG"
  expect_next review
}

if [ "$1" = ledger ]; then
  build_history
else
  # The history is built in the trial directory, out of the reviewer's
  # sight, and the reviewed repository starts from its final tree.
  mkdir "$TRIAL/history"
  mv "$REPO" "$TRIAL/history/repo"
  REPO=$TRIAL/history/repo
  METRON_STATE_DIR=$TRIAL/history/state
  build_history
  built=$(metron status --json | helper digest)
  REPO=$WORK/repo
  METRON_STATE_DIR=$WORK/state
  mkdir "$REPO"
  git -C "$TRIAL/history/repo" archive HEAD | tar -x -C "$REPO"
  cd "$REPO"
  git init -q
  git config user.name eval
  git config user.email eval@example.invalid
  git add -A
  git commit -q -m seed
  metron init >>"$LOG"
  metron contract pin -m "evaluation" >>"$LOG"
  [ "$(metron status --json | helper digest)" = "$built" ] || die "the fresh repository's candidate is not the one the history built"
  expect_next run-checks
  metron check >>"$LOG"
  expect_next review
fi

FAILED=0
run_role reviewer review
verdict=$(helper verdict "$METRON_STATE_DIR/ledger.jsonl")
end=role-failure
if [ "$FAILED" -eq 0 ] && [ -n "$verdict" ]; then end=reviewed:$verdict; fi

keep_state
if [ "$1" = fresh ]; then
  # The seed, for diff.json; the reviewer has finished.
  git fetch -q "$TRIAL/history/repo" HEAD
  rm -rf "$TRIAL/history"
fi
finish "$end"
