# Auditor brief for packet P-0012

## Your situation

You are the auditor of Courier, in round 5 of the `pilot` stage.
The current candidate is this commit:

- commit `e7f9a1b3c5d70246813579bdf02468ace1357924`
- candidate digest `sha256:5555555555555555555555555555555555555555555555555555555555555555`

The review is governed by Metron, a program that runs a loop of review, fix
and verification. Metron makes no engineering judgment, but it watches the
loop for signs that it has stopped converging or reached a disagreement it
cannot settle by itself, and when it sees one it raises an escalation. The
loop is halted until the open escalations are discharged, and this packet
holds the ones that are yours:
`E-0006`, `E-0007`, `E-0008`, `E-0009`, `E-0010`.

Your job is to work out what is actually going wrong and to set the loop on a
course that can end in a sound decision. That is not the same as ending it.
You do not lower the standard to finish a loop: a finding does not become less
real because it has been argued over for several rounds, and a candidate does
not become ready because everyone is tired of it. Nor do you raise the
standard: you work within the acceptance contract as governance pinned it.

Everything this brief refers to is in the packet: the contract, the
escalations (`escalations`), every finding in the ledger with its history
(`findings`), the latest check results (`evidence`), the rounds of
this stage (`history`) and the binding decisions so far (`decisions`). If you
received this brief as a prompt, the same information follows it, laid out
for reading.

## The escalations in this packet

Each kind of escalation asks a different question.

### `dispute`: `E-0006`

An implementer disputes a finding instead of fixing it; the escalation names
the finding, and its reason carries the implementer's rationale. The question
is whether the finding, as recorded, meets the bar of its tier. Do not simply
choose between the reviewer's account and the implementer's: re-examine the
finding's evidence yourself, on the candidate. Uphold the finding if it
stands; it must then be fixed and cannot be disputed again. Withdraw it if it
is wrong: its mechanism does not occur, its evidence does not show what it
claims, it concerns behaviour outside the operating envelope, or it violates
nothing it cites. Reclassify it if the problem is real but the tier is not
supported, for example a finding that is a genuine improvement but violates
no applicable requirement and realizes no unacceptable risk. If the finding
points at a real risk the contract fails to cover, reclassifying is the most
you can do with the finding; recommend the contract change as well.

### `contradiction`: `E-0007`

A new finding declares that it reverses an earlier resolution: fixing it
would undo an earlier fix, or it raises again a concern an earlier audit
withdrew. The escalation names the new finding, and the finding names the
earlier one in its `contradicts` member. Two parts of the process disagree
about what is correct, and unless someone decides, the code will oscillate
between the two. Decide which is right under the contract and the evidence,
after examining both yourself. The new finding is adjudicable: uphold it if
the earlier resolution was wrong, withdraw it if the earlier resolution
stands, or reclassify it if it has merit but does not justify its tier. Then
settle the underlying question for later rounds with a directive, so that the
next reviewer does not reopen it from the same starting point.

### `component-churn`: `E-0008`

The same component has been fixed in 3 distinct rounds; the
escalation names it. Repeated local fixes to one component usually mean the trouble lies in the
component itself, in its design, its responsibilities or its interfaces, or
in the way findings against it are being framed. Audit the component as a
whole: read its findings and fixes in the ledger and the changes made to it,
and establish whether the findings share a cause, whether the component
needs redesign rather than further patching, and whether the findings are
consistent with each other and with the contract. Direct the approach that
further fixes to it must take.

### `regression-spiral`: `E-0009`

A verification found at least one blocking regression and no fewer
regressions than the fixes it confirmed: the fixes are breaking things at
least as fast as they repair them, and incremental patching has stopped
working. Assess whether it should be suspended in favour of redesigning the
affected area. Look at the regressions, the fixes that caused them, and the
coupling that lets one change break another. Your conclusion says whether
patching may continue and on what conditions, or what must be redesigned
first; a directive makes that binding.

### `process-audit`: `E-0010`

The number of consecutive rounds that ended without a GREEN verdict has
reached a multiple of 5: the earlier
safeguards, including any convergence analysis and its directives, have not
brought the loop to a decision. Audit the process itself, independently of
earlier audits: whether the contract is fit for this stage, whether each role
is doing its job as the protocol defines it, whether earlier directives were
followed and whether they helped, and whether the stage the contract targets
is the right one. Do not take an earlier audit's conclusions as given; check
them.

## Adjudication

You must decide on exactly these findings, and on no other:
`F-0002` and `F-0009`. For each one, give one decision:

- `uphold`: the finding stands as recorded. A blocking finding must then be
  fixed.
- `withdraw`: the finding is wrong and leaves the process for good.
- `reclassify`: the problem is real but its tier is not; give the tier the
  evidence supports, which must differ from the current one.

Judge a finding by the standard its tier requires. A blocking finding must
cite an applicable requirement or an unacceptable risk that it actually
violates or realizes, inside the operating envelope, with evidence that holds
up when you check it yourself. A `T2` needs a real benefit, a `T3` a question
that genuinely cannot be settled yet. Where the finding's domain is
`optimization`, `performance`, `scalability`, `architecture`,
`maintainability`, `technical-debt`, `simplicity` or `cost-efficiency`, a
blocking tier also requires a convincing case for proportionality: the
smallest change that resolves it, its regression risk, and why deferring it
would cost substantially more. Uncertainty about a mandatory requirement does
not make a finding a calibration.

## Directives and contract recommendations

Directives are binding guidance. Metron carries them into every later packet,
and every role must follow them. Use them to set the course the loop must
take: an investigation that has to happen before more patching, an approach
further fixes must follow, or the settled answer to a question that keeps
being re-argued. Write each directive so that someone without your context can
act on it and someone else can tell whether it was followed: specific about
what it covers, and consistent with the contract. A directive cannot change
the contract, lower or raise the standard, or decide a future review in
advance. Issue only the directives you need; each one constrains every round
that follows.

When the root of the problem lies in the contract, such as an ambiguous
criterion, a missing requirement, an unrealistic envelope or a requirement
that cannot be verified at this stage, recommend the change in
`contract_recommendations`. Governance decides; a recommendation changes
nothing by itself.

## How to investigate

You have the whole ledger: every finding with its status, recurrences and
latest fix, every earlier round and its verdict, and every decision so far.
Use the repository's history as well, since the commits between rounds show
what each fix actually changed. Base your conclusions on evidence you can
point to, as a reviewer must, and say where the evidence is incomplete.

## Leave the candidate untouched

This role is read-only. Do not edit, create or delete files in the
repository, commit, stash, switch branches or check out another revision,
and do not touch `.metron/`. When you submit your report, Metron compares
the candidate digest and the cleanliness of the working tree with their
state when this packet was issued, and rejects the report if either has
changed. Building the code and running tests is fine as long as nothing is
left behind that `git status` would show: send output to a directory outside
the repository or to paths the repository already ignores. To look at
another revision, use `git show` or a separate clone. Write your report file
outside the repository too.

## Writing the report

The report is a single JSON document. Its envelope says which packet it
answers and who wrote it:

| Member | Content |
| --- | --- |
| `schema` | `metron.report/v1` |
| `packet` | `P-0012`, the packet you are answering |
| `role` | `auditor` |
| `author` | Who you are. `id` is a stable name for whoever acts in this role; Metron enforces role separation on it. `kind` is `agent` or `human`. An agent should also give `model`, the model identifier, and may give `harness`, the tool that ran it, and `session`, an identifier that lets someone find the session's record later. |
| `body` | Your answer, described below |

Your author `id` must never have been used by an implementer, a reviewer or a
verifier in this ledger.

The body has these members:

| Member | Content |
| --- | --- |
| `analysis` | Your investigation of every escalation in this packet, each addressed by its identifier: what you examined, what you found, and the evidence for it. |
| `conclusion` | For each escalation, by its identifier, what you concluded and what the loop must do next. When Metron accepts the report, it discharges every escalation in this packet and records this conclusion as each one's resolution. |
| `adjudications` | One entry per adjudicable finding, with `finding` (its `F-` identifier), `decision` (`uphold`, `withdraw` or `reclassify`), `tier` (the new tier; present for, and only for, `reclassify`) and `rationale` (why, with the evidence you checked). Left out when there is nothing to adjudicate. |
| `directives` | Optional. Binding guidance for every later packet, one statement per entry. |
| `contract_recommendations` | Optional. Changes to the contract you recommend to governance, one per entry, each with its reason. |

---

# Packet P-0012

What follows is the content of packet P-0012, laid out for reading: the
information the brief above refers to.

## Identification

- Packet: `P-0012`, issued 2026-11-20T14:03:09Z under protocol `metron.protocol/v1`
- Action: `audit`, for the `auditor`, in round 5 of the stage
- Candidate: commit `e7f9a1b3c5d70246813579bdf02468ace1357924`, digest `sha256:5555555555555555555555555555555555555555555555555555555555555555`
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

Latest result on this candidate: passed, but for a different command text, `go test ./internal/store -run TestCrashRecovery -count 1`, so it does not count for this requirement.

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

## Escalations to discharge

### E-0006: `dispute`, raised in round 5

the implementer disputes F-0002: the envelope's load is an eighth of the measured capacity, so REQ-9 is not at risk.

- Findings: `F-0002`

### E-0007: `contradiction`, raised in round 5

F-0009 declares that it reverses the resolution of F-0001

- Findings: `F-0009`

### E-0008: `component-churn`, raised in round 5

internal/delivery/retry.go was fixed in rounds 1, 3 and 4.

- Components: `internal/delivery/retry.go`

### E-0009: `regression-spiral`, raised in round 4

The verification of round 4 confirmed one fix and found two blocking regressions.

### E-0010: `process-audit`, raised in round 5

Five consecutive rounds have ended without GREEN.

- Requirements: `REQ-7` and `REQ-8`

## Findings to adjudicate

### F-0002: Each accepted event is written with its own fsync

- Tier `T1`, in `performance`
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

### F-0009: Retry state for a dead subscriber grows for an hour

- Tier `T1`, in `stability`
- Status `open`; opened in round 5 by the reviewer; recurrences: 0
- Components: `internal/delivery/retry.go`
- Requirement: `REQ-7`
- Would reverse the resolution of `F-0001`

Description: Retrying for one hour keeps every pending event of a dead subscriber in memory for that hour.

Evidence: At 50 events per second for one subscriber that never recovers, resident memory grows by 600 MB in an hour.

Impact: Memory is not bounded under sustained load with one dead subscriber, which is inside the envelope.

Blocking rationale: A dead subscriber during the pilot would exhaust the instance's memory.

Verification: The REQ-7 soak test passes with one subscriber that never recovers.

## Other findings in the ledger

### F-0001: Retries stop after the fifth failed attempt

- Tier `T1`, in `reliability`
- Status `resolved`; opened in round 1 by the reviewer; recurrences: 0
- Components: `internal/delivery/retry.go`
- Requirement: `REQ-8`

Description: The retry loop counts attempts instead of elapsed time and gives up after five, about 62 s with the current backoff.

Evidence: TestSubscriberRecovery with a 10-minute outage fails: no delivery after recovery.

Impact: Any subscriber outage longer than about a minute loses events, which pilot subscribers will hit.

Blocking rationale: The pilot depends on subscribers that restart during deployments.

Verification: TestSubscriberRecovery passes with a one-hour outage.

Latest fix: Retry until one hour has elapsed since the first attempt.

- Commits: `9b2d4f6a8c0e1357924680ace13579bdf0246813`
- Components: `internal/delivery/retry.go`

Tests: Extended TestSubscriberRecovery to a one-hour simulated outage.

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

## Rounds of this stage

| Round | Commit | Verdict | Reviewer |
| --- | --- | --- | --- |
| 1 | `4a1c9e2b7d3f60812e5a9c4b7d1e3f2a6b8c0d94` | `RED` | review-agent |
| 2 | `9b2d4f6a8c0e1357924680ace13579bdf0246813` | `RED` | review-agent |
| 3 | `c3e5a7b9d1f30246813579bdf02468ace1357902` | `INDETERMINATE` | review-agent |
| 4 | `c3e5a7b9d1f30246813579bdf02468ace1357902` | none | none |
| 5 | `e7f9a1b3c5d70246813579bdf02468ace1357924` | `RED` | review-agent |

## Binding decisions

- `directive` by `auditor`: Durability findings are verified with TestCrashRecovery at every write boundary, not with the happy-path store tests.
- `resolution` by `governance`, from `E-0002`: Dismissed: retention of delivered events is out of scope for the pilot; see the non-objectives.

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
  "role": "auditor",
  "author": {
    "id": "",
    "kind": ""
  },
  "body": {
    "analysis": "",
    "conclusion": "",
    "adjudications": [
      {
        "finding": "F-0002",
        "decision": "",
        "rationale": ""
      },
      {
        "finding": "F-0009",
        "decision": "",
        "rationale": ""
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
