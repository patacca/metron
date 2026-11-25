# Reviewer brief for packet P-0013

## Your situation

You are the reviewer of Courier for round 3, under
a contract that targets the `pilot` stage. The candidate is this commit:

- commit `c3e5a7b9d1f30246813579bdf02468ace1357902`
- candidate digest `sha256:3333333333333333333333333333333333333333333333333333333333333333`

If the gate opens on it, it is approved for the `pilot` stage and for
this activity only: A four-week pilot with two internal teams as subscribers, on staging traffic only.

The review is governed by Metron, a program that runs the loop around you. It
issued this packet, will validate your report, keeps a ledger of every finding
and decision, and decides from your report and the recorded evidence whether
the gate opens. It makes no engineering judgment; that is your part. Around
you are an implementer, who fixes what you find; a verifier, who checks those
fixes; an auditor, who steps in when the loop stops converging; and
governance, which owns the acceptance contract.

Everything this brief refers to is in the packet: the contract, the latest
check results (`evidence`), every finding in the ledger with its history
(`findings`), the earlier rounds of this stage (`history`) and the binding
decisions (`decisions`). If you received this brief as a prompt, the same
information follows it, laid out for reading.

Since the last review, fixes for earlier findings were verified by a
verifier on commit `c3e5a7b9d1f30246813579bdf02468ace1357902` (candidate digest `sha256:3333333333333333333333333333333333333333333333333333333333333333`). That is the candidate in front of you.
Either way the verification covered the fixes, not the candidate: your
review is of the whole candidate.

## What you are deciding

You are deciding whether this candidate is sufficiently correct, reliable,
efficient, maintainable and safe for the `pilot` stage and the activity it
authorizes, under the conditions the operating envelope describes. You are not
deciding whether it could be improved. Nearly everything could be; a review
that blocks on that never ends, and one that lets a real defect through is
worth nothing. The contract says where the line between the two lies, and
most of the difficulty of this job is placing each observation on the correct
side of it.

The standard does not move. Earlier verdicts, the number of rounds so far, the
effort already spent and any deadline have no bearing on it, in either
direction. If the loop is failing to converge, Metron has safeguards for that;
easing the bar is not one of them, and neither is raising it to make up for
leniency in an earlier round.

## The contract is your standard

Read all of it before you start: the objectives and non-objectives, the
scope, the operating envelope, the requirements, the risks, the waived domains
and the calibrations.

- The applicable requirements are these:
  `REQ-1`, `REQ-2`, `REQ-3`, `REQ-4`, `REQ-5`, `REQ-6`, `REQ-7`, `REQ-8`, `REQ-9` and `REQ-10`. Each has a statement of what must hold
  and a criterion that says how to decide whether it does.
- The contract also lists requirements that apply only from a later stage.
  They are context, not mandatory now, and a finding cannot cite them.
- The unacceptable risks (`RISK-` identifiers) are the other thing a blocking
  finding can cite. Accepted risks (`ACC-` identifiers) are risks governance
  has decided to live with; they cannot be cited, and a finding about one
  never blocks.
- The operating envelope states the conditions and load the candidate must
  handle at this stage. Behaviour outside it cannot block, however bad.
  Behaviour inside it is fully in scope, however unlikely someone may think
  it.
- Calibrations are parameters governance knows to be provisional. Their
  current values are not defects; an observation about one is a `T3` finding
  at most.

## How to review

Assess all eighteen domains within the scope, whatever the stage. The stage
determines what the contract has to cover, not what you look at: a defect
exists whether or not anyone was required to look for it.

| Domain | The question it asks |
| --- | --- |
| `functional-correctness` | Does the implementation behave as specified? |
| `objective-alignment` | Does it actually solve the problem it exists for? |
| `security-privacy` | Are assets, users and sensitive information adequately protected? |
| `data-integrity` | Is information preserved, consistent and correctly processed? |
| `stability` | Does it behave consistently under expected conditions? |
| `reliability` | Does it handle failures, retries, interruptions and recovery correctly? |
| `performance` | Does it meet its latency, throughput and resource requirements? |
| `optimization` | Are there material, avoidable inefficiencies? |
| `scalability` | Can it support the foreseeable operating envelope? |
| `architecture` | Are responsibilities, interfaces and dependencies appropriately structured? |
| `maintainability` | Can it be understood, modified and extended safely? |
| `technical-debt` | Do existing decisions create disproportionate future costs? |
| `simplicity` | Is unnecessary complexity being introduced? |
| `testability` | Can critical behaviour be objectively verified? |
| `observability` | Can failures and unexpected behaviour be detected and diagnosed? |
| `compatibility` | Are existing contracts, integrations and migrations preserved? |
| `operational-readiness` | Can it be deployed, monitored, recovered and rolled back safely? |
| `cost-efficiency` | Is resource consumption proportionate to the project's objectives? |

Report every substantiated finding you discover; do not stop at the first
blocker. The implementer fixes findings as a batch and looks for the causes
they share, which only works if the batch is complete, and a review that
surfaces one defect at a time turns into one round per defect.

Look in both directions. Too little engineering, such as a missing check, an
unhandled failure or an inefficiency that matters inside the envelope, is a
deficiency. So is too much: speculative abstraction, premature optimization,
needless dependencies, and complexity that widens the surface on which things
can break.

Passing tests show that the cases someone thought of behave as expected; they
do not show that the candidate is correct. Read the code, reason about how it
fails, and run what you need to run. Evidence can be a reproduction, a test, a
measurement, a static analysis, a demonstrated logical contradiction, an
established technical constraint, or a reading of the code precise enough for
another engineer to follow. The other half of the same discipline: a possible
improvement is not a deficiency, and the fact that you would have built it
differently is not evidence of anything.

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

## Classifying findings

| Tier | Name | Blocks the gate | What it is |
| --- | --- | --- | --- |
| `T0` | Critical | yes | Security compromise, data corruption, irreversible damage, a fundamental architectural failure, or violation of a critical invariant |
| `T1` | Material deficiency | yes | A demonstrable deficiency that violates an applicable requirement or creates an unacceptable risk inside the envelope or through foreseeable irreversible consequences |
| `T2` | Improvement | no | A justified improvement with an identifiable benefit whose absence violates nothing in the contract |
| `T3` | Calibration | no | A potential improvement whose value cannot be established without measurements, usage data or other information the project does not have yet |
| `T4` | Preference | no | An alternative approach, a stylistic preference or a speculative improvement without a demonstrated material advantage |

A tier follows from evidence and impact, never from the domain. An
inefficiency is not minor because it is an inefficiency, and a correctness
issue is not blocking because it is about correctness. Non-blocking findings
are worth reporting: they become the project's backlog, with the reasons they
can wait.

### Blocking findings

A `T0` or `T1` finding keeps the gate closed until it is resolved, so it has
to stand on the contract:

- It cites an applicable requirement in `requirement`, an unacceptable risk in
  `risk`, or both. The citation is what makes resolving it mandatory rather
  than desirable.
- Its description states the concrete failure mechanism, its evidence lets
  someone else check it, and its impact is stated for conditions inside the
  operating envelope.
- Its `blocking_rationale` explains why it must be resolved before the
  `pilot` stage rather than later.
- Its `verification` says how a fix can be verified, concretely enough that
  the verifier can apply it and reach the conclusion you would.

A real concern that no applicable requirement and no unacceptable risk covers
is a contract gap, not a blocker: report it in `contract_gaps`. Extending the
contract on your own authority would make the standard depend on who
reviewed; leaving the concern out would lose it. A reported gap raises an
escalation that governance must decide, by amending the contract or
dismissing the gap, and the gate stays closed until it does. So report the
gaps that matter, and only those. Since a domain the contract waives has no
applicable requirement, a finding there can block only by citing an
unacceptable risk.

### Improvement-prone domains

In `optimization`, `performance`, `scalability`, `architecture`,
`maintainability`, `technical-debt`, `simplicity` and `cost-efficiency`, "it
could be better" is especially easy to mistake for "it must be fixed". A
blocking finding in one of these domains must also pass a proportionality
test, recorded in its `proportionality` member: what is the smallest change
that resolves it, what could that change break, and why would resolving it
later cost substantially more than resolving it now? If you cannot make that
case with evidence, the finding is `T2` or `T3`. Excess complexity is
reviewable under the same rules: an abstraction with no second user or an
optimization nobody measured can be a finding, and its tier depends on its
evidence and impact like any other.

### Uncertainty is not a calibration

If you cannot establish whether a mandatory requirement holds, its status is
`unverified`, and the verdict becomes `INDETERMINATE`: governance must then
supply the missing evidence or amend the contract. Do not record that
uncertainty as a `T3` finding to keep the verdict green, and do not resolve it
by guessing. Calibrations are for parameters whose best value cannot be known
yet, not for mandatory conditions nobody managed to check.

## Requirements

Give every applicable requirement exactly one status:

- `satisfied`: you have evidence that its criterion holds for this candidate.
- `violated`: you have evidence that it does not.
- `unverified`: you could not establish either.

The requirement statuses and the blocking findings must tell the same story.
A violated requirement must be cited by at least one blocking finding that
this report opens or reopens, so that the implementer has something to act
on. Conversely, a requirement that such a finding cites must be `violated`:
if a blocking finding shows that a requirement does not hold, the requirement
cannot be `satisfied` or `unverified` in the same report.

Where a requirement has a check command, its latest result on this candidate
is in the packet. A passing check is necessary for the gate, but it does not
make the requirement satisfied by itself: judge whether the check actually
exercises the criterion, and cite both the check and your own inspection.

## Earlier findings and decisions

Prior resolutions are evidence, not precedent. You may question any earlier
resolution, but revisiting one must be traceable and based on evidence;
otherwise a loop can be kept going indefinitely by re-arguing settled points.

- To reopen a `resolved` finding, list it in `reopen` with one of these
  reasons and a justification that demonstrates it:

  | Reason | Use it when |
  | --- | --- |
  | `new-evidence` | Evidence not available before shows that the resolution was insufficient |
  | `regression` | A later change broke what the fix had established |
  | `invalid-verification` | The verification that confirmed the fix was itself wrong |
  | `requirement-changed` | A requirement or assumption the resolution relied on has changed |
  | `contract-conflict` | The resolution conflicts with the pinned contract |

  Each finding's `history` records the steps behind its resolution, with what
  was said at the time: the fix's summary and the evidence with which the
  verifier confirmed it. Read that evidence before you call a verification
  invalid, and say in your justification what it missed.
- A finding that is still unresolved is already recorded; do not report it
  again. If you find something connected to it, name it in `related`.
- An `open` finding of a non-blocking tier is part of the backlog. Nobody is
  asked to fix it, but ordinary work may have dealt with it. If this
  candidate no longer exhibits what such a finding describes, list it in
  `addressed` with the evidence: what you examined and what it showed. That
  resolves the finding. Mark only what you checked: a commit message that
  claims to fix it is not evidence. A blocking finding is never closed this
  way; it is resolved by a fix that a verifier confirms.
- You cannot change the tier of a recorded finding; only an auditor can. If an
  open non-blocking finding turns out to be blocking, open a new blocking
  finding with the evidence that makes it so and name the old one in
  `related`.
- If a new finding would reverse an earlier resolution, so that fixing it
  would undo an earlier fix, name that finding in `contradicts`. An auditor
  then decides which is right. Leaving the reversal unmarked is how a loop
  starts to oscillate between two fixes.
- A `withdrawn` finding cannot be reopened: an auditor decided it was wrong.
  If you have evidence that the auditor did not have, open a new finding and
  name the withdrawn one in `contradicts`, so that an auditor decides again.
- The packet's decisions bind you: audit directives, and the resolutions of
  earlier escalations by an auditor or by governance, each with where it came
  from. If you have evidence that one rests on something false, report that
  evidence through the channels above rather than acting against the
  decision.

## The verdict

The verdict follows from the content of your report, not from an overall
impression:

- `RED` if this report opens or reopens any blocking finding;
- otherwise `INDETERMINATE` if any requirement is `unverified`;
- otherwise `GREEN`.

Metron rejects a report whose verdict disagrees with its content.
Non-blocking findings do not affect the verdict, and neither do contract gaps,
although a gap keeps the gate closed until governance decides it.

A GREEN verdict says that every mandatory requirement is satisfied with
evidence and nothing blocking remains, inside the stated envelope, for this
stage and this activity. It does not say the candidate is free of defects or
optimal. Use `residual_risks` and `limitations` to make clear what your review
did not establish.

## Writing the report

The report is a single JSON document. Its envelope says which packet it
answers and who wrote it:

| Member | Content |
| --- | --- |
| `schema` | `metron.report/v1` |
| `packet` | `P-0013`, the packet you are answering |
| `packet_digest` | The packet's digest, copied unchanged from the report skeleton; if you were given the packet without a skeleton, take it from `metron next --json` or from the note `metron packet` printed when it issued the packet. Packet identifiers repeat from one ledger to the next, but the digest does not, so it ties your report to this exact packet; a report that carries any other packet's digest is refused. |
| `role` | `reviewer` |
| `author` | Who you are. `id` is a stable name for whoever acts in this role, 1 to 128 printable ASCII characters with no spaces: Metron decides role separation on it, so two identities must not differ only by something invisible. `kind` is `agent` or `human`. An agent should also give `model`, the model identifier, and may give `harness`, the tool that ran it, and `session`, an identifier that lets someone find the session's record later. |
| `body` | Your answer, described below |

Your author `id` must never have been used by an implementer or an auditor in
this ledger.

The body has these members:

| Member | Content |
| --- | --- |
| `coverage` | Exactly one entry per domain, eighteen in all. Each has `domain`, `status` and `summary`. Use `assessed` with a summary of what you examined and what you concluded, specific enough that a reader can tell what was covered and what was not. Use `not-applicable` only for a domain that has no bearing on this candidate within the scope, with the reason in `summary`. A domain in which an applicable requirement lies is always `assessed`. A contract waiver is a reason you may cite, but if a waived domain bears on an unacceptable risk, assess it. |
| `requirements` | Exactly one entry per applicable requirement: `requirement` (its identifier), `status` (`satisfied`, `violated` or `unverified`) and `evidence`, the checks, tests, measurements or reasoning behind the status, specific enough for someone to repeat. For `unverified`, say what evidence is missing and why you could not obtain it. |
| `findings` | The new findings of this review, described below. The list may be empty. |
| `reopen` | Optional. Resolved findings you reopen, each with `finding` (its `F-` identifier), `reason` and `justification`. |
| `addressed` | Optional. Open non-blocking findings that this candidate no longer exhibits, each with `finding` (its `F-` identifier) and `evidence`. |
| `contract_gaps` | Optional. Risks the contract does not cover, each with `title`, `description` (the risk, its evidence, and why no applicable requirement or unacceptable risk covers it) and optionally `suggested_requirement`, wording governance could adopt. |
| `verdict` | `GREEN`, `RED` or `INDETERMINATE`, as derived above. |
| `justification` | Why the verdict follows: for `RED`, the blocking findings and what they cite; for `INDETERMINATE`, the unverified requirements and the evidence each lacks; for `GREEN`, the basis for concluding that every requirement is satisfied and nothing blocks. |
| `residual_risks` | Optional. Risks that remain if the candidate proceeds, each as one statement. |
| `limitations` | Optional. What you could not examine or run, and how that limits your conclusions. |

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

# Packet P-0013

What follows is the content of packet P-0013, laid out for reading: the
information the brief above refers to.

## Identification

- Packet: `P-0013`, issued 2026-11-20T14:03:09Z under protocol `metron.protocol/v1`
- Packet digest: `sha256:efefefefefefefefefefefefefefefefefefefefefefefefefefefefefefefef`
- Action: `review`, for the `reviewer`, round 3
- Candidate: commit `c3e5a7b9d1f30246813579bdf02468ace1357902`, digest `sha256:3333333333333333333333333333333333333333333333333333333333333333`
- Fixes last verified on: commit `c3e5a7b9d1f30246813579bdf02468ace1357902`, digest `sha256:3333333333333333333333333333333333333333333333333333333333333333`, which is this candidate
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
- Convergence analysis: once per streak, when the failed steps without a GREEN verdict reach 3. A failed step is a review that does not end GREEN, a verification that rejects a fix or finds a blocking regression, or a run of the checks with a failure.
- Independent process audit: each time the failed steps without a GREEN verdict reach a multiple of 5. Once it is discharged, the campaign is suspended until governance resumes it.
- Campaign budget: the campaign is suspended once it has issued 20 packets without an approval, whatever the outcome of each step.
- The count of failed steps restarts with a GREEN verdict or when governance resumes a suspended campaign. A pin does not restart it, whether or not it changes the stage.

## Findings in the ledger

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

History, oldest first:

- Round 1, `finding.opened` in packet `P-0001`, on candidate `sha256:1111111111111111111111111111111111111111111111111111111111111111`
- Round 1, `finding.fix-claimed` in packet `P-0002`, on candidate `sha256:6666666666666666666666666666666666666666666666666666666666666666`: Retry until one hour has elapsed since the first attempt.
- Round 1, `finding.fix-confirmed` in packet `P-0004`, on candidate `sha256:6666666666666666666666666666666666666666666666666666666666666666`: TestSubscriberRecovery passes with a one-hour simulated outage; the loop now compares elapsed time, not attempts.

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

History, oldest first:

- Round 1, `finding.opened` in packet `P-0001`, on candidate `sha256:1111111111111111111111111111111111111111111111111111111111111111`
- Round 1, `escalation.raised` in packet `P-0002`: the implementer disputes F-0002: the envelope's load is an eighth of the measured capacity, so REQ-9 is not at risk
- Round 1, `escalation.resolved` in packet `P-0003`, on candidate `sha256:6666666666666666666666666666666666666666666666666666666666666666`: The latency budget holds at the envelope's load with a wide margin; F-0002 is an improvement, not a deficiency.
- Round 1, `finding.reclassified` in packet `P-0003`, on candidate `sha256:6666666666666666666666666666666666666666666666666666666666666666`: T1 to T2: TestLatencyBudget passes at 50 events per second with a 99th percentile of 40 ms.

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

History, oldest first:

- Round 1, `finding.opened` in packet `P-0001`, on candidate `sha256:1111111111111111111111111111111111111111111111111111111111111111`

### F-0004: An event acknowledged before its append is synced can be lost on a crash

- Tier `T1`, in `data-integrity`
- Status `resolved`; opened in round 1 by the reviewer; recurrences: 3
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
- Round 2, `finding.fix-confirmed` in packet `P-0012`, on this candidate: TestCrashRecovery passes 100 consecutive runs with a kill at every write boundary, on both endpoints.

### F-0005: internal/store imports internal/delivery for the event type

- Tier `T1`, in `architecture`
- Status `resolved`; opened in round 2 by the reviewer; recurrences: 0
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
- Round 2, `finding.fix-confirmed` in packet `P-0012`, on this candidate: `go list -deps ./internal/store` no longer lists internal/delivery.

### F-0006: Constructor takes seven positional parameters

- Tier `T4`, in `simplicity`
- Status `open`; opened in round 2 by the reviewer; recurrences: 0
- Components: `internal/delivery/dispatcher.go`

Description: An options struct would make call sites easier to read.

Evidence: dispatcher.go, NewDispatcher.

Impact: Readability only.

History, oldest first:

- Round 2, `finding.opened` in packet `P-0007`, on candidate `sha256:2222222222222222222222222222222222222222222222222222222222222222`

### F-0007: Signing error message includes the subscriber secret

- Tier `T0`, in `security-privacy`
- Status `withdrawn`; opened in round 2 by the reviewer; recurrences: 0
- Disputed once; cannot be disputed again
- Components: `internal/delivery/sign.go`
- Risk: `RISK-2`

Description: The error returned when signing fails formats the key into the message.

Evidence: sign.go formats the key with %q in the error path.

Impact: A secret could reach the logs.

Blocking rationale: Secrets in logs are an unacceptable risk at every stage.

Verification: No error path in sign.go formats the key.

History, oldest first:

- Round 2, `finding.opened` in packet `P-0007`, on candidate `sha256:2222222222222222222222222222222222222222222222222222222222222222`
- Round 2, `escalation.raised` in packet `P-0008`: the implementer disputes F-0007: the error path is unreachable, since NewSigner rejects every key that could make signing fail
- Round 2, `escalation.resolved` in packet `P-0009`, on candidate `sha256:4444444444444444444444444444444444444444444444444444444444444444`: Withdrawn: the only caller passes keys validated by NewSigner, and HMAC signing cannot fail for a valid key.
- Round 2, `finding.withdrawn` in packet `P-0009`, on candidate `sha256:4444444444444444444444444444444444444444444444444444444444444444`: The error path cannot be reached with any key NewSigner accepts; the mechanism does not occur.

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

This skeleton already has the right shape for packet P-0013. Fill in every
empty value as the brief describes, add the entries the brief allows, and
remove nothing that is required. Metron rejects a report that still has an
empty value where one is required.

```json
{
  "schema": "metron.report/v1",
  "packet": "P-0013",
  "packet_digest": "sha256:efefefefefefefefefefefefefefefefefefefefefefefefefefefefefefefef",
  "role": "reviewer",
  "author": {
    "id": "",
    "kind": ""
  },
  "body": {
    "coverage": [
      {
        "domain": "functional-correctness",
        "status": "",
        "summary": ""
      },
      {
        "domain": "objective-alignment",
        "status": "",
        "summary": ""
      },
      {
        "domain": "security-privacy",
        "status": "",
        "summary": ""
      },
      {
        "domain": "data-integrity",
        "status": "",
        "summary": ""
      },
      {
        "domain": "stability",
        "status": "",
        "summary": ""
      },
      {
        "domain": "reliability",
        "status": "",
        "summary": ""
      },
      {
        "domain": "performance",
        "status": "",
        "summary": ""
      },
      {
        "domain": "optimization",
        "status": "",
        "summary": ""
      },
      {
        "domain": "scalability",
        "status": "",
        "summary": ""
      },
      {
        "domain": "architecture",
        "status": "",
        "summary": ""
      },
      {
        "domain": "maintainability",
        "status": "",
        "summary": ""
      },
      {
        "domain": "technical-debt",
        "status": "",
        "summary": ""
      },
      {
        "domain": "simplicity",
        "status": "",
        "summary": ""
      },
      {
        "domain": "testability",
        "status": "",
        "summary": ""
      },
      {
        "domain": "observability",
        "status": "",
        "summary": ""
      },
      {
        "domain": "compatibility",
        "status": "",
        "summary": ""
      },
      {
        "domain": "operational-readiness",
        "status": "",
        "summary": ""
      },
      {
        "domain": "cost-efficiency",
        "status": "",
        "summary": ""
      }
    ],
    "requirements": [
      {
        "requirement": "REQ-1",
        "status": "",
        "evidence": ""
      },
      {
        "requirement": "REQ-2",
        "status": "",
        "evidence": ""
      },
      {
        "requirement": "REQ-3",
        "status": "",
        "evidence": ""
      },
      {
        "requirement": "REQ-4",
        "status": "",
        "evidence": ""
      },
      {
        "requirement": "REQ-5",
        "status": "",
        "evidence": ""
      },
      {
        "requirement": "REQ-6",
        "status": "",
        "evidence": ""
      },
      {
        "requirement": "REQ-7",
        "status": "",
        "evidence": ""
      },
      {
        "requirement": "REQ-8",
        "status": "",
        "evidence": ""
      },
      {
        "requirement": "REQ-9",
        "status": "",
        "evidence": ""
      },
      {
        "requirement": "REQ-10",
        "status": "",
        "evidence": ""
      }
    ],
    "findings": [],
    "verdict": "",
    "justification": ""
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
