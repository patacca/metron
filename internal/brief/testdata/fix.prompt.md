# Implementer brief for packet P-0012

## Your situation

You are the implementer of Courier, in round 2 of the `pilot` stage.
This round's review examined the commit below and found problems that
must be resolved before a candidate can be approved for the `pilot`
stage:

- reviewed commit `9b2d4f6a8c0e1357924680ace13579bdf0246813`

Your work goes on top of that commit.

The review is governed by Metron, a program that runs the loop around you. It
issued this packet, will validate your report and records your fixes in its
ledger. After you, a verifier who is not you checks each fix, and a new,
complete review of the resulting candidate follows. Governance owns the
acceptance contract, and an auditor adjudicates disputes.

Everything this brief refers to is in the packet: the contract, the findings
you work on with their evidence (`findings`), the latest check results
(`evidence`), the rounds of this stage (`history`) and the binding decisions
(`decisions`). If you received this brief as a prompt, the same information
follows it, laid out for reading.

## What you are asked to do

Resolve each of these blocking findings, by fixing it or, if you believe it
is wrong, by disputing it: `F-0004` and `F-0005`. An auditor has already adjudicated a dispute of
`F-0005`, and a finding can be disputed only once, so that one has to be fixed.

The packet also lists open non-blocking findings as an optional backlog:
`F-0002`, `F-0003` and `F-0006`. You may fix any of them in the same batch when it is cheap and
safe to do so; nothing requires it, and a backlog fix is verified like any
other.

## Fix the batch as a batch

Read every finding before you change anything. Findings that touch the same
component, describe similar mechanisms or name each other in `related` often
have a common cause, and one change at the cause is better than several
patches at the symptoms: it is smaller, it is easier to verify, and it does
not leave the cause in place to produce the next finding. Where findings do
have separate causes, fix them separately; merging unrelated changes makes
each harder to verify.

Root-cause analysis is required for `F-0004`. A finding is flagged this
way once its rejected fixes and reopenings add up to 2, which
means the earlier fixes treated a symptom, so another local patch is not
acceptable for it. Find out why the defect occurs and why the earlier fixes
did not hold, change that, and explain both in the fix's `root_cause`; the
verifier will check that the fix addresses the cause you identified.

Each finding says how a fix can be verified, in its `verification` member.
That is what the verifier will apply, so run it yourself before you submit.
A regression test that fails before your change and passes after it is the
strongest evidence you can give; describe what you added or ran in `tests`.

Keep each change proportionate to its finding. Every change you make is
verified and then reviewed again in full, and over-engineering is reviewable:
an unrequested refactoring or a speculative abstraction widens what can go
wrong and can become a finding of its own.

## What a fix must not do

Do not weaken a check, a test or the contract to make a finding go away. That
includes deleting or skipping tests, loosening assertions, narrowing what a
test or a check command exercises, and special-casing the conditions a
finding's evidence uses. Such a change makes the evidence pass while the
defect remains, so the gate would report something false; the verifier will
reject it.

Do not touch `.metron/`. The contract belongs to governance and the state
directory to Metron. A change there is never a fix: commits that change
nothing outside `.metron/` do not change the candidate, and editing the state
directory corrupts the ledger.

## Disputing a finding

If you believe a finding is wrong, dispute it instead of fixing it. A finding
is wrong when its mechanism does not occur, when its evidence does not show
what it claims, when it describes behaviour outside the operating envelope,
when it does not actually violate the requirement or realize the
unacceptable risk it cites, or when the evidence does not support its tier.
The rationale must engage with the finding's own evidence and bring yours: an
auditor will decide between the two, and will check both.

A dispute is not a way to avoid work. Only the blocking findings this packet
asks you to resolve can be disputed, and each only once. The auditor upholds
the finding, withdraws it, or reclassifies it to another tier; an upheld
finding must be fixed and cannot be disputed again. A dispute without
substance costs a full audit and comes back upheld, with the work still owed.

## Committing

Commit everything. When you submit, the working tree must be clean:
`git status --porcelain` shows nothing outside `.metron/`, untracked files
included. Write your report file outside the repository for the same reason.

List each fix's commits by their full hash, as `git rev-parse` prints it.
Every listed commit must be one you added after the reviewed commit:
reachable from the commit you submit, and not from the reviewed one. A commit
that contains changes for several findings is listed under each of them. A
report with at least one fix must change the candidate.

## Writing the report

The report is a single JSON document. Its envelope says which packet it
answers and who wrote it:

| Member | Content |
| --- | --- |
| `schema` | `metron.report/v1` |
| `packet` | `P-0012`, the packet you are answering |
| `role` | `implementer` |
| `author` | Who you are. `id` is a stable name for whoever acts in this role; Metron enforces role separation on it. `kind` is `agent` or `human`. An agent should also give `model`, the model identifier, and may give `harness`, the tool that ran it, and `session`, an identifier that lets someone find the session's record later. |
| `body` | Your answer, described below |

Your author `id` must never have been used by a reviewer, a verifier or an
auditor in this ledger.

The body has these members. Every finding listed above as one to resolve
appears in exactly one of `fixes` and `disputes`.

| Member | Content |
| --- | --- |
| `fixes` | One entry per finding you fixed, including any from the backlog. The list is empty if you only dispute. |
| `disputes` | Optional. One entry per finding you dispute, with `finding` (its `F-` identifier) and `rationale`: why the finding is wrong, with your evidence. |
| `notes` | Optional. Anything the verifier should know that belongs to no single fix, such as a shared cause behind several findings. |

Each entry in `fixes` has these members:

| Member | Content |
| --- | --- |
| `finding` | The `F-` identifier of the finding fixed. |
| `commits` | The full hashes of the commits that contain the fix; at least one. |
| `summary` | What you changed and why that removes the mechanism the finding describes. |
| `components` | The files, modules or subsystems the fix touched; at least one. |
| `root_cause` | Required for a finding flagged root-cause-required, and welcome for any other: why the defect occurred, why any earlier fix did not hold, and how this fix addresses the cause. |
| `tests` | Optional. The regression tests you added and the checks you ran, with their outcome. |

---

# Packet P-0012

What follows is the content of packet P-0012, laid out for reading: the
information the brief above refers to.

## Identification

- Packet: `P-0012`, issued 2026-11-20T14:03:09Z under protocol `metron.protocol/v1`
- Action: `fix`, for the `implementer`, in round 2 of the stage
- Candidate: commit `9b2d4f6a8c0e1357924680ace13579bdf0246813`, digest `sha256:2222222222222222222222222222222222222222222222222222222222222222`
- Reviewed commit: `9b2d4f6a8c0e1357924680ace13579bdf0246813`, digest `sha256:2222222222222222222222222222222222222222222222222222222222222222`
- Contract digest: `sha256:abababababababababababababababababababababababababababababababab`
- Read-only: no, you are expected to commit changes

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

Latest result on this candidate: passed, exit status 0, output digest `sha256:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd`.

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

Latest result on this candidate: passed, exit status 0, output digest `sha256:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd`.

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

Latest result on this candidate: passed, exit status 0, output digest `sha256:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd`.

### REQ-9 (`performance`, from `pilot`)

Statement: Delivery latency stays low at the envelope's load.

Criterion: The 99th percentile from acceptance to first attempt is under 500 ms at 50 events per second.

Check command: `go test ./internal/delivery -run TestLatencyBudget`

Latest result on this candidate: passed, exit status 0, output digest `sha256:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd`.

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
- Convergence analysis: once 3 consecutive rounds have ended without GREEN.
- Independent process audit: each time the count of consecutive rounds without GREEN reaches a multiple of 5.

## Findings to fix or dispute

### F-0004: An event acknowledged before its append is synced can be lost on a crash

- Tier `T1`, in `data-integrity`
- Status `open`; opened in round 1 by the reviewer; recurrences: 2
- Root-cause analysis required for its next fix
- Components: `internal/store/append.go` and `internal/delivery/accept.go`
- Requirement: `REQ-4`

Description: The HTTP handler acknowledges an event as soon as the append call returns, but the append returns before the sync completes.

Evidence: TestCrashRecovery with a kill between write and sync loses the acknowledged event in 3 of 10 runs.

Impact: An acknowledged event disappears after a restart, and the sender will not resend it.

Blocking rationale: Restarts are inside the envelope; the pilot's senders rely on acknowledgement.

Verification: TestCrashRecovery passes 100 consecutive runs with a kill at every write boundary.

### F-0005: internal/store imports internal/delivery for the event type

- Tier `T1`, in `architecture`
- Status `open`; opened in round 2 by the reviewer; recurrences: 0
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

## Optional backlog

Open non-blocking findings you may also fix.

### F-0002: Each accepted event is written with its own fsync

- Tier `T2`, reclassified from `T1`, in `performance`
- Status `open`; opened in round 1 by the reviewer; recurrences: 0
- Disputed once; cannot be disputed again
- Components: `internal/store/append.go`
- Requirement: `REQ-9`

Description: Appends are synced one at a time, which caps throughput at the disk's sync rate.

Evidence: A benchmark on the reference machine reaches 400 appends per second.

Impact: Throughput headroom above the envelope's 50 events per second is smaller than it could be.

Blocking rationale: Latency could exceed the budget under bursts.

Verification: TestLatencyBudget passes at 200 events per second.

Proportionality:

- Proposed change: Group appends that arrive within 5 ms into one sync.
- Regression risk: Group commit changes the durability point of each event.
- Deferral cost: None identified beyond the pilot.

Expected benefit: Roughly eight times the append throughput.

Deferral rationale: The envelope's load is an eighth of the measured capacity.

### F-0003: The 5-minute backoff cap may delay recovery for long outages

- Tier `T3`, in `reliability`
- Status `open`; opened in round 1 by the reviewer; recurrences: 0
- Components: `internal/delivery/retry.go`
- Related: `F-0001`

Description: After a long outage a recovered subscriber may wait up to five minutes for the next attempt.

Evidence: Follows from the backoff schedule in retry.go.

Impact: Slower catch-up after outages longer than ten minutes.

Evidence needed: Outage durations of pilot subscribers.

Reassess when: Two weeks of pilot traffic have been delivered.

### F-0006: Constructor takes seven positional parameters

- Tier `T4`, in `simplicity`
- Status `open`; opened in round 2 by the reviewer; recurrences: 0
- Components: `internal/delivery/dispatcher.go`

Description: An options struct would make call sites easier to read.

Evidence: dispatcher.go, NewDispatcher.

Impact: Readability only.

## Rounds of this stage

| Round | Commit | Verdict | Reviewer |
| --- | --- | --- | --- |
| 1 | `4a1c9e2b7d3f60812e5a9c4b7d1e3f2a6b8c0d94` | `RED` | review-agent |
| 2 | `9b2d4f6a8c0e1357924680ace13579bdf0246813` | `RED` | review-agent |

## Binding decisions

- `directive` by `auditor`: Durability findings are verified with TestCrashRecovery at every write boundary, not with the happy-path store tests.

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
  "role": "implementer",
  "author": {
    "id": "",
    "kind": ""
  },
  "body": {
    "fixes": [
      {
        "finding": "F-0004",
        "commits": [],
        "summary": "",
        "components": [],
        "root_cause": ""
      },
      {
        "finding": "F-0005",
        "commits": [],
        "summary": "",
        "components": []
      }
    ]
  }
}
```

# Handing the report back

Save the completed report as a file outside the repository and submit it:

```
metron ingest <file>
```

If you cannot run commands, return the report as a single JSON document and
nothing else; whoever operates Metron will submit it. Metron accepts or
rejects the report as a whole. A rejection says what is wrong and changes
nothing, so the report can be corrected and submitted again; the packet stays
open until a report for it is accepted.
