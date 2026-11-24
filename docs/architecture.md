# Metron architecture

Metron is a convergent engineering review framework. It decides whether a
specific commit of a repository is approved for a specific lifecycle stage, and
it keeps the review process that leads to that decision from looping forever.

This document is the normative design. Where code and this document disagree,
one of them has a bug. Trust assumptions are in [trust-model.md](trust-model.md).

## 1. Principles

| Principle | Meaning in Metron |
| --- | --- |
| Safety | The gate never opens while a mandatory requirement is violated or unverified. Every failure mode closes the gate. |
| Liveness | When the mandatory requirements are satisfied there is always a finite path to an open gate. Possible improvements never block. |
| Proportionality | A finding blocks only if it is tied to a contract requirement or an unacceptable risk. Over-engineering is itself reviewable. |
| Empiricism | Decisions cite evidence. What cannot be decided without real-world data is a calibration, not a blocker. |
| Independence | Iteration counts, effort and deadlines change the investigation procedure, never the approval threshold. |

The objective of a review is to establish that a candidate is sufficiently
correct, reliable, efficient, maintainable and safe for its authorized purpose.
It is not to establish that no further improvement is possible.

## 2. Authority

Metron is a standalone deterministic CLI. It never runs a model and never
depends on an agent harness or workflow engine.

| Party | Decides | Cannot |
| --- | --- | --- |
| Metron | The next permitted action, whether a report is acceptable, ledger state, escalations, the gate outcome | Make engineering judgments |
| Agents and humans in a role | Engineering judgments: findings, fixes, verifications, audits | Write ledger state, change the contract, open the gate |
| Orchestrators | When and how to execute the action Metron names | Choose a different action, skip one, or override an outcome |
| Governance (repository owner) | The acceptance contract and its amendments | Bypass the gate conditions |

All execution modes (manual packet exchange, a harness calling the CLI, an
external workflow engine) go through the same commands and therefore enforce
identical semantics.

## 3. Fixed vocabulary

These sets are part of the protocol and are not configurable per repository.

**Lifecycle stages**, in order: `prototype`, `controlled-validation`, `pilot`,
`production`, `continuous-improvement`.

**Tiers**

| Tier | Name | Blocks the gate | Resolution |
| --- | --- | --- | --- |
| T0 | Critical | yes | Mandatory before proceeding |
| T1 | Material deficiency | yes | Mandatory before proceeding |
| T2 | Improvement | no | Recorded with expected benefit and deferral rationale |
| T3 | Calibration | no | Recorded with the evidence needed and a reassessment trigger |
| T4 | Preference | no | Recorded; no action required |

A tier is determined by evidence and impact, never by the domain alone.

**Review domains** (18): `functional-correctness`, `objective-alignment`,
`security-privacy`, `data-integrity`, `stability`, `reliability`,
`performance`, `optimization`, `scalability`, `architecture`,
`maintainability`, `technical-debt`, `simplicity`, `testability`,
`observability`, `compatibility`, `operational-readiness`, `cost-efficiency`.

**Verdicts**: `GREEN`, `RED`, `INDETERMINATE`. Only a reviewer issues a verdict.

**Roles**: `governance`, `implementer`, `reviewer`, `verifier`, `auditor`.

**Protocol version**: `metron.protocol/v1`. The ledger, every round and every
packet record it. A binary refuses a ledger written under another protocol
version.

## 4. Files

Project configuration and operational state are separate.

```
.metron/
  contract.yaml     project configuration: the acceptance contract.
                    Authored by governance, version-controlled.
  state/            operational state. Written only by metron.
    ledger.jsonl    the authoritative event log
    packets/        issued packets, immutable, P-0001.json
    reports/        accepted reports exactly as submitted, P-0001.json
```

The state directory is `--state-dir`, else `$METRON_STATE_DIR`, else
`.metron/state`. In cooperative use, commit `state/` together with the work:
git history is the only anchor it has. In a protected environment, point the
state directory outside the repository, at a location agents cannot write.

The ledger is the single source of truth. Finding lists, the improvement
backlog, status summaries and decision history are views computed from it by
read-only commands. Nothing derived is ever stored.

Formats: YAML for what a human authors, JSON and JSON Lines for what Metron or
a role produces, JSON Schema for the public protocol, Markdown for instructions.

## 5. Candidate identity

A candidate is a commit. Its identity is the **candidate digest**: the SHA-256
of the output of `git ls-tree -r -z --full-tree <commit>` after removing every
entry whose path lies under `.metron/`. It is written `sha256:<hex>`.

Excluding `.metron/` is what lets the ledger live in the repository it governs:
committing ledger updates does not change the candidate, while any change to
any other tracked file does.

The working tree is **clean** when `git status --porcelain` reports nothing
outside `.metron/`, untracked files included. Metron requires a clean tree
whenever it binds something to a candidate.

Read-only roles are checked after the fact, because no agent protocol can
enforce them: a reviewer, verifier or auditor report is rejected if the
candidate digest or tree cleanliness changed between packet issue and report
submission.

## 6. Acceptance contract

The contract is the only project-specific input. `metron init` writes a
commented template. A contract that fails validation cannot be pinned, and
nothing can be reviewed against an unpinned contract.

| Field | Required | Content |
| --- | --- | --- |
| `schema` | yes | `metron.contract/v1` |
| `project.name`, `project.objectives` | yes | What the project is for; at least one objective |
| `project.non_objectives` | no | What it deliberately does not do |
| `stage` | yes | The lifecycle stage a GREEN gate authorizes entry to |
| `authorizes` | yes | The concrete activity a GREEN gate permits |
| `scope` | yes | Statements describing what is under review |
| `operating_envelope` | yes | Statements of expected conditions and load; findings outside it cannot block |
| `requirements` | yes | See below; at least one |
| `risks.unacceptable` | yes | `RISK-n` statements a blocking finding may cite |
| `risks.accepted` | no | Residual risks governance has accepted; never blocking |
| `not_applicable` | no | Domains waived for this stage, each with a rationale |
| `calibrations` | no | Provisional parameters, the evidence needed, and when to reassess |
| `convergence` | no | Thresholds; defaults 2, 3 and 5 |

A **requirement** has an `id` (`REQ-n`), a `domain`, a `statement` (what must
hold), a `criterion` (the acceptance boundary: how anyone decides it holds), an
optional `command` (an automated check Metron runs itself) and an optional
`from` stage (default `prototype`). A requirement is **applicable** when
`from` is not later than the contract's `stage`. This is how acceptance is made
lifecycle-aware: a requirement that only matters in production is written down
now and starts blocking when the contract targets production.

There are no gate settings. The gate conditions are fixed (section 11).

**Stage coverage.** Each stage adds domains that must be covered by at least
one applicable requirement or an explicit `not_applicable` waiver:

| Stage | Domains that become mandatory to cover |
| --- | --- |
| `prototype` | `functional-correctness`, `objective-alignment`, `architecture` |
| `controlled-validation` | `data-integrity`, `testability`, `observability`, `stability` |
| `pilot` | `reliability`, `performance`, `compatibility`, `security-privacy` |
| `production` | `scalability`, `operational-readiness`, `maintainability` |
| `continuous-improvement` | `optimization`, `technical-debt`, `simplicity`, `cost-efficiency` |

Coverage is cumulative. It constrains what the contract must address, not what
the reviewer looks at: the reviewer always assesses all 18 domains.

**Parsing.** The contract is read in three steps, each of which can reject it:

1. Strict YAML: a single document; mappings with unique string keys; no
   anchors, aliases, merge keys or custom tags; scalars limited to strings,
   integers and booleans.
2. Structural: the resulting JSON value satisfies `contract.schema.json`, which
   forbids unknown members.
3. Semantic: identifiers are unique and well-formed, text members are not
   blank, at least one requirement is applicable, stage coverage holds, a
   waived domain has no applicable requirement, and the thresholds are
   positive with the convergence threshold below the process-audit one.

**Digests.** Two digests are computed and both are recorded when pinning:

- The **raw digest** is the SHA-256 of the file's bytes. It is kept for audit.
- The **contract digest** is the SHA-256 of the canonical JSON form of the
  parsed document. It is the binding identity: comments and formatting can
  change without invalidating a review; meaning cannot.

Canonical JSON is RFC 8785 (JCS). Metron documents contain no non-integer
numbers, so its number rule reduces to plain integers.

**Pinning.** `metron contract pin -m <reason>` records the current contract
digest as authorized. If the file's contract digest differs from the pinned
one, every command that depends on the contract refuses to proceed until
governance pins it. A new pin voids the open packet and invalidates any verdict
issued under the old one. When a pin changes `stage`, round numbering and the
failed-round count restart; findings carry over.

## 7. Ledger

`ledger.jsonl` holds one event per line. State is never stored; it is the fold
of the events. The core exposes pure functions:

- `Decide(state, command, facts) -> events | error` enforces the state machine.
- `Apply(state, event) -> state` folds one event into state.

A command yields its events in a defined order, and the same state, command
and facts always yield the same events.

**Envelope.** Each line is a JSON object without insignificant whitespace,
with members in exactly this order: `seq`, `time`, `prev`, `type`, `data`,
`hash`. Who caused an event is part of its payload where that is meaningful.

- `seq` starts at 1 and increases by 1.
- `time` is RFC 3339 UTC with second precision.
- `prev` is the previous event's `hash`, or 64 zeros for the first event.
- `hash` is the lowercase hex SHA-256 of the line's bytes up to, but excluding,
  the last occurrence of `,"hash":"`, followed by a closing `}`.

Verification needs no canonicalization and can be reimplemented in any
language. Payloads never use a member named `hash`; they use `digest`.

**Writing.** All events of one command are committed together or not at all:
the store writes the complete new ledger to a temporary file in the state
directory, syncs it, and renames it over `ledger.jsonl`. A leftover temporary
file is discarded on the next run. Every command that writes holds an exclusive
operating-system lock on `<state>/lock` from before it reads the ledger until
after it commits; the lock disappears with the process, so a crash never
leaves it held.

**Event types**

| Type | Meaning |
| --- | --- |
| `ledger.initialized` | Genesis; records the protocol version |
| `contract.pinned` | Governance authorized a contract digest, with a reason |
| `evidence.recorded` | A requirement's check command ran against a candidate |
| `round.opened` | A review round started on a candidate under a contract digest |
| `packet.issued` / `packet.voided` | A role packet was issued, or became void |
| `report.accepted` | A role report passed all validation and was stored |
| `verdict.recorded` | The reviewer's verdict for a round |
| `finding.opened` | A new finding |
| `finding.fix-claimed` | An implementer claims a fix |
| `finding.fix-confirmed` / `finding.fix-rejected` | A verifier's result |
| `finding.reopened` | A resolved finding was reopened, with a reason |
| `finding.withdrawn` / `finding.reclassified` | An auditor adjudication |
| `escalation.raised` / `escalation.resolved` | A convergence safeguard fired or was discharged |
| `directive.issued` | Binding guidance from an audit, carried into later packets |

**Finding state machine**

```
            fix-claimed            fix-confirmed
  open ─────────────────▶ claimed ───────────────▶ resolved
   ▲                         │                         │
   │       fix-rejected      │                         │ reopened
   └─────────────────────────┘◀────────────────────────┘
   │
   └── withdrawn (terminal; auditor adjudication only)
```

A finding is **unresolved** while `open` or `claimed`. Its **recurrence count**
is the number of `fix-rejected` plus `reopened` events it has received.
Reclassification changes the tier and nothing else.

Reopening requires one of five reasons: `new-evidence`, `regression`,
`invalid-verification`, `requirement-changed`, `contract-conflict`.

## 8. The review loop

`metron next` returns exactly one action. It is the first row of this table
whose condition holds.

| # | Condition | Action | Owner |
| --- | --- | --- | --- |
| 1 | The ledger fails verification | `repair-ledger` | governance |
| 2 | The contract is invalid, or its digest differs from the pinned one | `pin-contract` | governance |
| 3 | A packet is open and still valid | `submit-report` | the packet's role |
| 4 | A governance-owned escalation is open | `adjudicate` | governance |
| 5 | An auditor-owned escalation is open | `audit` | auditor |
| 6 | A finding is `claimed` | `verify` | verifier |
| 7 | A blocking finding is `open` | `fix` | implementer |
| 8 | The working tree is not clean | `commit-changes` | implementer |
| 9 | A mandatory check has no passing result on the candidate | `run-checks` | implementer |
| 10 | The candidate has no GREEN verdict under the pinned contract | `review` | reviewer |
| 11 | Otherwise | `none` (the gate is open) | |

Row 9 means a review round never starts on a candidate whose own mandatory
checks fail. Row 10 is refused when the same candidate already received a
non-GREEN verdict under the same contract digest and no evidence, escalation
resolution or contract pin has been recorded since: an unchanged candidate
cannot be resubmitted in the hope of a different verdict.

A **round** is one independent, read-only, full-scope review of one candidate.
Rounds are numbered from 1 per pinned stage. Fixing and verifying happen
between rounds:

```
review ──RED──▶ fix ──▶ verify ──all confirmed──▶ review (new candidate)
   │             ▲         │
   │             └─rejected or regressions
   ├──INDETERMINATE──▶ adjudicate (governance supplies evidence or amends the contract)
   └──GREEN──▶ gate
```

At most one packet is open at a time. `metron packet` issues the packet for the
current action. Called again while that packet is open and still valid, it
returns the same packet, so a crashed agent can simply be retried. A packet
becomes void when the candidate or the pinned contract changes underneath a
read-only role, or when the contract is re-pinned.

## 9. Packets and reports

A **packet** is everything a role needs for one action: the contract, the
candidate, the relevant findings and evidence, binding directives, the
rendered instructions, and the schema its report must satisfy. A **report** is
the role's structured answer. Both are versioned JSON documents whose JSON
Schemas, published under `schemas/`, are the canonical public protocol.

A report is a versioned envelope around a role-specific body:

| Member | Content |
| --- | --- |
| `schema` | `metron.report/v1` |
| `packet` | The packet this report answers |
| `role` | The role that produced it |
| `author` | Declared identity: `id`, `kind` (`agent` or `human`), optionally `model`, `harness`, `session` |
| `body` | The role-specific content |

`metron ingest` validates a report in three separate layers and accepts it only
if all three pass. A rejected report changes nothing.

1. **Structural**: the document satisfies its JSON Schema.
2. **Semantic**: it is internally consistent and consistent with the contract.
3. **Transition**: the state machine permits it now.

Two digests are recorded for an accepted report: the raw digest of the bytes
as submitted, which is what an attestation covers in authenticated mode, and
the digest of its canonical JSON form.

**Ingestion rules, all roles**

- The report names the open packet and has that packet's role. A packet is
  consumed by the first report accepted for it and its identifier is never
  reused, so a report cannot be replayed against a later state.
- Submitting a report whose canonical digest equals the one already accepted
  for its packet succeeds and changes nothing.
- Role separation holds on `author.id`: an implementer is never also a
  reviewer, verifier or auditor in the same ledger, and an auditor is never
  also a reviewer or verifier.

**Reviewer body**

- `coverage` has exactly one entry per domain: `assessed` or `not-applicable`,
  each with a summary or rationale.
- `requirements` has exactly one entry per applicable requirement: `satisfied`,
  `violated` or `unverified`, each with evidence.
- A blocking finding cites an applicable `requirement` or an unacceptable
  `risk`, and states its failure mechanism, evidence, impact, why it must be
  resolved before this stage, and how a fix can be verified. A concern that
  fits neither goes in `contract_gaps`, where governance decides.
- A blocking finding in `optimization`, `performance`, `scalability`,
  `architecture`, `maintainability`, `technical-debt`, `simplicity` or
  `cost-efficiency` additionally states the proportionality of the proposed
  change, its regression risk and the cost of deferring it.
- T2 states expected benefit and deferral rationale. T3 states the evidence
  needed and when to reassess.
- A `violated` requirement is cited by at least one blocking finding that this
  report opens or reopens.
- The verdict equals the one implied by the content: `RED` if any blocking
  finding is opened or reopened; otherwise `INDETERMINATE` if any requirement
  is `unverified`; otherwise `GREEN`.

**Implementer body**

- Addresses every blocking finding in the packet with either a fix or a
  dispute. It may also fix non-blocking findings from the backlog.
- Each fix lists full commit hashes that lie between the round's candidate and
  the current one. The tree is clean.
- A finding flagged root-cause-required carries a root-cause analysis.
- A finding can be disputed once. After an auditor upholds it, it must be fixed.

**Verifier body**

- Gives `confirmed` or `rejected`, with evidence, for every claimed fix.
- May report regressions as new findings, each naming the finding whose fix
  introduced it.

**Auditor body**

- Discharges every escalation named in the packet with an analysis and a
  conclusion.
- Gives a decision (`uphold`, `withdraw` or `reclassify`) for every finding the
  packet lists as adjudicable, and for no other finding.
- May issue directives. May recommend, but not make, contract changes.

## 10. Convergence safeguards

Safeguards change the investigation procedure. They never lower the bar and
never open the gate. Each raises an escalation; any open escalation keeps the
gate closed and takes priority in `metron next`.

| Kind | Raised when | Owner | Discharged by |
| --- | --- | --- | --- |
| `dispute` | An implementer disputes a finding | auditor | Adjudication |
| `contradiction` | A finding declares that it reverses an earlier resolution | auditor | Adjudication |
| `component-churn` | The same component is fixed in 3 distinct rounds | auditor | Component-level audit |
| `regression-spiral` | A verification finds at least one blocking regression and no fewer than the fixes it confirms | auditor | Redesign assessment |
| `non-convergence` | The 3rd consecutive round ends without GREEN | auditor | Convergence analysis |
| `process-audit` | Every 5th consecutive round ends without GREEN | auditor | Independent process audit |
| `contract-gap` | A reviewer reports a risk the contract does not cover | governance | `metron resolve`, after amending the contract or dismissing with a rationale |
| `evidence-gap` | A verdict is `INDETERMINATE` | governance | `metron resolve`, after supplying evidence or amending the contract |

One safeguard is not an escalation: when a finding's recurrence count reaches
2, it is flagged **root-cause-required**, and the next fix for it must include
a root-cause analysis.

The thresholds 2, 3 and 5 are defaults and the only tunable values.
Governance resolutions and audit directives are carried into every later
packet.

## 11. The gate

```
G = C ∧ B ∧ E ∧ I ∧ V
```

| | Condition | Evaluated as |
| --- | --- | --- |
| C | Acceptance criteria satisfied | Every applicable requirement is `satisfied` in the reviewer report of the latest round |
| B | No blockers | No unresolved T0 or T1 finding, and no open escalation |
| E | Evidence sufficient | Every applicable requirement with a `command` has a passing result recorded for this candidate and this command text |
| I | Integrity | The current candidate digest equals the reviewed one; the tree is clean; the contract digest equals the pinned one; the ledger verifies and was written under this protocol version |
| V | Independent verdict | The latest round's verdict is `GREEN`, issued for this candidate under this contract digest, by an author who never acted as implementer |

`metron gate` exits 0 only when all five hold. Anything else, including an
internal error, an unreadable file or a missing tool, exits non-zero. No
iteration limit, timeout or failure ever produces approval.

A GREEN gate approves one candidate for one stage and the activity named in
`authorizes`. It does not claim the implementation is defect-free or optimal.

## 12. Command surface

| Command | Effect |
| --- | --- |
| `metron init` | Create `.metron/` with a commented contract template and an initialized ledger |
| `metron contract` | Validate the contract and show whether it matches the pinned digest |
| `metron contract pin -m <reason>` | Governance: authorize the current contract |
| `metron next` | Print the single next permitted action and the command that performs it |
| `metron check` | Run the applicable requirements' check commands on the candidate and record the results |
| `metron packet [-o file] [--format json\|prompt]` | Issue, or return again, the packet for the next action |
| `metron ingest <file\|->` | Validate and ingest a role report |
| `metron resolve <E-id> -m <decision>` | Governance: discharge a governance-owned escalation |
| `metron gate` | Evaluate the gate |
| `metron status`, `metron findings`, `metron log` | Read-only views of the ledger |
| `metron fsck` | Verify the ledger's hash chain and replay |
| `metron schema [name]`, `metron protocol` | Print the embedded schemas and protocol text |
| `metron version` | Print version information |

Commands whose output an orchestrator consumes accept `--json`.

Exit status is part of the public interface:

| Status | Meaning |
| --- | --- |
| 0 | Success. For `gate`, the gate is open. |
| 1 | A negative outcome: gate closed, report rejected, contract invalid, a check failed |
| 2 | Usage error |
| 3 | Environment or internal error: I/O, git unavailable, ledger corrupt, lock held |

## 13. Code layout

```
cmd/metron/         entry point
internal/core/      domain: vocabulary, contract rules, events, state, decide,
                    convergence, next action, gate. No I/O, no clock, no git.
internal/canon/     canonical JSON and digests
internal/schema/    structural validation against the embedded JSON Schemas
internal/protocol/  rendering of role instructions from embedded templates
internal/store/     state directory: ledger, lock, packets, reports;
                    strict contract loading
internal/vcs/       git plumbing through the git executable
internal/cli/       commands, flags, output
schemas/            canonical JSON Schemas (public protocol), embedded
protocol/           canonical protocol text and role instructions, embedded
```

Dependencies point inward: `cli` depends on everything; `store`, `schema`,
`vcs` and `protocol` depend on `core` and `canon`; `core` and `canon` depend
only on the standard library. Anything nondeterministic (time, the file
system, git) is observed outside the core and passed in as facts.

Metron requires `git` at runtime and nothing else.

## 14. Not in the first version

Agent execution (ACP through acpx, a generic command runner), Dagu workflow
templates, authenticated enforcement mode, and schema migrations beyond v1 are
deliberately absent. See [decisions/](decisions/) and
[trust-model.md](trust-model.md).
