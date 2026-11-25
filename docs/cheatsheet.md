# Cheat sheet

Metron on one page. New to it? Start with the [quick start](quickstart.md).

## The loop

```
metron next          # what to do now, and who does it
  ...do it...
metron next          # repeat until:  Next step: none, the gate is open
metron gate          # exit 0 = this commit is approved for this stage
```

```
review ──RED──▶ fix ──▶ verify ──confirmed──▶ review (new commit) ──GREEN──▶ gate
                 ▲         │
                 └─rejected┘
```

## Commands

**Set up**

| Command | What it does |
| --- | --- |
| `metron init` | Create `.metron/` with a commented contract template |
| `metron contract` | List what is missing or wrong in the contract |
| `metron contract pin -m "<reason>"` | Authorize the contract. Needed again after any change of meaning |

**Every day**

| Command | What it does |
| --- | --- |
| `metron next` | The single next step, its owner and its command |
| `metron check` | Run the contract's check commands on `HEAD` and record the results |
| `metron packet --format prompt` | The brief for whoever acts next, to read or to give to an agent |
| `metron packet --format skeleton -o .metron/work/report.json` | The report to fill in |
| `metron packet` | The packet as JSON |
| `metron ingest <file>` | Validate and record a report. `-` reads standard input |
| `metron gate` | Evaluate the gate. Exit 0 only when it is open |
| `metron resolve <E-id> -m "<decision>"` | Governance closes an escalation it owns |

**Look around**

| Command | What it shows |
| --- | --- |
| `metron status` | Contract, commit, rounds, findings, checks, gate, next step |
| `metron findings` | Unresolved findings. `--backlog`: the non-blocking ones. `--all`: every finding |
| `metron log` | Every recorded event, in order |
| `metron fsck` | Whether the ledger and the stored files are intact |
| `metron protocol` | The rules every role works under |
| `metron schema [name]` | The JSON Schemas: `contract`, `packet`, `report`, `event`, `defs` |
| `metron <command> -h` | Help for one command |

`next`, `gate`, `status`, `findings`, `check`, `ingest`, `contract` and
`version` accept `--json`. The state directory is `--state-dir <dir>` after
the command name, else `$METRON_STATE_DIR`, else `.metron/state`.

## What `metron next` can say

The first row that applies is the one you get.

| Step | Owner | It means | You do |
| --- | --- | --- | --- |
| `repair-ledger` | governance | The ledger is damaged or missing | `metron fsck` says what is wrong and the ways out; usually, restore `.metron/state/` from git |
| `pin-contract` | governance | The contract is incomplete or changed | `metron contract`, fix, `metron contract pin -m "<reason>"` |
| `submit-report` | the packet's role | A packet is waiting for its report | `metron ingest <file>` |
| `adjudicate` | governance | A decision is owed on an escalation | Decide, then `metron resolve <E-id> -m "<decision>"` |
| `commit-changes` | implementer | Files outside `.metron/` are modified or untracked | Commit or discard them |
| `audit` | auditor | The loop is not converging | `metron packet`, audit, `metron ingest` |
| `verify` | verifier | A fix is claimed | `metron packet`, verify, `metron ingest` |
| `fix` | implementer | A blocking finding is open | `metron packet`, fix and commit, `metron ingest` |
| `run-checks` | implementer | A check has no passing result on this commit | `metron check` |
| `review` | reviewer | This commit has no `GREEN` verdict | `metron packet`, review, `metron ingest` |
| `none` | | The gate is open | Ship what the contract authorizes |

## Roles

| Role | Writes code | May also be |
| --- | --- | --- |
| governance | | anyone; identified by git `user.name` and `user.email` |
| implementer | yes | nobody else |
| reviewer | no | verifier |
| verifier | no | reviewer |
| auditor | no | nobody else |

Separation is decided on `author.id` in each report.

## Reports

Every report is one JSON document. The skeleton fills in the first four
members for you.

```json
{
  "schema": "metron.report/v1",
  "packet": "P-0001",
  "packet_digest": "sha256:…",
  "role": "reviewer",
  "author": { "id": "alice", "kind": "human" },
  "body": { }
}
```

`author.id` is 1 to 128 printable ASCII characters without spaces. `kind` is
`human` or `agent`; an agent also gives `model`, and may give `harness` and
`session`.

| Role | The body contains |
| --- | --- |
| reviewer | `coverage`: all 18 domains, each `assessed` or `not-applicable` with a `summary`<br>`requirements`: each one `satisfied`, `violated` or `unverified`, with `evidence`<br>`findings`: the new findings<br>`verdict` and `justification`<br>Optional: `reopen`, `contract_gaps`, `residual_risks`, `limitations` |
| implementer | `fixes`: each with `finding`, `commits` (full hashes), `summary`, `components`; optional `root_cause`, `tests`<br>Optional: `disputes` (each with `finding`, `rationale`), `notes` |
| verifier | `verifications`: each with `finding`, `result` (`confirmed` or `rejected`), `evidence`<br>Optional: `regressions`, `notes` |
| auditor | `resolutions`: each with `escalation`, `analysis`, `conclusion`<br>`adjudications`, when the packet lists findings to adjudicate: each with `finding`, `decision` (`uphold`, `withdraw` or `reclassify`), `rationale`, and `tier` for `reclassify`<br>Optional: `directives`, `contract_recommendations` |

**The verdict is derived, not chosen:**

| The report | Verdict |
| --- | --- |
| opens or reopens a `T0` or `T1` finding | `RED` |
| otherwise, has an `unverified` requirement | `INDETERMINATE` |
| otherwise | `GREEN` |

A requirement marked `violated` needs a blocking finding that cites it, and a
requirement cited by a blocking finding is `violated`.

## Findings

| Tier | Meaning | Blocks | Besides the common members, it needs |
| --- | --- | --- | --- |
| `T0` | Critical | yes | `requirement` or `risk`, `blocking_rationale`, `verification` |
| `T1` | Material deficiency | yes | the same |
| `T2` | Improvement | no | `expected_benefit`, `deferral_rationale` |
| `T3` | Calibration: needs evidence nobody has yet | no | `evidence_needed`, `reassess_when` |
| `T4` | Preference | no | nothing |

Common members: `key`, `tier`, `domain`, `title`, `components`, `description`,
`evidence`, `impact`.

A `T0` or `T1` finding in `optimization`, `performance`, `scalability`,
`architecture`, `maintainability`, `technical-debt`, `simplicity` or
`cost-efficiency` also needs `proportionality`: `proposed_change`,
`regression_risk`, `deferral_cost`.

A finding goes `open` → `claimed` (fix reported) → `resolved` (fix
confirmed). A rejected fix makes it `open` again. An auditor can make it
`withdrawn`. A reviewer can reopen a resolved one with a reason:
`new-evidence`, `regression`, `invalid-verification`, `requirement-changed`
or `contract-conflict`.

## Contract

| Member | What to write |
| --- | --- |
| `project` | `name`, `objectives`, optional `non_objectives` |
| `stage` | The stage a `GREEN` gate lets you enter |
| `authorizes` | The concrete activity the gate permits |
| `scope` | What is under review |
| `operating_envelope` | The conditions and load it must handle. Outside it, nothing blocks |
| `requirements` | `id` (`REQ-1`), `domain`, `statement`, `criterion`; optional `command`, `from` |
| `risks.unacceptable` | `id` (`RISK-1`), `statement`. A blocking finding may cite these |
| `risks.accepted` | `id` (`ACC-1`), `statement`. A finding about one never blocks |
| `not_applicable` | `domain`, `rationale`: a domain you waive |
| `calibrations` | `id` (`CAL-1`), `parameter`, `evidence_needed`, `reassess_when` |
| `convergence` | Optional thresholds: `root_cause_after` 2, `convergence_audit_after` 3, `process_audit_after` 5 |

Each stage adds domains that need a requirement or a waiver. Earlier stages
still count.

| Stage | Domains it adds |
| --- | --- |
| `prototype` | `functional-correctness`, `objective-alignment`, `architecture` |
| `controlled-validation` | `data-integrity`, `testability`, `observability`, `stability` |
| `pilot` | `reliability`, `performance`, `compatibility`, `security-privacy` |
| `production` | `scalability`, `operational-readiness`, `maintainability` |
| `continuous-improvement` | `optimization`, `technical-debt`, `simplicity`, `cost-efficiency` |

## Escalations

An open escalation keeps the gate closed. None of them lowers the bar.

| Kind | Raised when | Closed by |
| --- | --- | --- |
| `evidence-gap` | A review ends `INDETERMINATE` | governance: `metron resolve` |
| `contract-gap` | A reviewer reports a risk the contract does not cover | governance: `metron resolve` |
| `dispute` | An implementer disputes a finding | auditor's report |
| `contradiction` | A finding would reverse an earlier resolution | auditor's report |
| `component-churn` | The same component is fixed in 3 rounds of a stage | auditor's report |
| `regression-spiral` | A verification finds at least one blocking regression, and at least as many as the fixes it confirms | auditor's report |
| `non-convergence` | 3 consecutive rounds without `GREEN` | auditor's report |
| `process-audit` | Every 5th consecutive round without `GREEN` | auditor's report |

A finding that has come back twice, through a rejected fix or a reopening, is
flagged root-cause-required: its next fix must include `root_cause`.

## The gate

`G = C ∧ B ∧ E ∧ I ∧ V`

| | Open only when |
| --- | --- |
| C | every applicable requirement is `satisfied` in the latest review |
| B | no `T0` or `T1` finding is unresolved and no escalation is open |
| E | every check command passed on this exact commit |
| I | the commit, the contract and the ledger are the ones that were reviewed, and the tree is clean |
| V | the latest verdict is `GREEN`, for this commit, from someone who never implemented |

## Exit status

| Status | Meaning |
| --- | --- |
| 0 | Success. For `gate`: open |
| 1 | A negative outcome: gate closed, report rejected, contract invalid or unpinned, a check failed |
| 2 | Usage error |
| 3 | Could not evaluate: I/O error, git missing, ledger corrupt, lock held |

## Files

```
.metron/
  contract.yaml     you write this
  state/            metron writes this; commit it with your work
    ledger.jsonl    everything that happened
    packets/        P-0001.json, ...
    reports/        accepted reports, as submitted
  work/             scratch for briefs and reports; ignored by git
  .gitignore        written by init; leave both as they are
  .gitattributes
```

## Rules of thumb

- When in doubt, run `metron next`.
- What gets reviewed is a commit. Uncommitted work does not exist.
- Changing anything outside `.metron/` means a new check and a new review.
  Committing `.metron/` never does.
- Commit `.metron/` whenever you commit code and when a loop ends. Git
  history is the only backup the ledger has.
- Write reports under `.metron/work/`, never next to the code.
- A rejected report changed nothing. Fix it and submit it again.
- Editing the contract's meaning requires a new pin, which voids the open
  packet.
- Never put a check script under `.metron/`: it is not part of what is
  reviewed.
- Identities are declared, not proven. See the [trust model](trust-model.md).
