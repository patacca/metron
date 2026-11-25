# Evaluation

Tooling to evaluate Metron empirically with real agents: synthetic cases with
planted defects and ground truth (`cases/`), the glue that runs one trial of
one arm (`run/`), and a scorer that reads what the trials recorded
(`score/`). The role agents run under an existing harness, Oh My Pi (`omp`);
nothing here is an agent harness.

## Running a trial

```
eval/run/metron.sh       <case> <results-root>/<case>/metron/<trial>
eval/run/unrestricted.sh <case> <results-root>/<case>/unrestricted/<trial>
eval/run/bounded.sh      <case> <results-root>/<case>/bounded/<trial>
```

Each script needs `git`, `python3`, `omp` and, for the Metron arm, `go`. The
trial directory must not exist yet. While the arm runs, the repository lives
in a fresh temporary directory, so that nothing the agents see points to the
trial directory and the other roles' transcripts in it; it is moved to
`<trial>/repo` when the arm ends. `EVAL_MODEL`, if set, is passed to every
omp run as `--model`.

| Arm | What runs | End states |
| --- | --- | --- |
| `metron` | The loop of `docs/architecture.md`, section 12: whatever `metron next` names, until the step is `none` or governance owns it. Never `metron resolve` or a pin after setup | `approved`, `governance:<kinds or step>`, `role-failure`, `driver-cap` |
| `unrestricted` | One agent, one prompt: review, fix, commit, decide | `approved`, `not-ready`, `role-failure` |
| `bounded` | Reviewer and fixer, fresh agents each step, at most 3 rounds | `approved`, `cap`, `role-failure` |

A variant case holds a one-line `base` file naming the case whose `repo/`
and `oracle/` it shares; its own `contract.yaml` and `truth.json` apply.

## What a trial records

| File | Content |
| --- | --- |
| `run.json` | Arm, timing, end state and every step: omp runs with model and tokens (input counts cached prompt tokens too), and for the Metron arm each `metron check` as a step of kind `check` |
| `oracle.json` | The fixture's tests and each hidden oracle test on the final commit |
| `diff.json` | Lines and files changed from the seed to the final commit, without `.metron/` |
| `ledger.jsonl`, `packets/`, `reports/` | The Metron arm's state |
| `baseline.json` | The baselines' verdicts and reviews, normalized |
| `steps/` | Each step's prompt, omp events and errors |
| `repo/` | The fixture repository as the arm left it |

The final commit is what an arm delivers: uncommitted changes are not judged.

## Scoring

```
go run ./eval/score [-cases eval/cases] [-json file] <results-root>
```

It prints a Markdown report and makes no model calls. An unsafe GREEN, an
approval while a blocker plant's oracle tests do not all pass, is the one hard
acceptance criterion; a bounded loop that ships at its cap is reported apart.
