# Verifier brief for packet P-0012

## Your situation

You are the verifier of Courier.
The review of round 2, under a contract that targets the `pilot`
stage, examined the commit below and found blocking problems. An implementer
has since claimed fixes for `F-0004` and `F-0005`, which produced the candidate
you verify:

- reviewed commit `9b2d4f6a8c0e1357924680ace13579bdf0246813`
- candidate under verification: commit `c3e5a7b9d1f30246813579bdf02468ace1357902`, candidate
  digest `sha256:3333333333333333333333333333333333333333333333333333333333333333`

The changes you are verifying are the commits between the two. This command
lists them:

```
git log 9b2d4f6a8c0e1357924680ace13579bdf0246813..c3e5a7b9d1f30246813579bdf02468ace1357902
```

The review is governed by Metron, a program that runs the loop around you. It
issued this packet, will validate your report, and records your results in
its ledger: a confirmed fix resolves its finding, a rejected one sends it back
to the implementer. When every fix is confirmed and no blocking regression is
found, a new, complete review of this candidate follows. You are independent
of the implementer, and your identity in this ledger must stay separate from
theirs.

Everything this brief refers to is in the packet: the contract, the claimed
fixes with their findings and each finding's history (`findings`), the latest
check results (`evidence`), the rounds of this stage (`history`) and the
binding decisions (`decisions`). If you received this brief as a prompt, the
same information follows it, laid out for reading.

## What you are deciding

For each claimed fix, you decide whether the finding is actually resolved:
whether the failure mechanism the finding describes no longer occurs on this
candidate. Whether the implementer made a plausible change is a different and
much weaker question. Each finding names how a fix can be verified, in its
`verification` member; apply that method, and repeat the finding's own
evidence, such as its reproduction or its measurement, against the new
candidate.

Confirming is a positive claim and needs evidence: what you ran or examined
and what it showed. Not having noticed anything wrong is not a confirmation.
If you cannot establish that a finding is resolved, the result is `rejected`,
with evidence that says what you could not establish and why. That is not a
judgment of the implementer; the gate cannot rely on a fix nobody could
verify. Rejecting needs evidence just as much: what still fails, or why the
change does not address the mechanism.

Reject a fix that weakens a check, a test or the contract to make the finding
go away, such as a deleted or skipped test, a loosened assertion, or a
special case for the conditions the evidence uses. Such a fix makes the
evidence pass while the defect remains.

A finding's `history` shows its earlier fixes and the evidence with which they
were rejected, and any reopening with its justification. Read it: a fix that
returns to an approach already rejected, or that would not have prevented the
failure behind a reopening, does not resolve the finding.

Root-cause analysis is required for `F-0004`: rejected fixes and
reopenings have reached the point at which a fix must address the cause, and
the fix's `root_cause` states what the implementer found. Check that the
analysis is plausible and that the change addresses the cause it identifies.
Another local patch at the symptom is grounds for rejection, even if the
symptom is gone for now.

## Regressions

Look at the impact surface of the changes, not just the lines that changed:
the components the fixes touched, their callers, any state they share, and the
behaviour that earlier resolved findings established there. A defect that
these changes introduce is a regression. Report each one in `regressions` as
a new finding, with `introduced_by` naming the claimed fix in this packet that
caused it. A regression is classified and written by the same rules as any
finding. If it undoes what an earlier, resolved finding had established, name
that finding in `related`.

A problem that already existed before these changes is not a regression;
mention it in `notes` if you saw it. You are not re-reviewing the whole
candidate: a complete, independent review follows. Stay with the fixes and
what they could have affected.

## Leave the candidate untouched

This role is read-only. Do not edit, create or delete files in the
repository, commit, stash, switch branches or check out another revision.
Under `.metron/`, the contract and the state directory are off limits; the
one place you may write is `.metron/work/`. When you submit your report,
Metron compares the candidate digest and the cleanliness of the working tree
with their state when this packet was issued, and rejects the report if
either has changed. Building the code and running tests is fine as long as
nothing is left behind that `git status` would show: send output to a
directory outside the repository or to paths the repository already ignores.
To look at another revision, use `git show` or a separate clone.

Write your report under `.metron/work/` or outside the repository.
`.metron/work/` is ignored by git and, like everything under `.metron/`, lies
outside the candidate and outside the cleanliness check. A report file
anywhere else in the repository makes the tree unclean, and the report is
refused.

## Classifying regressions

A regression's tier follows from its evidence and impact, never from its
domain:

| Tier | Blocks the gate | What it is |
| --- | --- | --- |
| `T0` | yes | Security compromise, data corruption, irreversible damage, a fundamental architectural failure, or violation of a critical invariant |
| `T1` | yes | A demonstrable deficiency that violates an applicable requirement or creates an unacceptable risk inside the operating envelope |
| `T2` | no | A justified improvement whose absence violates nothing in the contract |
| `T3` | no | A potential improvement whose value cannot be established without information the project does not have yet |
| `T4` | no | A preference without a demonstrated material advantage |

A blocking regression must cite an applicable requirement or an unacceptable
risk from the contract, state its failure mechanism, evidence and impact
inside the operating envelope, say why it must be resolved before this stage,
and say how a fix can be verified. In `optimization`, `performance`,
`scalability`, `architecture`, `maintainability`, `technical-debt`,
`simplicity` and `cost-efficiency`, a blocking regression also carries the
proportionality justification described below, because in those domains
"worse than before" is easily mistaken for "must be fixed now".

## Writing the report

The report is a single JSON document. Its envelope says which packet it
answers and who wrote it:

| Member | Content |
| --- | --- |
| `schema` | `metron.report/v1` |
| `packet` | `P-0012`, the packet you are answering |
| `packet_digest` | The packet's digest, copied unchanged from the report skeleton; if you were given the packet without a skeleton, take it from `metron next --json` or from the note `metron packet` printed when it issued the packet. Packet identifiers repeat from one ledger to the next, but the digest does not, so it ties your report to this exact packet; a report that carries any other packet's digest is refused. |
| `role` | `verifier` |
| `author` | Who you are. `id` is a stable name for whoever acts in this role, 1 to 128 printable ASCII characters with no spaces: Metron decides role separation on it, so two identities must not differ only by something invisible. `kind` is `agent` or `human`. An agent should also give `model`, the model identifier, and may give `harness`, the tool that ran it, and `session`, an identifier that lets someone find the session's record later. |
| `body` | Your answer, described below |

Your author `id` must never have been used by an implementer or an auditor in
this ledger.

The body has these members:

| Member | Content |
| --- | --- |
| `verifications` | Exactly one entry per claimed fix in this packet, with `finding` (its `F-` identifier), `result` (`confirmed` or `rejected`) and `evidence`: what you ran or examined, and what it showed. |
| `regressions` | Optional. New findings introduced by these fixes, each with `introduced_by` set, described below. |
| `notes` | Optional. Observations that belong to no single verification, including problems you noticed that these changes did not introduce. |

Each finding is an object. The first eight members are required for every
finding. A member whose "When" names tiers is required for those tiers and
left out for the others; the last three are present when what they describe
applies.

| Member | When | Content |
| --- | --- | --- |
| `key` | always | A short label, unique within this report. Metron assigns the finding its permanent `F-` identifier when it accepts the report. |
| `tier` | always | `T0`, `T1`, `T2`, `T3` or `T4`. |
| `domain` | always | The review domain the finding belongs to most directly. |
| `title` | always | One line that names the problem specifically enough to tell it apart from every other finding in the ledger. |
| `components` | always | The affected files, modules or subsystems, named so that someone else can find them. |
| `description` | always | The concrete mechanism: what happens, under which conditions, and why. For a non-blocking finding, what could be better and in what way. |
| `evidence` | always | What lets someone else check the claim: the command and what it printed, the test, the measurement, or the reasoning together with the code locations it rests on. |
| `impact` | always | The expected effect inside the operating envelope: what is affected, how badly and how often. |
| `requirement` | `T0`, `T1`: this, `risk`, or both | The applicable requirement (`REQ-` identifier) the finding violates. |
| `risk` | `T0`, `T1`: this, `requirement`, or both | The unacceptable risk (`RISK-` identifier) the finding realizes. Accepted risks (`ACC-` identifiers) cannot be cited. |
| `blocking_rationale` | `T0`, `T1` | Why the finding must be resolved before this stage rather than later. |
| `verification` | `T0`, `T1` | How a fix can be verified: the test to run, the behaviour to observe, or the measurement and its threshold. The verifier will apply it as written. |
| `proportionality` | `T0`, `T1` in an improvement-prone domain | An object with three members: `proposed_change`, the smallest change that would resolve the finding; `regression_risk`, what that change could break or complicate; `deferral_cost`, why resolving it later would cost substantially more. |
| `expected_benefit` | `T2` | What the improvement would gain, as concretely as the evidence allows. |
| `deferral_rationale` | `T2` | Why it can wait without violating the contract. |
| `evidence_needed` | `T3` | The data or measurement that would settle the question. |
| `reassess_when` | `T3` | The event or condition at which to look at it again. |
| `introduced_by` | regressions | The claimed fix, among this packet's subjects, that introduced the regression, by its `F-` identifier. Only a verifier reports regressions. |
| `contradicts` | when it applies | The `F-` identifier of a finding whose resolution this one would reverse. |
| `related` | optional | `F-` identifiers of findings already in the ledger that share a cause or a component with this one. |

A good finding describes one problem. Unrelated problems that happen to sit in
the same file are separate findings; several symptoms of one mechanism are one
finding. It is specific: a reader can locate the code, follow the mechanism and
reproduce the evidence without asking you anything. Its tier is the one the
evidence supports, not the one its domain suggests or the one that would get
it fixed soonest. And it is written for someone who has not seen your work:
the implementer and the verifier act on what the finding says and nothing
else.

---

# Packet P-0012

What follows is the content of packet P-0012, laid out for reading: the
information the brief above refers to.

## Identification

- Packet: `P-0012`, issued 2026-11-20T14:03:09Z under protocol `metron.protocol/v1`
- Packet digest: `sha256:efefefefefefefefefefefefefefefefefefefefefefefefefefefefefefefef`
- Action: `verify`, for the `verifier`, round 2
- Candidate: commit `c3e5a7b9d1f30246813579bdf02468ace1357902`, digest `sha256:3333333333333333333333333333333333333333333333333333333333333333`
- Reviewed commit: `9b2d4f6a8c0e1357924680ace13579bdf0246813`, digest `sha256:2222222222222222222222222222222222222222222222222222222222222222`
- Contract digest: `sha256:abababababababababababababababababababababababababababababababab`
- Read-only: yes, the candidate must not change

## Project

Courier.

Objectives:

- Deliver every accepted webhook event to every matching subscriber.
- Let subscribers filter the events they receive by type.

Non-objectives, which the project deliberately does not pursue:

- Exactly-once delivery.

## Stage and authorization

The contract targets the `pilot` stage. A GREEN gate authorizes
this activity: A four-week pilot with two internal teams as subscribers, on staging traffic only.

## Scope

- cmd/courier
- internal/delivery
- internal/store

## Operating envelope

Findings about behaviour outside these conditions cannot block.

- Up to 50 events per second, sustained.
- At most 200 subscribers.
- A single instance; restarts are allowed at any time.

## Applicable requirements

These are mandatory at the `pilot` stage.

### REQ-1 (`functional-correctness`)

Statement: Every accepted event is delivered at least once to every matching subscriber.

Criterion: The delivery test over 1000 events and 20 subscribers reports no missing delivery.

Check command: `go test ./internal/delivery -run TestAtLeastOnce`

No result is recorded for this candidate.

### REQ-2 (`objective-alignment`)

Statement: A subscriber receives only the event types it subscribed to.

Criterion: Filtering is applied before delivery for every subscription, as shown by the routing tests and by inspection.

No check command; the criterion is judged by inspection and evidence.

### REQ-3 (`architecture`)

Statement: Delivery, storage and the HTTP surface are separate packages with one-way dependencies.

Criterion: No import cycle, and internal/store imports neither of the other two.

No check command; the criterion is judged by inspection and evidence.

### REQ-4 (`data-integrity`, from `controlled-validation`)

Statement: An accepted event survives a crash of the process.

Criterion: Killing the process at each write boundary loses no accepted event.

Check command: `go test ./internal/store -run TestCrashRecovery`

No result is recorded for this candidate.

### REQ-5 (`testability`, from `controlled-validation`)

Statement: Delivery behaviour can be tested without a network.

Criterion: Every delivery test runs against the in-process transport.

No check command; the criterion is judged by inspection and evidence.

### REQ-6 (`observability`, from `controlled-validation`)

Statement: Every failed delivery attempt is logged with the event, the subscriber and the cause.

Criterion: The log of a run with injected failures has one structured entry per failed attempt.

No check command; the criterion is judged by inspection and evidence.

### REQ-7 (`stability`, from `controlled-validation`)

Statement: Memory use stays bounded under sustained load inside the envelope.

Criterion: Resident memory after one hour at 50 events per second is within 10% of that after five minutes.

No check command; the criterion is judged by inspection and evidence.

### REQ-8 (`reliability`, from `pilot`)

Statement: A subscriber that is down receives its events once it recovers.

Criterion: Retries with backoff continue for at least one hour, and the recovery test passes.

Check command: `go test ./internal/delivery -run TestSubscriberRecovery`

No result is recorded for this candidate.

### REQ-9 (`performance`, from `pilot`)

Statement: Delivery latency stays low at the envelope's load.

Criterion: The 99th percentile from acceptance to first attempt is under 500 ms at 50 events per second.

Check command: `go test ./internal/delivery -run TestLatencyBudget`

No result is recorded for this candidate.

### REQ-10 (`security-privacy`, from `pilot`)

Statement: Every delivery is signed so that a subscriber can authenticate it.

Criterion: Each request carries an HMAC-SHA256 signature over the body with the subscriber's secret.

No check command; the criterion is judged by inspection and evidence.

## Requirements from later stages

These are recorded in the contract but are not mandatory at the
`pilot` stage, and no finding can cite them yet.

- `REQ-11` (`scalability`, from `production`): Several instances can share the delivery load.
- `REQ-12` (`operational-readiness`, from `production`): A deployment can be rolled back without losing queued events.

## Risks

Unacceptable risks, which a blocking finding may cite:

- `RISK-1`: An event is delivered to a subscriber that did not subscribe to its type.
- `RISK-2`: A subscriber secret appears in logs or error messages.

Accepted risks, which governance has decided to live with and which never
block:

- `ACC-1`: A subscriber may receive the same event more than once.

## Waived domains

The contract declares these domains not applicable at this stage.

- `compatibility`: First release; there are no existing subscribers or stored data to stay compatible with.

## Calibrations

Provisional parameters that cannot be settled without evidence the project
does not have yet.

### CAL-1: Retry backoff starting at 2 s and capped at 5 min

Evidence needed: Recovery times of real subscribers during the pilot.

Reassess when: Two weeks of pilot traffic have been delivered.

## Convergence thresholds

- Root-cause analysis: required for a finding's next fix once its rejected fixes and reopenings reach 2.
- Convergence analysis: once per streak, when consecutive rounds without GREEN reach 3.
- Independent process audit: each time the count of consecutive rounds without GREEN reaches a multiple of 5.
- A streak of rounds without GREEN ends with a GREEN verdict, and its count restarts when a pin changes the stage.

## Claimed fixes to verify

### F-0004: An event acknowledged before its append is synced can be lost on a crash

- Tier `T1`, in `data-integrity`
- Status `claimed`; opened in round 1 by the reviewer; recurrences: 3
- Root-cause analysis required for its next fix
- Components: `internal/store/append.go` and `internal/delivery/accept.go`
- Requirement: `REQ-4`

Description: The HTTP handler acknowledges an event as soon as the append call returns, but the append returns before the sync completes.

Evidence: TestCrashRecovery with a kill between write and sync loses the acknowledged event in 3 of 10 runs.

Impact: An acknowledged event disappears after a restart, and the sender will not resend it.

Blocking rationale: Restarts are inside the envelope; the pilot's senders rely on acknowledgement.

Verification: TestCrashRecovery passes 100 consecutive runs with a kill at every write boundary.

Latest fix: Acknowledge only after the append is synced.

- Commits: `c3e5a7b9d1f30246813579bdf02468ace1357902`
- Components: `internal/delivery/accept.go` and `internal/store/append.go`

Root cause: Both endpoints treated the return of append as durability, and every earlier fix patched one acknowledgement path. Acknowledgement now goes through a single function that waits for the sync, and the retries added in P-0002 are removed.

Tests: TestCrashRecovery now kills the process at every write boundary on both endpoints; 100 runs pass.

History, oldest first:

- Round 1, `finding.opened` in packet `P-0001`, on candidate `sha256:1111111111111111111111111111111111111111111111111111111111111111`
- Round 1, `finding.fix-claimed` in packet `P-0002`, on candidate `sha256:6666666666666666666666666666666666666666666666666666666666666666`: Retry the append when the sync fails.
- Round 1, `finding.fix-rejected` in packet `P-0004`, on candidate `sha256:6666666666666666666666666666666666666666666666666666666666666666`: TestCrashRecovery still loses the acknowledged event in 2 of 10 runs with a kill between write and sync: the retry does not change when the acknowledgement is sent.
- Round 1, `finding.fix-claimed` in packet `P-0005`, on candidate `sha256:2222222222222222222222222222222222222222222222222222222222222222`: Sync before returning from append on the single-event endpoint.
- Round 1, `finding.fix-confirmed` in packet `P-0006`, on candidate `sha256:2222222222222222222222222222222222222222222222222222222222222222`: TestCrashRecovery passes 20 runs with a kill between write and sync.
- Round 2, `finding.reopened` in packet `P-0007`, on candidate `sha256:2222222222222222222222222222222222222222222222222222222222222222`: regression: the new batch endpoint acknowledges before the sync; TestCrashRecovery against /batch loses events in 4 of 10 runs.
- Round 2, `finding.fix-claimed` in packet `P-0008`, on candidate `sha256:4444444444444444444444444444444444444444444444444444444444444444`: Wait for the sync in the batch endpoint as well.
- Round 2, `finding.fix-rejected` in packet `P-0010`, on candidate `sha256:4444444444444444444444444444444444444444444444444444444444444444`: The refactoring in the same commit moved the single-event acknowledgement before the sync again; TestCrashRecovery loses 1 of 10.
- Round 2, `finding.fix-claimed` in packet `P-0011`, on this candidate: Acknowledge only after the append is synced.

### F-0005: internal/store imports internal/delivery for the event type

- Tier `T1`, in `architecture`
- Status `claimed`; opened in round 2 by the reviewer; recurrences: 0
- Disputed once; cannot be disputed again
- Components: `internal/store/event.go`
- Requirement: `REQ-3`

Description: The store depends on the delivery package for the Event type, which reverses the intended dependency direction.

Evidence: `go list -deps ./internal/store` includes internal/delivery.

Impact: Changes to delivery force rebuilding and retesting the store, and the store cannot be reused by the planned replay tool.

Blocking rationale: The pilot adds the replay tool, which needs the store without delivery.

Verification: `go list -deps ./internal/store` no longer lists internal/delivery.

Proportionality:

- Proposed change: Move the Event type to a new internal/event package used by both.
- Regression risk: Mechanical; the compiler finds every use.
- Deferral cost: Every package added during the pilot would deepen the inverted dependency.

Latest fix: Moved Event to internal/event.

- Commits: `c3e5a7b9d1f30246813579bdf02468ace1357902`
- Components: `internal/event`, `internal/store` and `internal/delivery`

History, oldest first:

- Round 2, `finding.opened` in packet `P-0007`, on candidate `sha256:2222222222222222222222222222222222222222222222222222222222222222`
- Round 2, `escalation.raised` in packet `P-0008`: the implementer disputes F-0005: the replay tool is not part of the pilot, so the dependency direction blocks nothing at this stage
- Round 2, `escalation.resolved` in packet `P-0009`, on candidate `sha256:4444444444444444444444444444444444444444444444444444444444444444`: Upheld: REQ-3 requires that internal/store import neither of the other packages, whether or not the replay tool ships.
- Round 2, `finding.fix-claimed` in packet `P-0011`, on this candidate: Moved Event to internal/event.

## Rounds of this stage

| Round | Commit | Verdict | Reviewer |
| --- | --- | --- | --- |
| 1 | `4a1c9e2b7d3f60812e5a9c4b7d1e3f2a6b8c0d94` | `RED` | review-agent |
| 2 | `9b2d4f6a8c0e1357924680ace13579bdf0246813` | `RED` | review-agent |

## Binding decisions

Audit directives and escalation resolutions recorded so far, oldest first.
They bind every role.

- A `resolution` of `E-0001` by `governance`: Dismissed: retention of delivered events is out of scope for the pilot; see the non-objectives.
- A `directive` by `auditor`, in packet `P-0003`: Durability findings are verified with TestCrashRecovery at every write boundary, not with the happy-path store tests.
- A `resolution` of `E-0002` by `auditor`, in packet `P-0003`: The latency budget holds at the envelope's load with a wide margin; F-0002 is an improvement, not a deficiency.
- A `resolution` of `E-0003` by `auditor`, in packet `P-0009`: Upheld: REQ-3 requires that internal/store import neither of the other packages, whether or not the replay tool ships.
- A `resolution` of `E-0004` by `auditor`, in packet `P-0009`: Withdrawn: the only caller passes keys validated by NewSigner, and HMAC signing cannot fail for a valid key.

---

# Report skeleton

This skeleton already has the right shape for packet P-0012. Fill in every
empty value as the brief describes, add the entries the brief allows, and
remove nothing that is required. Metron rejects a report that still has an
empty value where one is required.

```json
{
  "schema": "metron.report/v1",
  "packet": "P-0012",
  "packet_digest": "sha256:efefefefefefefefefefefefefefefefefefefefefefefefefefefefefefefef",
  "role": "verifier",
  "author": {
    "id": "",
    "kind": ""
  },
  "body": {
    "verifications": [
      {
        "finding": "F-0004",
        "result": "",
        "evidence": ""
      },
      {
        "finding": "F-0005",
        "result": "",
        "evidence": ""
      }
    ]
  }
}
```

# Handing the report back

Write the skeleton to a file, fill it in, and submit it:

```
metron packet --format skeleton -o .metron/work/report.json
metron ingest .metron/work/report.json
```

Keep the report under `.metron/work/` or outside the repository; anywhere
else in the repository it makes the tree unclean, and the report is refused.

If you cannot run commands, return the report as a single JSON document and
nothing else; whoever operates Metron will submit it. Metron accepts or
rejects the report as a whole. A rejection says what is wrong and changes
nothing, so the report can be corrected and submitted again; the packet stays
open until a report for it is accepted.
