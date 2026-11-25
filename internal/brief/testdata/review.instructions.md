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
