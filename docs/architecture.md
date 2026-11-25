# Metron architecture

Metron is a convergent engineering review framework. It decides whether a
specific commit of a repository is approved for a specific lifecycle stage, and
it bounds how long the review process that leads to that decision can run
without a decision by the repository's owner.

This document is the normative design. Where code and this document disagree,
one of them has a bug. Trust assumptions are in [trust-model.md](trust-model.md).

## 1. Principles

| Principle | Meaning in Metron |
| --- | --- |
| Safety | The gate never opens while a mandatory requirement is violated or unverified. Every failure mode closes the gate. |
| Liveness | Every state names one action that someone can perform, and a campaign never runs unattended without bound: within the limits the contract sets, the gate opens or the next action belongs to governance. Possible improvements never block. Whether a campaign reaches GREEN depends on engineering judgments, which Metron does not make (section 10). |
| Proportionality | A finding blocks only if it is tied to a contract requirement or an unacceptable risk. Over-engineering is itself reviewable. |
| Empiricism | Decisions cite evidence. What cannot be decided without real-world data is a calibration, not a blocker. |
| Independence | Iteration counts, effort and deadlines change the investigation procedure, never the approval threshold. A reviewer is not told how many reviews came before, how they ended or who held them. |

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
| Orchestrators | When and how to execute the action Metron names | Choose a different action, skip one, override an outcome, or continue past an action that belongs to governance |
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

Nothing is frozen before the first release. Until then `metron.protocol/v1`
and the v1 document shapes can still change incompatibly, and a ledger written
by an earlier build may be rejected as malformed.

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
  work/             scratch space for skeletons and reports being written.
                    Ignored by git, and never read by metron on its own.
```

The state directory is `--state-dir`, else `$METRON_STATE_DIR`, else
`.metron/state`. In cooperative use, commit `state/` together with the work:
git history is the only anchor it has. In a protected environment, point the
state directory outside the repository, at a location agents cannot write.
Those are the only two choices: any other directory inside the repository is
refused, because writes to it would make the tree unclean or escape the
ignore and line-ending rules that `metron init` sets up for `.metron/state`.
The rule is applied to where the directory really is, after resolving symbolic
links, including when `.metron` or `.metron/state` is itself a link.

A role that must leave the candidate untouched still needs somewhere to write
its report. `.metron/work/` is that place: like everything under `.metron/` it
is outside the candidate and outside the cleanliness check.

`metron init` also writes `.metron/.gitattributes`, which stops git from
converting line endings, applying filters or merging textually under `state/`.
Stored reports are compared byte for byte with what was ingested, and two
ledgers can never be merged line by line.

Nothing under `.metron/` is part of the candidate, so nothing that affects the
outcome of a review belongs there: a check script kept under `.metron/` could
change without invalidating its recorded result.

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

The working tree is **clean** when all of the following hold outside
`.metron/`:

- `git status --porcelain` reports nothing: no modified, deleted or untracked
  file. Metron asks with the file system monitor disabled and with git's
  default, strictest comparison of file times, whatever the repository is
  configured to use.
- No tracked file, in the repository or in an initialized submodule, carries
  the `assume-unchanged` or `skip-worktree` index flag. Either flag makes git
  report a modified file as unchanged, so checks and reviewers would see
  content that is not the candidate. A sparse checkout sets the flag on every
  file it leaves out, so a sparse checkout is never clean.
- No tracked symbolic link resolves into `.metron/` or `.git/`, whose content
  is not part of the candidate and not covered by the points above.
- Git is not set to ignore case (`core.ignoreCase`) on a file system that
  distinguishes it. That combination arises when a repository is copied from
  a case-insensitive system, and it makes git overlook files whose names
  differ only by case.

Metron requires a clean tree whenever it binds something to a candidate.

**A candidate is its content, not its place in history.** A commit that
restores an earlier tree is the earlier candidate again, so undoing a change
returns to a candidate the ledger has already seen. Everything the ledger
records about a candidate is therefore bound to its digest: a verdict, a check
result, and every step of a finding (section 7). What was established on one
candidate is never carried over to a candidate on which the opposite was
recorded.

**What the cleanliness check does not cover.** It establishes that git
considers the working tree identical to the commit. It does not establish more
than that:

- Ignored files are outside the candidate and are not examined, whichever
  ignore mechanism hides them. The same holds for whatever a tracked symbolic
  link points to outside the repository.
- Local git configuration can make git consider different content identical: a
  clean filter, or `core.fileMode=false`, which git sets by itself on file
  systems without an executable bit. Metron does not second-guess these.

A check result is therefore only as reproducible as the environment it ran in,
as with any locally run test. Running checks from a fresh checkout of the
candidate in an environment nobody else controls is what closes this, and that
belongs to authenticated mode (see [trust-model.md](trust-model.md)).

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
| `scope` | yes | Statements describing what is under review; at least one |
| `operating_envelope` | yes | Statements of expected conditions and load; at least one. Findings outside it cannot block |
| `requirements` | yes | See below; at least one |
| `risks.unacceptable` | yes | `RISK-n` statements a blocking finding may cite; at least one |
| `risks.accepted` | no | Residual risks governance has accepted; never blocking |
| `not_applicable` | no | Domains waived for this stage, each with a rationale |
| `calibrations` | no | Provisional parameters, the evidence needed, and when to reassess |
| `convergence` | no | Thresholds; defaults 2, 3, 5 and 20. When present it states all four |

A **requirement** has an `id` (`REQ-n`), a `domain`, a `statement` (what must
hold), a `criterion` (the acceptance boundary: how anyone decides it holds), an
optional `command` (an automated check Metron runs itself) and an optional
`from` stage (default `prototype`). A requirement is **applicable** when
`from` is not later than the contract's `stage`. This is how acceptance is made
lifecycle-aware: a requirement that only matters in production is written down
now and starts blocking when the contract targets production.

A `command` is run by a POSIX shell (`sh -c`) from the repository root, on
every platform, so that the same command text means the same thing wherever a
result is recorded. On Windows, Metron uses the `sh` found on `PATH`, and
otherwise the one that ships with Git.

Every run is bounded. A command still running after the limit given to
`metron check --timeout`, 30 minutes unless stated, is stopped together with
what it started, and recorded as a failed result with exit status 124. A check
that hangs is therefore a failed step like any other (section 10), not a
campaign that waits for ever.

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
   waived domain has no applicable requirement, and the four thresholds are
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
governance pins it. A new pin voids the open packet. A verdict counts only
while the pinned digest is the one it was issued under: pinning a different
contract sets every earlier verdict aside, and pinning the earlier contract
again, unchanged, lets its verdicts count again, unless a pin in between
changed the stage: the gate looks only at rounds held since the stage was last
set. Pinning the digest that is
already pinned is refused. A finding that was disputed can be disputed again
after a new pin, because the contract it is judged against has changed.

A pin that changes `stage` is a new question for the gate, which then looks
only at the rounds held since. It is not a new campaign: round numbers and
findings carry over, and so does everything the loop has counted towards an
escalation (section 10). Amending the contract never erases the record of a
loop that is not converging.

## 7. Ledger

`ledger.jsonl` holds one event per line. State is never stored; it is the fold
of the events. The core exposes pure functions:

- `Decide(state, command, facts) -> events | error` enforces the state machine.
- `Apply(state, event) -> state` folds one event into state.

A command yields its events in a defined order, and the same state, command
and facts always yield the same events.

**Envelope.** Each line is a JSON object without insignificant whitespace,
with members in exactly this order: `seq`, `time`, `prev`, `type`, `more`,
`data`, `hash`. Who caused an event is part of its payload where that is
meaningful.

- `seq` starts at 1 and increases by 1.
- `time` is RFC 3339 UTC with second precision. The events of one command
  share one time.
- `prev` is the previous event's `hash`, or 64 zeros for the first event.
- `more` is the number of events of the same command that follow this one. It
  counts down to 0 within a command's batch, and the last line of a ledger has
  `more` 0. A ledger cut off in the middle of a batch therefore fails
  verification, even though what remains is a valid chain.
- `hash` is the lowercase hex SHA-256 of the line's bytes up to, but excluding,
  the last occurrence of `,"hash":"`, followed by a closing `}`.

Verification needs no canonicalization and can be reimplemented in any
language. Payloads are written in canonical JSON and never use a member named
`hash`; they use `digest`. The first event is `ledger.initialized`, and replay
rejects a ledger whose first event is anything else.

**Writing.** All events of one command are committed together or not at all:
the store writes the complete new ledger to a temporary file in the state
directory, syncs it, and renames it over `ledger.jsonl`. A leftover temporary
file is discarded by the next command that writes. Every command that writes
holds an exclusive operating-system lock on `<state>/lock` from before it reads
the ledger until after it commits; the lock disappears with the process, so a
crash never leaves it held.

A packet file is written after the ledger commit that issues the packet. A
crash in between leaves an open packet without its file, which is detected:
the packet is voided and issued again.

A report file is written before the ledger commit that accepts the report,
because the exact bytes submitted cannot be recreated afterwards. Until that
commit the file belongs to no accepted report and may be replaced. A crash in
between leaves a file the ledger does not refer to, which the next ingestion
for that packet overwrites.

A stored packet file whose identifier the ledger never issued can only mean
that the ledger was rolled back or replaced, since a packet file is written
after its ledger commit. `metron fsck` reports it, and the gate cannot be
evaluated until the ledger is restored. This catches a ledger restored from an
older version by accident; it does not stop someone who also removes the
files.

For the same reason `metron init` does not create a ledger in a state
directory that already holds packets or reports: the ledger they belong to was
lost, not never written. There are two ways out and both are governance's to
choose: restore the ledger, or set the old state directory aside and start a
new history, which discards every verdict recorded so far.

`metron fsck` reports every missing or mismatching file of an accepted report
or of a packet that was not voided. The gate requires less: the report that
carries the verdict it relies on must be stored intact. A lost report file can
be restored from version control or by ingesting the identical bytes again;
failing that, a change to the candidate leads to a new review and a new
report, so the loss of a file never closes the gate for good.

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
| `finding.fix-confirmed` / `finding.fix-rejected` | A verifier's result, with its judgment of the fix's scope |
| `finding.addressed` | A reviewer found an open non-blocking finding addressed |
| `finding.reopened` | A resolved finding was reopened, with a reason |
| `finding.withdrawn` / `finding.reclassified` | An auditor adjudication |
| `escalation.raised` / `escalation.resolved` | A convergence safeguard fired or was discharged; resolving a `suspension` resumes the campaign |
| `directive.issued` | Binding guidance from an audit, carried into later packets |
| `decision.superseded` | An audit withdrew an earlier directive or audit conclusion |

**Finding state machine**

```
            fix-claimed            fix-confirmed
  open ─────────────────▶ claimed ───────────────▶ resolved
   ▲ │                       │                       ▲ │
   │ │     fix-rejected      │                       │ │ reopened
   └─┼───────────────────────┘◀──────────────────────┼─┘
     │                    addressed (non-blocking only)
     ├───────────────────────────────────────────────┘
     └── withdrawn (terminal; auditor adjudication only)
```

A finding is **unresolved** while `open` or `claimed`. A fix is claimed for an
`open` finding and, when the candidate changed after the claim, claimed again
for a `claimed` one (section 8). `addressed` is how the backlog is closed: a
reviewer finds that the candidate no longer exhibits an open non-blocking
finding and says so, with evidence. A blocking finding is resolved only by a
fix that a verifier confirms. Its **recurrence count**
is the number of `fix-rejected` plus `reopened` events it has received.
Reclassification changes the tier and nothing else, so it can make blocking
only a finding that, as recorded, already meets every rule of a blocking
finding under the pinned contract (section 9).

Each step in a finding's history names the candidate it was made on: the one
the reviewer, verifier or auditor examined, or the one a fix was claimed on. A
finding is **recorded on** every candidate on which it was opened or reopened,
or on which a fix for it was rejected: on those candidates the ledger holds
that the defect is present. A candidate **carries** a finding when the finding
is blocking and `resolved` and the candidate is one it was recorded on. The fix
that resolved it was necessarily confirmed on another candidate, so this one
does not contain it.

Reopening requires one of five reasons: `new-evidence`, `regression`,
`invalid-verification`, `requirement-changed`, `contract-conflict`.

A finding also records the digest of the contract it was established under.
It cites a requirement or a risk by identifier, and after a pin the same
identifier may say something else, so a packet shows whether a finding
predates the pinned contract.

**Identifiers.** Findings are `F-0001`, packets `P-0001`, escalations
`E-0001`, decisions `D-0001`: a prefix and a sequence number zero-padded to at
least four digits, assigned in order of creation and never reused.

**Event order.** Accepting a report yields, in this order: `report.accepted`;
the finding events in the order the report lists them, reopenings before
findings marked addressed and those before new findings; `verdict.recorded`;
`directive.issued` in report order; `decision.superseded` in report order;
`escalation.resolved` in identifier order; `escalation.raised`, ordered by
kind as listed in section 10 and then by subject.

Two other commands can raise an escalation. Recording check results yields
the `evidence.recorded` events in contract order, then the escalations of the
failure streak if a check failed. Issuing a packet yields `packet.voided` for
a stale open packet, `round.opened` for a review, `packet.issued`, and then a
`suspension` if that packet uses up the campaign budget.

A command's events form one batch (`more`). A batch that records a failure
counts as one failed step, however many of its events record one.

## 8. The review loop

`metron next` returns exactly one action. It is the first row of this table
whose condition holds.

| # | Condition | Action | Owner |
| --- | --- | --- | --- |
| 1 | The ledger fails verification | `repair-ledger` | governance |
| 2 | The contract is invalid, or its digest differs from the pinned one | `pin-contract` | governance |
| 3 | A packet is open and still valid | `submit-report` | the packet's role |
| 4 | A governance-owned escalation is open | `adjudicate` | governance |
| 5 | The working tree is not clean | `commit-changes` | implementer |
| 6 | An auditor-owned escalation is open | `audit` | auditor |
| 7 | A blocking finding is `open`, or a fix was claimed on a candidate other than this one | `fix` | implementer |
| 8 | The candidate carries a blocking finding (section 7) | `commit-changes` | implementer |
| 9 | A mandatory check has no passing result on the candidate | `run-checks` | implementer |
| 10 | A finding is `claimed` | `verify` | verifier |
| 11 | The candidate has no GREEN verdict under the pinned contract | `review` | reviewer |
| 12 | Otherwise | `none` (the gate is open) | |

An open packet is **valid** while the pinned contract digest is the one it was
issued under and, for a read-only role, the candidate digest is unchanged and
the tree is clean. An open packet that is no longer valid is ignored by
`metron next` and voided when the next packet is issued.

Row 5 precedes every packet-issuing row because a packet is bound to a
candidate, and a candidate is a commit. Row 9 means that neither a
verification nor a review starts on a candidate whose own mandatory checks
fail; the latest result per requirement, candidate and command text is the one
that counts.

**A correction is one batch on one candidate** (rows 7, 9 and 10). The
implementer fixes or disputes every open blocking finding; the checks pass on
the result; then one verification covers every claimed fix. A fix is claimed
on a candidate. If the candidate changes before the verification, whether
because a check failed and was corrected or for any other reason, the claims
have lapsed: row 7 applies again, and the implementer claims each fix again,
with every commit that belonged to it, in a report that accounts for the new
commits. A finding an audit upholds while other fixes are already claimed is
fixed first, and since fixing it changes the candidate, the fixes claimed
before are claimed again in the same report. So every fix a verifier sees was
claimed on the candidate it is verified on, passed the rules of section 9 on
that candidate, and lists all its commits; nothing reaches a verifier with a
commit that no fix lists.

Row 8 is what keeps an unchanged candidate from being submitted again in the
hope of a different verdict. A RED verdict rests on the blocking findings its
report opened or reopened, and each of them is recorded on the candidate. While
one is unresolved, row 7 applies. Once its fix is confirmed, on another
candidate by necessity, returning to this one neither revives an earlier
verdict nor earns a new review: the candidate has to change. The same candidate
can be reviewed again only when what its verdict rested on is gone: every such
finding was withdrawn or reclassified to a non-blocking tier by an audit, or,
after INDETERMINATE, governance resolved the evidence gap (row 4). Resolving
some other escalation, re-running the checks or upholding a disputed finding
changes none of that.

A **round** is one independent, read-only, full-scope review of one candidate.
Issuing a review packet opens a new round. Rounds are numbered from 1 across
the whole ledger, so a round number identifies one review unambiguously. A
round whose packet was voided has no verdict and is not a failed step, though
its packet counts against the campaign budget (section 10). Fixing and
verifying happen between rounds:

```
review ──RED──▶ fix ──▶ run-checks ──▶ verify ──all confirmed──▶ review
   │             ▲           │            │
   │             └───────────┴────────────┘
   │               a check fails, a fix is rejected, or a regression blocks
   ├──INDETERMINATE──▶ adjudicate (governance supplies evidence or amends the contract)
   └──GREEN──▶ gate
```

The review that follows a correction is of the whole candidate, like any
other. It can find anything, including what the correction broke, and it alone
issues a verdict: a verification establishes that fixes hold, never that a
candidate is acceptable. Its packet names the candidate the fixes were
verified on. That is usually the candidate under review; when it is not,
changes were made after the verification that no verifier saw, and the
reviewer is told so.

**Improvements are not corrections.** A fix packet holds the blocking findings
and nothing else. The backlog, the open findings of the non-blocking tiers, is
worked on in ordinary development: commits made outside any correction, on a
candidate that then needs the review any changed candidate needs. A reviewer
closes a backlog finding by marking it addressed (section 9). Keeping the two
apart means an optional change can never fail a mandatory verification, and a
mandatory one is never widened by an optional one.

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

What a packet carries depends on the action:

| Action | Role | Bound to | Subjects | Findings included |
| --- | --- | --- | --- | --- |
| `review` | reviewer | The candidate, read-only; names the candidate the fixes were last verified on, if any since the previous verdict | none | Every finding in the ledger |
| `fix` | implementer | The candidate at issue, with the round's candidate as base | The whole batch: every open blocking finding and every claimed one | Subjects |
| `verify` | verifier | The candidate, read-only, with the round's candidate as base | Every claimed finding | Subjects |
| `audit` | auditor | The candidate, read-only | Every open auditor-owned escalation; the findings of `dispute` and `contradiction` escalations are adjudicable | Every finding in the ledger |

Every packet also carries the contract, the latest check result per applicable
requirement for its candidate, and the decisions recorded so far, sorted into
those that bind and those that no longer do (section 10). Each finding carries
its history: every event recorded for it, with the candidate it was made on
and the evidence or rationale given at the time.

A fix, verify or audit packet names the round it follows and lists the earlier
rounds of the stage with their verdicts. **A review packet does not**: no round
number, no earlier verdict, no reviewer identity, in the packet, in its brief
or in its prompt, and the findings it carries come without the round each
step belongs to; their order remains. These have no use in a reviewer's
report, and the Independence principle says they must not move the threshold.
This is about what a reviewer is handed, not secrecy: in local mode the
ledger is a file in the repository, and `metron status` shows the rounds to
whoever asks. What a reviewer does get is everything
technical: every finding with its full history, the binding and the earlier
decisions, the check results, and the candidate the fixes were verified on.
Nothing that limits what a review can conclude is withheld.

A report is a versioned envelope around a role-specific body:

| Member | Content |
| --- | --- |
| `schema` | `metron.report/v1` |
| `packet` | The packet this report answers |
| `packet_digest` | The digest of that packet, copied from the skeleton. Packet identifiers repeat from one ledger to the next; the digest does not, so a report written for another repository, another ledger or an earlier issue of the same identifier is refused |
| `role` | The role that produced it |
| `author` | Declared identity: `id`, `kind` (`agent` or `human`), optionally `model`, `harness`, `session`. `id` is 1 to 128 printable ASCII characters without spaces, so that two identities cannot differ only by something invisible |
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
  each with a summary or rationale. A domain in which an applicable
  requirement lies is always `assessed`.
- A finding of any tier that cites a requirement cites an applicable one, and
  one that cites a risk cites an unacceptable one.
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
  report opens or reopens, and a requirement cited by such a finding is
  `violated`.
- A blocking finding can be reopened only while it still cites an applicable
  requirement or an unacceptable risk of the pinned contract. Otherwise the
  reviewer opens a new finding against the current contract.
- The verdict equals the one implied by the content: `RED` if any blocking
  finding is opened or reopened; otherwise `INDETERMINATE` if any requirement
  is `unverified`; otherwise `GREEN`.
- `addressed` may list open findings of a non-blocking tier that the
  candidate no longer exhibits, each with evidence. That resolves them, and
  like any resolved finding they can be reopened later. A blocking finding
  cannot be listed.

**Implementer body**

- Addresses the subjects of the packet and nothing else. An open finding is
  fixed or disputed. A finding whose fix is already claimed is claimed again,
  with every commit of the earlier claim that is still part of the candidate,
  whenever the report is submitted on another candidate than the one the
  claims stand on; it cannot be disputed. Non-blocking findings are not fixed
  in a correction.
- Each fix lists full commit hashes and names at least one component. The
  tree is clean.
- Every commit made since the round's candidate that changes something
  outside `.metron/` is listed under a fix, in this report or in one accepted
  earlier. Metron lists the interval itself, with `git rev-list`, and refuses
  a report that leaves a commit out: a correction has no unattributed
  changes. The rule holds while the round's commit still exists in the
  repository; after history was rewritten and that commit discarded, the
  interval cannot be listed and only the commits a report names are examined.
- Every listed commit is part of the current candidate's history, changes
  something outside `.metron/`, and is not already contained in the round's
  candidate. The last condition is checked only while that commit still
  exists; history may have been rewritten since. In a shallow clone, a commit
  at the boundary of the history is judged as if it had no parent.
- For every finding it fixes, the candidate is one the finding was never
  recorded on (section 7). A fix committed before the packet was requested is
  therefore valid; claiming a rejected fix again without a new change is not,
  and neither is returning to any tree on which the finding was opened,
  reopened or had a fix rejected, however many changes lie in between.
- A finding flagged root-cause-required carries a root-cause analysis.
- A finding can be disputed once per pinned contract. After an auditor upholds
  it, it must be fixed, unless the contract is pinned again.

**Verifier body**

- Gives `confirmed` or `rejected`, with evidence, for every claimed fix.
- Gives every fix a `scope`, `proportionate` or `excessive`, with evidence:
  whether its commits change only what resolving the finding needs. That
  every commit is listed under a finding shows only that it was labelled; the
  verifier judges whether it belongs. A fix whose scope is `excessive` is
  `rejected`, whether or not the finding is resolved, and a report that
  confirms one is refused.
- May report regressions as new findings, each naming the finding whose fix
  introduced it.

**Auditor body**

- Gives an analysis and a conclusion for every escalation named in the packet,
  one entry each, and for no other escalation.
- Gives a decision (`uphold`, `withdraw` or `reclassify`) for every finding the
  packet lists as adjudicable, and for no other finding.
- Reclassifies a finding to T0 or T1 only if the finding as recorded cites an
  applicable requirement or an unacceptable risk of the pinned contract and
  states its blocking rationale, its verification and, in the domains that
  require it, its proportionality. An audit cannot supply what the finding
  lacks: it upholds the finding at its tier, and a reviewer opens a blocking
  finding that stands on the contract. A finding never blocks on an auditor's
  word alone.
- May issue directives. May recommend, but not make, contract changes.
- May supersede an earlier audit decision, a directive or an audit's
  conclusion, by naming it in `supersede` with a rationale. A governance
  decision is not an auditor's to supersede.
- May ask, in `suspend`, for the campaign to be suspended until governance
  decides, with the reason.

## 10. Convergence safeguards

Safeguards change the investigation procedure, and in the end they stop it.
They never lower the bar and never open the gate. Each raises an escalation;
any open escalation keeps the gate closed and takes priority in `metron next`.

| Kind | Raised when | Owner | Discharged by |
| --- | --- | --- | --- |
| `dispute` | An implementer disputes a finding | auditor | Adjudication |
| `contradiction` | A finding declares that it reverses an earlier resolution | auditor | Adjudication |
| `component-churn` | Fixes change the same path in 3 distinct rounds of a campaign; raised once per path per campaign, one escalation for all the paths a report brings to the threshold | auditor | Component-level audit |
| `regression-spiral` | A verification finds at least one blocking regression and no fewer than the fixes of blocking findings it confirms | auditor | Redesign assessment |
| `non-convergence` | The failure streak reaches 3; raised once per streak | auditor | Convergence analysis |
| `process-audit` | The failure streak reaches a multiple of 5 | auditor | Independent process audit. Discharging it raises a `suspension` |
| `contract-gap` | A reviewer reports a risk the contract does not cover | governance | `metron resolve`, after amending the contract or dismissing with a rationale |
| `evidence-gap` | A verdict is `INDETERMINATE` | governance | `metron resolve`, after supplying evidence or amending the contract |
| `suspension` | A process audit is discharged; an auditor asks for it; or the campaign issues its 20th packet without an approval | governance | `metron resolve`: the decision to resume |

One safeguard is not an escalation: when a finding's recurrence count reaches
2, it is flagged **root-cause-required**, and the next fix for it must include
a root-cause analysis.

The thresholds 2, 3, 5 and 20 are defaults and the only tunable values:
`root_cause_after`, `convergence_audit_after`, `process_audit_after` and
`suspend_after`.

Component churn is counted on facts: the paths that the commits listed under a
fix change outside `.metron/`, which Metron reads from the repository when the
fix is claimed. The component names a report gives are descriptions for
people; two reports rarely spell them alike, so nothing is counted on them.

**Decisions.** An audit directive and the resolution of an escalation, by an
auditor or by governance, are decisions. Each records where it was made: the
stage and contract digest pinned at the time, the candidate (the one the
auditor examined or, for a governance resolution, the one the escalation was
raised on), and the requirements the escalation named. That decides where it
applies. A decision **binds** a packet's role when all of these hold:

- no audit has superseded it;
- the contract it was made under is the pinned one. A decision answers a
  question the contract poses, so any change of contract ends it, whether or
  not the stage changed. Pinning that contract again makes it bind again, as
  it makes the verdicts issued under it count again;
- for the resolution of an `evidence-gap`, which is evidence governance
  supplies: the packet's candidate is the one the gap was raised on. Evidence
  establishes something about one candidate and is not carried to another.

Every other decision is listed in the packet as an **earlier decision**, with
the reason it no longer binds: context for what was decided and why, not a
rule. A decision that should outlive a contract change is issued again.

**Campaigns.** A campaign is the work towards one approval. It begins with the
ledger, after an approval, or when governance resumes a suspended campaign. It
ends in one of two ways: **approved** or **suspended**, when a `suspension` is
open. Nothing else ends it, and nothing else begins one: not a pin, not a
change of stage, not a new candidate, not the discharge of an audit.

The ledger cannot see the working tree, so it takes a campaign as approved
when a command leaves it showing everything of the gate that does not depend
on the tree: the latest round is a GREEN round of the current stage under the
pinned contract, no escalation is open, and no blocking finding is unresolved
or carried by that round's candidate. A GREEN verdict that leaves an
escalation open, or that a pin has set aside, approves nothing and ends
nothing.

**The failure streak** counts failed steps. A failed step is a command whose
events record a failure:

- a review whose verdict is not GREEN;
- a verification that rejects a fix or reports a blocking regression;
- a check recording in which a check failed, a check stopped for exceeding its
  time limit included.

The streak restarts at zero with a GREEN verdict and when governance resumes a
suspended campaign. Steps that succeed in between (a confirmed fix, a passing
check, a discharged audit) leave it where it is: they are progress on a
finding, not on the campaign. Because the streak counts every kind of step, a
loop that never reaches a second review, such as a fix rejected again and
again or a check that cannot pass, is seen as soon as one that fails round
after round. Failing checks can make an audit due before any round has a
verdict; its packet then names no round, and the escalation records round
0.

**The campaign budget** counts every packet issued in the campaign, whatever
becomes of it: answered, or voided before anyone answered. It is what bounds
a campaign whose individual steps keep succeeding, or whose packets are never
answered. The packet that uses the budget up is issued together with the
suspension. Its role may still answer it, and then nothing more is issued.

**Suspension** is termination without approval. While a `suspension` is open:

- the gate is closed, even if the last verdict is GREEN;
- `metron next` names `adjudicate`, owned by governance (section 8, row 4);
- no packet is issued and no check result is recorded. Only the report of a
  packet that was already open is still accepted.

Only governance ends it, with `metron resolve`: an explicit decision to
resume, usually after doing what the audit found necessary, such as amending
the contract. Resuming begins a new campaign: the failure streak, the budget
and the component-churn counts start again. Governance may also leave the
campaign suspended, which is how a campaign that cannot converge ends.
Pinning a contract does not resume anything.

**What this guarantees, and what it does not.** Whatever the roles do:

- *Safety.* The gate opens only when all five conditions of section 11 hold.
- *No approval through exhaustion.* No count, budget or time limit opens the
  gate or lowers a threshold. Running out of budget suspends.
- *A next action.* Every state names exactly one action and its owner, and a
  report or command that performs it as the protocol describes is accepted.
- *Bounded unattended execution.* An orchestrator that performs the actions
  `metron next` names reaches an open gate or an action owned by governance
  after at most `suspend_after` packets, and after at most
  `process_audit_after` failed steps and the audits they raise.

Metron cannot guarantee that a campaign reaches GREEN. That takes a candidate
that can meet the contract, a contract that can be met, and roles that judge
well; none of these is something a deterministic program can establish. A
suspended campaign is therefore not a failure of the process: it is the
process stopping where only a decision can help. What Metron does not see at
all is a role that never reports, or whose reports are all rejected: a
rejected report changes nothing, so an orchestrator must bound its own
retries of one packet.

## 11. The gate

```
G = C ∧ B ∧ E ∧ I ∧ V
```

| | Condition | Evaluated as |
| --- | --- | --- |
| C | Acceptance criteria satisfied | Every applicable requirement is `satisfied` in the reviewer report of the latest round |
| B | No blockers | No unresolved T0 or T1 finding, no T0 or T1 finding carried by this candidate (section 7), no claimed fix awaiting verification, and no open escalation |
| E | Automated evidence sufficient | Every applicable requirement with a `command` has a passing result recorded for this candidate and this command text |
| I | Integrity | The current candidate digest equals the reviewed one; the tree is clean; the contract digest equals the pinned one; the ledger verifies and was written under this protocol version; the report carrying the verdict is stored intact |
| V | Independent verdict | The latest round's verdict is `GREEN`, issued for this candidate under this contract digest, by an author who never acted as implementer |

Condition E is about automated evidence only. A requirement without a
`command` adds nothing to it, and a contract with no command at all satisfies
it trivially; such requirements are established by the reviewer's assessment
under condition C, with the evidence the report cites. Evidence governance
supplies after an INDETERMINATE verdict is a decision bound to its candidate
(section 10) and reaches the gate only through a later review.

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
| `metron check [--timeout <duration>]` | Run the applicable requirements' check commands on the candidate, each within the time limit, and record the results. A recording with a failure is a failed step (section 10) |
| `metron packet [-o file] [--format json\|prompt\|skeleton]` | Issue, or return again, the packet for the next action: as JSON, as a self-contained prompt, or as a report skeleton to fill in |
| `metron ingest <file\|->` | Validate and ingest a role report |
| `metron resolve <E-id> -m <decision>` | Governance: discharge a governance-owned escalation. Resolving a `suspension` resumes the campaign |
| `metron gate` | Evaluate the gate |
| `metron status`, `metron findings [--backlog\|--all]`, `metron log` | Read-only views of the ledger. `findings` lists what is unresolved, the non-blocking backlog, or everything |
| `metron fsck` | Verify the ledger's hash chain and replay, every event against the event schema, and every stored file against the ledger |
| `metron schema [name]`, `metron protocol` | Print the embedded schemas and protocol text |
| `metron version` | Print version information |

Commands whose output an orchestrator consumes accept `--json` and then print
exactly one JSON document, whose `schema` member names and versions its shape.
A command that fails under `--json` prints a `metron.error/v1` document with
its exit status and message.

`metron next --json` separates what can be executed from what must be
completed by someone. `command` is present only when it is a complete command
that is safe to run as it stands in a POSIX shell: it never contains a file
name or any other text taken from the repository, and anything taken from the
invocation, such as the state directory, is quoted for the shell. A step that
needs a decision, a message or a file carries `usage` instead, a template with
placeholders, and the things it applies to in their own members.

Exit status is part of the public interface:

| Status | Meaning |
| --- | --- |
| 0 | Success. For `gate`, the gate is open. |
| 1 | A negative outcome: gate closed, report rejected, contract invalid, a check failed |
| 2 | Usage error |
| 3 | Environment or internal error: I/O, git unavailable, ledger corrupt, lock held |

For `metron gate` the distinction matters to an orchestrator: 1 means the
gate was evaluated and is closed; 3 means it could not be evaluated, because
the ledger is missing, unreadable or fails verification, so someone has to
intervene. A closed gate does not mean the loop continues. Whether it does is
for `metron next` to say. `metron next` and `metron status` report a ledger
problem as their result and exit 0; `metron fsck` exits 1 when it finds one.

**The orchestrator's stop rule.** An orchestrator runs the action `metron
next` names and stops in two cases: the step is `none`, because the gate is
open, or the `owner` is `governance`, because the next action is a decision
only the repository's owner can make (`repair-ledger`, `pin-contract`,
`adjudicate`). It does not perform a governance action itself, and it does not
look for another action to run instead. Metron enforces its side of this: while
governance owns the next action no packet is issued, and while the campaign is
suspended no check is recorded either, so `metron packet` and `metron check`
exit 1. A campaign stopped for governance has not been approved, and it must
not be reported as if it had.

## 13. Code layout

```
cmd/metron/         entry point
internal/core/      domain: vocabulary, contract rules, events, state, decide,
                    convergence, next action, gate. No I/O, no clock, no git.
internal/canon/     canonical JSON and digests
internal/schema/    structural validation against the embedded JSON Schemas
internal/brief/     rendering of role briefs from the embedded protocol templates
internal/store/     state directory: ledger, lock, packets, reports;
                    strict contract loading
internal/vcs/       git plumbing through the git executable
internal/cli/       commands, flags, output
schemas/            canonical JSON Schemas (public protocol), embedded
protocol/           canonical protocol text and role instructions, embedded
```

Dependencies point inward: `cli` depends on everything; `store`, `schema`,
`vcs` and `brief` depend on `core` and `canon`; `core` and `canon` depend
only on the standard library. Anything nondeterministic (time, the file
system, git) is observed outside the core and passed in as facts.

Metron requires `git` at runtime and nothing else.

## 14. Not in the first version

Agent execution (ACP through acpx, a generic command runner), Dagu workflow
templates, authenticated enforcement mode, and schema migrations beyond v1 are
deliberately absent. See [decisions/](decisions/) and
[trust-model.md](trust-model.md).
