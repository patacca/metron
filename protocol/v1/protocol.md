# The Metron Engineering Review Protocol

Protocol version `metron.protocol/v1`.

This is the reference for everyone who takes part in a review governed by
Metron: the people who write the acceptance contract, the implementers,
reviewers, verifiers and auditors who do the engineering work, whether they
are humans or agents, and whoever runs the loop. It states the rules and the
reasons for them. The briefs Metron hands to each role apply these rules to
one concrete situation. The JSON Schemas printed by `metron schema` are the
exact definition of the documents exchanged; this text explains what those
documents mean.

## 1. What a review is for

A review answers one question: is this candidate sufficiently correct,
reliable, efficient, maintainable and safe for the purpose it is about to be
used for? It does not answer whether the candidate could be better. Almost
every codebase could be better, so a review that asks that question never
ends.

Review loops, and agent-driven ones in particular, tend to fail in one of two
ways. Either the bar is whatever the last reviewer felt like, so the outcome
depends on who looked, or the bar silently becomes "no further improvement is
possible", which nothing ever meets. The protocol avoids both by separating
two things that are easy to confuse:

- the gate, which asks whether the candidate meets an explicit, written
  standard for one stage of its life and one authorized activity; and
- the backlog, which records everything that could be improved, with the
  reasons it can wait.

Three parts make this work. The **acceptance contract** says what
"sufficient" means for this project at this stage. The **roles** supply the
engineering judgment: what is wrong, how to fix it, whether a fix holds. And
Metron, a deterministic program, supplies the rules nobody can bend: what must
be reviewed, what may block, when a decision may be revisited, when repeated
failure must change how the problem is investigated, and when the gate opens.
Metron never makes an engineering judgment, and no role can change ledger
state, the contract or the gate outcome.

## 2. Principles

| Principle | Rule | Why |
| --- | --- | --- |
| Safety | The gate never opens while a mandatory requirement is violated or unverified. Every failure mode closes the gate. | A gate that opens by default when something goes wrong (a crash, a timeout, a malformed report) approves exactly the cases nobody looked at. |
| Liveness | When the mandatory requirements are satisfied there is always a finite path to an open gate. Possible improvements never block. | A process that can be blocked forever by "it could be better" stops being used, and then it protects nothing. |
| Proportionality | A finding blocks only if it is tied to a contract requirement or an unacceptable risk. Over-engineering is itself reviewable. | Blocking needs a standard that exists before the review, or the standard is whatever the reviewer prefers. Unnecessary complexity is a cost, not a safe default. |
| Empiricism | Decisions cite evidence. What cannot be decided without real-world data is a calibration, not a blocker. | Arguments without evidence cannot be checked and so cannot converge. Some questions can only be settled by running the system. |
| Independence | Iteration counts, effort and deadlines change the investigation procedure, never the approval threshold. | A bar that drops after enough failed rounds rewards persistence instead of quality; one that rises does the opposite. Both make the verdict depend on history instead of on the candidate. |

## 3. Roles and authority

| Party | Decides | Cannot |
| --- | --- | --- |
| Metron | The next permitted action, whether a report is acceptable, ledger state, escalations, the gate outcome | Make engineering judgments |
| Roles (`implementer`, `reviewer`, `verifier`, `auditor`) | Engineering judgments: findings, fixes, verifications, audits | Write ledger state, change the contract, open the gate |
| Orchestrators | When and how to carry out the action Metron names | Choose a different action, skip one, or override an outcome |
| `governance` (the repository owner) | The acceptance contract and its amendments; governance-owned escalations | Bypass the gate conditions |

The four working roles divide the work so that no one marks their own
homework:

- The **reviewer** examines one candidate, read-only and in full, finds what
  is wrong with it and issues the verdict. Only a reviewer issues a verdict.
- The **implementer** fixes the blocking findings, or disputes those it
  believes are wrong, and commits the result.
- The **verifier** checks, read-only, whether each claimed fix actually
  resolves its finding and whether the fixes broke anything.
- The **auditor** is called in, read-only, when the loop stops converging or
  reaches a disagreement it cannot settle by itself. It adjudicates disputed
  and contradictory findings and can issue binding directives.

Role separation is checked on the identity each report declares in
`author.id`: an implementer is never also a reviewer, verifier or auditor in
the same ledger, and an auditor is never also a reviewer or verifier. A
reviewer who wrote the code reviews their intentions instead of the code; a
verifier who made the fix confirms what they meant to do. The same underlying
model can serve in several roles; what must differ is the identity, the
context and the authority with which it acts. An `author.id` is 1 to 128
printable ASCII characters without spaces: separation is decided by comparing
identities, so two of them must never differ only by something invisible. In
Metron's local trust mode the identity is declared, not proven, so
separation is a rule that cooperating parties follow.

The reviewer, verifier and auditor are read-only. No agent protocol can
enforce that, so Metron checks it after the fact: their report is rejected if
the candidate digest changed or the working tree stopped being clean between
the moment the packet was issued and the moment the report was submitted.

A read-only role still has to write its report somewhere, and `.metron/work/`
is that place. It is ignored by git and, like everything under `.metron/`,
outside the candidate and outside the cleanliness check. It is also the only
part of `.metron/` a role may write to: the contract belongs to governance
and the state directory to Metron. A report may also be written outside the
repository, but a report file anywhere else in the repository makes the tree
unclean, and the report is refused.

## 4. Fixed vocabulary

These sets are part of the protocol. They are not configurable per
repository, and they are spelled exactly as shown.

| Set | Members | Explained in |
| --- | --- | --- |
| Lifecycle stages | `prototype`, `controlled-validation`, `pilot`, `production`, `continuous-improvement` | Section 6 |
| Tiers | `T0`, `T1`, `T2`, `T3`, `T4` | Section 7 |
| Review domains | the 18 domains listed below | This section |
| Verdicts | `GREEN`, `RED`, `INDETERMINATE` | Section 13 |
| Roles | `governance`, `implementer`, `reviewer`, `verifier`, `auditor` | Section 3 |
| Requirement statuses | `satisfied`, `violated`, `unverified` | Section 10 |
| Coverage statuses | `assessed`, `not-applicable` | Section 10 |
| Finding statuses | `open`, `claimed`, `resolved`, `withdrawn` | Section 11 |
| Reopen reasons | `new-evidence`, `regression`, `invalid-verification`, `requirement-changed`, `contract-conflict` | Section 11 |
| Verification results | `confirmed`, `rejected` | Section 10 |
| Adjudication decisions | `uphold`, `withdraw`, `reclassify` | Section 10 |
| Escalation kinds | `dispute`, `contradiction`, `component-churn`, `regression-spiral`, `non-convergence`, `process-audit`, `contract-gap`, `evidence-gap` | Section 12 |

The **review domains** are the aspects of a system every review examines.
Every review assesses all eighteen, whatever the stage, because a defect does
not stop existing when nobody is required to look for it.

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

A domain being reviewed does not mean every possible improvement in it must
be made before proceeding. Whether something must be fixed is decided by its
tier, and its tier by evidence and impact.

## 5. The acceptance contract

The contract, `.metron/contract.yaml`, is the only project-specific input to
a review. Governance writes it; everyone else works within it. It exists
because a standard has to be written down before the review starts:
otherwise every reviewer brings their own, and the outcome measures the
reviewer rather than the candidate.

| Member | Meaning |
| --- | --- |
| `project` | The project's name, its objectives and, optionally, what it deliberately does not do |
| `stage` | The lifecycle stage a GREEN gate authorizes entry to |
| `authorizes` | The concrete activity a GREEN gate permits, such as a pilot with named users |
| `scope` | What is under review |
| `operating_envelope` | The expected conditions and load. Findings about behaviour outside the envelope cannot block |
| `requirements` | The mandatory acceptance criteria, `REQ-n` |
| `risks.unacceptable` | Risks a blocking finding may cite, `RISK-n` |
| `risks.accepted` | Residual risks governance has accepted, `ACC-n`. They never block |
| `not_applicable` | Domains waived for this stage, each with a rationale |
| `calibrations` | Provisional parameters, `CAL-n`, with the evidence needed to settle them and when to reassess |
| `convergence` | The thresholds of the convergence safeguards; defaults 2, 3 and 5 |

A **requirement** has a `domain`, a `statement` of what must hold, a
`criterion` that says how anyone can decide that it holds, an optional
`command` that Metron runs itself as an automated check, and an optional
`from` stage at which it becomes mandatory. The criterion is the important
part: a statement without an acceptance boundary invites endless argument
about whether it is met. A requirement is **applicable** when its `from`
stage is not later than the contract's `stage`.

The **operating envelope** matters as much as the requirements. A system is
not deficient because it fails under conditions it was never meant to face
at this stage. Stating the envelope explicitly keeps reviewers from blocking
on speculative scenarios and keeps implementers from claiming that a real
failure inside the envelope is out of scope.

**Stage coverage.** Each stage adds domains that the contract must cover with
at least one applicable requirement or an explicit `not_applicable` waiver.
Coverage is cumulative.

| Stage | Domains that become mandatory to cover |
| --- | --- |
| `prototype` | `functional-correctness`, `objective-alignment`, `architecture` |
| `controlled-validation` | `data-integrity`, `testability`, `observability`, `stability` |
| `pilot` | `reliability`, `performance`, `compatibility`, `security-privacy` |
| `production` | `scalability`, `operational-readiness`, `maintainability` |
| `continuous-improvement` | `optimization`, `technical-debt`, `simplicity`, `cost-efficiency` |

Coverage constrains what the contract must address, not what the reviewer
looks at. It forces governance to make an explicit decision about every
domain that matters at a stage, instead of letting a domain go unmentioned
by accident.

**Pinning.** A contract is used only once governance has pinned it with
`metron contract pin -m <reason>`. The binding identity is the digest of the
contract's meaning, its canonical JSON form, so comments and formatting can
change without invalidating a review while any change of meaning cannot.
While the file's meaning differs from the pinned one, everything that
depends on the contract waits for governance. A new pin voids the open
packet and invalidates any verdict issued under the old contract, because
that verdict answered a different question. When a pin changes the stage,
the count of consecutive failed rounds restarts, because the new stage is a
new question; round numbers and findings carry over.

## 6. Lifecycle-aware acceptance

A system is evaluated against the requirements of its next authorized
lifecycle stage, not against a hypothetical final production state. Judging
a prototype by production standards blocks learning; judging a production
release by prototype standards ships risk. The stages, in order:

| Stage | What it is about |
| --- | --- |
| `prototype` | Feasibility, objective alignment, foundational architecture and basic correctness |
| `controlled-validation` | Deterministic invariants, bounded operation, integrity, testability, observability and containment of failures |
| `pilot` | Real-world use under a stated envelope: integration behaviour, measured performance, recovery and operational reliability |
| `production` | Full operational readiness, security, scalability, monitoring, resilience, rollback and contractual obligations |
| `continuous-improvement` | Measured optimization, emerging technical debt, evolving requirements and production feedback |

Each transition needs its own GREEN gate, under a contract that names that
stage.

This assigns quality requirements to the stage where they matter; it does
not lower them. A requirement that only matters in production is written
down now with `from: production` and starts blocking when the contract
targets production. And if an early stage can cause irreversible effects,
for instance because a pilot touches valuable real data, the integrity and
safety requirements for those effects are mandatory at that stage, stated as
requirements or unacceptable risks. Foreseeable irreversible decisions, such
as a data format that would make a required later migration impractical, are
legitimate review concerns at any stage, through the same channels.

## 7. Findings and tiers

A **finding** is one substantiated observation about the candidate. Every
finding has a tier, a domain, a title, the affected components, a
description of the concrete mechanism, the evidence, and the impact inside
the operating envelope.

| Tier | Name | Blocks the gate | Resolution |
| --- | --- | --- | --- |
| `T0` | Critical | yes | Mandatory before proceeding |
| `T1` | Material deficiency | yes | Mandatory before proceeding |
| `T2` | Improvement | no | Recorded with its expected benefit and the rationale for deferring it |
| `T3` | Calibration | no | Recorded with the evidence needed and when to reassess |
| `T4` | Preference | no | Recorded; no action required |

- `T0` is a critical failure: security compromise, data corruption,
  irreversible damage, a fundamental architectural failure, or violation of
  a critical invariant.
- `T1` is a demonstrable deficiency that violates an applicable requirement
  or creates an unacceptable risk under the operating envelope or through
  foreseeable irreversible consequences.
- `T2` is a justified improvement with an identifiable benefit whose absence
  violates nothing in the contract.
- `T3` is a potential improvement whose value cannot be established without
  real-world measurements, usage patterns or other information the project
  does not have yet.
- `T4` is an alternative approach, a stylistic preference or a speculative
  improvement without a demonstrated material advantage.

**A tier is determined by evidence and impact, never by the domain.** An
inefficiency is not automatically minor and a correctness issue is not
automatically blocking. An algorithm that exceeds the required execution
time is a `T1` performance finding; an index that would cut latency by a
tenth when latency already meets its requirement is a `T2` performance
finding. An architecture that makes a required upcoming feature
prohibitively difficult can be `T1`; a reviewer's preference for an
equivalent pattern is `T4`.

Non-blocking findings are recorded so that they are not lost and need not
be rediscovered. They form the backlog. A finding about an accepted risk
never blocks, and cannot cite it: governance has already decided to live
with that risk.

### What makes a finding blocking

A `T0` or `T1` finding closes the gate until it is resolved, so the protocol
asks more of it. A blocking finding must:

- cite an applicable requirement (`REQ-n`) or an unacceptable risk
  (`RISK-n`), which is what makes resolving it mandatory rather than
  desirable;
- describe a concrete failure mechanism, not a general unease;
- carry evidence: a reproduction, a test, a measurement, static analysis, a
  demonstrated logical contradiction or an established technical
  constraint;
- state its impact under the conditions of the operating envelope;
- explain why it must be resolved before this stage rather than later; and
- say how a fix can be verified, so that the verifier has an objective test.

Preferences, scenarios outside the operating envelope and hypothetical
benefits do not establish a blocker on their own, however well argued.

### Contract gaps

Sometimes a reviewer finds a real risk that no applicable requirement and no
unacceptable risk covers. Promoting it to a blocker would mean the reviewer
is extending the contract on their own authority; ignoring it would mean the
risk goes unrecorded. Instead the reviewer reports it as a **contract gap**.
Each gap raises a `contract-gap` escalation that governance must discharge,
either by amending the contract or by dismissing the gap with a rationale.
The escalation keeps the gate closed until governance has decided, so
reporting a gap is not free, and it is not optional when the risk is real.

### Uncertainty is not a calibration

Missing evidence is a different matter from a possible improvement. When the
evidence needed to establish a mandatory requirement is unavailable, the
requirement is `unverified` and the gate stays closed. `T3` is for parameters
whose best value cannot be known yet, not for mandatory conditions nobody
has managed to check. Using a calibration to disguise an unresolved
mandatory risk defeats the purpose of having mandatory requirements.

## 8. Proportionality in improvement-prone domains

In eight domains, "it could be better" is especially easy to mistake for "it
must be fixed": `optimization`, `performance`, `scalability`, `architecture`,
`maintainability`, `technical-debt`, `simplicity` and `cost-efficiency`. A
blocking finding in one of them must pass a proportionality test as well:

1. **Problem.** What concrete inefficiency or limitation exists?
2. **Evidence.** What measurement, analysis, reproducible behaviour or
   established constraint supports it?
3. **Impact.** What happens if the candidate proceeds without the change?
4. **Urgency.** Why must it be addressed at this stage?
5. **Proportionality.** What is the smallest change that resolves it, and is
   that change proportionate to the problem?
6. **Regression risk.** What could the change break or complicate?
7. **Deferral cost.** Why would resolving it later cost substantially more?
8. **Acceptance boundary.** Which requirement or unacceptable risk makes it
   mandatory?

The first four and the last are the ordinary requirements of any blocking
finding. The finding additionally records the proposed change, its
regression risk and the cost of deferral.

Both directions of error are real. Under-engineering creates debt that grows
more expensive to repay; over-engineering creates it too, through
unnecessary abstractions, speculative generality, premature optimization,
extra dependencies and new regression surfaces. Excess complexity is
therefore a legitimate finding, usually in `simplicity`, `maintainability`
or `technical-debt`, classified by the same rules as any other. An
optimization can be blocking without production benchmarks, for instance an
unbounded memory growth inside the envelope, or a design that makes a
mandatory future migration impractical. The fact that something can be made
more efficient is never sufficient by itself.

## 9. Evidence

Evidence is whatever lets someone else check a claim: a reproducible test or
command, a measurement, a static analysis, a demonstrated logical
contradiction, an established technical constraint, or a precise reading of
the code that another engineer can follow. A failing test is one form of
evidence, not the only acceptable one.

Passing tests do not establish correctness. They establish that the cases
someone thought of behave as expected. A reviewer who stops at "the tests
pass" has reviewed the tests, not the system.

When a requirement has a `command`, Metron runs it with `metron check` and
records the result against the candidate and the exact command text; the
latest result per requirement, candidate and command text is the one that
counts. A passing check is necessary for the gate but does not by itself
make the requirement `satisfied`: the reviewer still judges whether the
check actually exercises the criterion.

## 10. What each role produces

Every role answers its packet with a report: a JSON document with an
envelope (`schema`, `packet`, `packet_digest`, `role`, `author`) and a
role-specific `body`.
The role briefs describe every member; this section describes what the
report must accomplish.

### Reviewer

The reviewer assesses all eighteen domains (`assessed`, or `not-applicable`
with a rationale) and gives every applicable requirement a status:
`satisfied`, `violated` or `unverified`, each with its evidence. It reports
every substantiated finding it discovers, not only the first blocker: the
implementer fixes findings in batches and looks for shared causes, which only
works if the batch is complete, and a review that reports one defect at a
time turns into one round per defect.

A `violated` requirement must be cited by at least one blocking finding the
same report opens or reopens, so that every violation has something the
implementer can act on; conversely, a requirement that such a finding cites
must be marked `violated`, so that the requirement statuses and the findings
tell the same story. Risks the contract does not cover go into contract
gaps. Residual risks and the limits of the inspection (what was not
examined, what could not be run) are recorded so that a GREEN verdict does
not claim more than the review established.

The review is read-only, full-scope and independent. Earlier verdicts, the
number of rounds, the effort spent and deadlines do not change the standard
a reviewer applies.

### Implementer

The implementer addresses every blocking finding in its packet, each with
either a fix or a dispute, and may also fix non-blocking findings from the
backlog. It works on the batch as a whole and looks for shared root causes
before patching locally, because several findings with one cause fixed one
at a time produce several patches and no cure.

Each fix lists the full hashes of the commits that contain it, all lying
between the round's candidate and the current one, and names the components
it touched. The working tree must be clean. Every listed commit must change
something outside `.metron/`, and for every finding fixed, the candidate must
be one the finding was never recorded on: not the one it was opened or
reopened on, and not any on which a fix for it was rejected. A fix committed
before the packet was requested is therefore valid, while claiming a rejected
fix again without a new change is not, and neither is going back to an earlier
tree: the ledger already holds that the finding is present there. A fix for a
finding flagged root-cause-required must include a root-cause analysis.

A finding can be disputed once, with reasons. A dispute raises a `dispute`
escalation and an auditor adjudicates it; if the auditor upholds the
finding, it must be fixed. Weakening a check, a test or the contract to make
a finding disappear is not a fix.

### Verifier

The verifier gives every claimed fix a result, `confirmed` or `rejected`,
with evidence. Confirming is a positive claim: that the failure mechanism the
finding describes no longer occurs, checked by the verification method the
finding names. The verifier also examines the impact surface of the changes
and may report regressions as new findings, each naming the finding whose
fix introduced it. A rejected fix returns the finding to `open` and counts
towards its recurrence.

### Auditor

The auditor discharges every escalation named in its packet, and no other,
with one resolution each: an analysis and a conclusion. Each escalation asks
its own question, and a single answer for all of them would let one be passed
over. It gives a decision for every finding the packet lists as adjudicable,
and for no other: `uphold` (the finding stands as recorded), `withdraw` (the
finding is wrong and leaves the process), or `reclassify` (the finding is
real but its tier is wrong, and the decision gives the new tier).
Reclassification changes the tier and nothing else, so an audit can make a
finding blocking only if the finding, as recorded, already has everything
section 7 asks of a blocking finding: a citation of an applicable requirement
or an unacceptable risk of the pinned contract, the reason it must be
resolved before this stage, how a fix can be verified and, in the
improvement-prone domains, its proportionality. Where any of that is missing
the auditor upholds the finding at its tier and says why it should block; a
reviewer then opens a blocking finding that stands on the contract, or
governance amends the contract.

It may issue directives, which are binding guidance carried into every later
packet, and it may recommend contract changes to governance. It cannot make
them, and it does not lower the standard to end a loop.

## 11. Review continuity

The ledger keeps every finding with its history. A finding moves through
this state machine:

```
            fix-claimed            fix-confirmed
  open ─────────────────▶ claimed ───────────────▶ resolved
   ▲                         │                         │
   │       fix-rejected      │                         │ reopened
   └─────────────────────────┘◀────────────────────────┘
   │
   └── withdrawn (terminal; auditor adjudication only)
```

A finding is unresolved while `open` or `claimed`. Only an auditor can
change a recorded finding's tier, and only through adjudication.

Every packet carries each finding's **history**: every step recorded for it,
with the round, the packet whose report caused it, the candidate it was made
on, and what was said at the time, such as the fix's summary, the verifier's
evidence, the justification for a reopening, a dispute or contradiction and
how it was resolved, or the rationale for a withdrawal or reclassification.
Continuity depends on it. A reviewer who suspects that a verification was
wrong can read the evidence that confirmed the fix, and an auditor can read
the reasoning of earlier audits instead of guessing at it.

**A step holds for the candidate it was made on.** A candidate is identified
by its content, so undoing a change returns to a candidate the ledger has
already seen. A finding is *recorded on* every candidate on which it was
opened or reopened, or on which a fix for it was rejected. A fix confirmed on
one candidate says nothing about those: a candidate that a resolved blocking
finding was recorded on *carries* that finding. It cannot be approved, and it
is not reviewed again; the candidate has to move to one that contains the
fix.

Prior resolutions are evidence, not precedent. A reviewer may question an
earlier decision, but revisiting one must be traceable and based on
evidence, or the loop can be kept going indefinitely by re-arguing settled
points. A resolved finding may be reopened for one of five reasons:

| Reason | Meaning |
| --- | --- |
| `new-evidence` | Evidence not available before shows that the resolution was insufficient |
| `regression` | A later change broke what the fix had established |
| `invalid-verification` | The verification that confirmed the fix was itself wrong |
| `requirement-changed` | A requirement or assumption the resolution relied on has changed |
| `contract-conflict` | The resolution conflicts with the pinned contract |

A new finding that would reverse an earlier resolution, so that fixing it
would undo a previous fix, must say so. That raises a `contradiction`
escalation: oscillating between two fixes is a classic way for a loop to
never end, and an auditor decides which way is right. A withdrawn finding
cannot be reopened, since an auditor has decided it was wrong; a reviewer
with evidence the auditor did not have opens a new finding that declares
the contradiction, and an auditor decides again.

Each finding has a **recurrence count**: the number of times a fix for it
was rejected plus the number of times it was reopened. When the count
reaches the root-cause threshold (2 by default), the finding is flagged
root-cause-required, and the next fix for it must include a root-cause
analysis. A finding that keeps coming back is telling you the local patches
are aimed at a symptom.

## 12. Convergence safeguards

Some loops do not converge: the same component is patched round after
round, fixes cause more defects than they resolve, or reviewers and
implementers disagree without a way to settle it. The protocol detects these
patterns and changes the investigation procedure. It never lowers the bar
and never opens the gate. Each safeguard raises an escalation; any open
escalation keeps the gate closed and takes priority over the ordinary loop.

| Kind | Raised when | Owner | Discharged by |
| --- | --- | --- | --- |
| `dispute` | An implementer disputes a finding | `auditor` | Adjudication |
| `contradiction` | A finding declares that it reverses an earlier resolution | `auditor` | Adjudication |
| `component-churn` | The same component is fixed in 3 distinct rounds | `auditor` | Component-level audit |
| `regression-spiral` | A verification finds at least one blocking regression and no fewer than the fixes of blocking findings it confirms | `auditor` | Redesign assessment |
| `non-convergence` | Consecutive rounds without GREEN reach 3; raised once per streak | `auditor` | Convergence analysis |
| `process-audit` | Every 5th consecutive round ends without GREEN | `auditor` | Independent process audit |
| `contract-gap` | A reviewer reports a risk the contract does not cover | `governance` | `metron resolve`, after amending the contract or dismissing the gap with a rationale |
| `evidence-gap` | A verdict is `INDETERMINATE` | `governance` | `metron resolve`, after supplying evidence or amending the contract |

The thresholds for root-cause analysis (2), convergence analysis (3) and the
process audit (5) are defaults that the contract may change; they are the
only tunable values. A streak of consecutive rounds without GREEN ends with
a GREEN verdict or with a pin that changes the stage. Escalation resolutions,
whether by an auditor or by governance, and audit directives are carried into
every later packet, each with the packet it came from, and bind every role.

One more safeguard is built into the loop itself: an unchanged candidate
cannot be resubmitted in the hope of a different verdict. A RED verdict rests
on the blocking findings its report opened or reopened, all recorded on that
candidate, so the candidate is reviewed again only when none of them stands:
an audit withdrew them or reclassified them to a non-blocking tier. An
INDETERMINATE verdict rests on missing evidence, so its candidate is reviewed
again only after governance has resolved the evidence gap. Upholding a
disputed finding, resolving an unrelated escalation or re-running the checks
changes nothing about what the verdict rested on, and going back to a
candidate after its findings were fixed elsewhere does not either (section
11). A failed assessment is resolved through correction, evidence or
adjudication, not by asking again.

## 13. Verdicts

Only the reviewer issues a verdict, and the verdict follows from the content
of the review rather than from an overall impression:

- `RED` if the report opens or reopens any blocking finding;
- otherwise `INDETERMINATE` if any applicable requirement is `unverified`;
- otherwise `GREEN`.

Metron rejects a report whose verdict does not match its content. A RED
round leads to fixing and verification and then a review of the new
candidate. An INDETERMINATE round raises an `evidence-gap` escalation:
governance supplies the missing evidence or amends the contract. Non-blocking
findings do not affect the verdict, and the number of earlier rounds does
not change these conditions.

## 14. The review loop

`metron next` names exactly one permitted action: the first row of this
table whose condition holds.

| # | Condition | Action | Owner |
| --- | --- | --- | --- |
| 1 | The ledger fails verification | `repair-ledger` | governance |
| 2 | The contract is invalid, or its digest differs from the pinned one | `pin-contract` | governance |
| 3 | A packet is open and still valid | `submit-report` | the packet's role |
| 4 | A governance-owned escalation is open | `adjudicate` | governance |
| 5 | The working tree is not clean | `commit-changes` | implementer |
| 6 | An auditor-owned escalation is open | `audit` | auditor |
| 7 | A finding is `claimed` | `verify` | verifier |
| 8 | A blocking finding is `open` | `fix` | implementer |
| 9 | The candidate carries a resolved blocking finding (section 11) | `commit-changes` | implementer |
| 10 | A mandatory check has no passing result on the candidate | `run-checks` | implementer |
| 11 | The candidate has no GREEN verdict under the pinned contract | `review` | reviewer |
| 12 | Otherwise | `none`: the gate is open | |

A **candidate** is a commit, identified by its candidate digest: a SHA-256
over the commit's tree with everything under `.metron/` left out, so that
committing review state does not change the candidate while any other change
does. A **round** is one independent, read-only, full-scope review of one
candidate; fixing and verifying happen between rounds. Rounds are numbered
from 1 across the whole ledger and never restart, so a round number
identifies one review unambiguously, whatever stage it belonged to.

```
review ──RED──▶ fix ──▶ verify ──all confirmed──▶ review (new candidate)
   │             ▲         │
   │             └─rejected or regressions
   ├──INDETERMINATE──▶ adjudicate (governance supplies evidence or amends the contract)
   └──GREEN──▶ gate
```

Each action is carried out through a **packet**: a JSON document holding
everything the role needs, including the contract, the candidate, the
relevant findings with their history, the latest check results, the rounds
of the current stage, the binding decisions, the role's brief and the schema
its report must satisfy. At most one packet is open at a time.
Asking for the packet again while it is open and valid returns the same
packet, so a crashed agent can simply be retried. A packet becomes void when
the contract is re-pinned, or when the candidate changes or the tree stops
being clean underneath a read-only role.

## 15. Reports and ingestion

A role starts from the report skeleton for its packet, which already has the
right shape, and writes the report under `.metron/work/` (section 3):

```
metron packet --format skeleton -o .metron/work/report.json
metron ingest .metron/work/report.json
```

Metron validates a report in three layers and accepts it only if all three
pass; a rejected report changes nothing and the error says what to correct.

1. **Structural**: the document satisfies its JSON Schema.
2. **Semantic**: it is internally consistent and consistent with the
   contract and the packet.
3. **Transition**: the state machine permits it now.

The report must name the open packet, carry that packet's role, and carry
its `packet_digest`, copied unchanged from the report skeleton; a role given
only the JSON packet finds the digest in `metron next --json` or in the note
`metron packet` prints when it issues the packet. The identifier alone is not
enough: packet identifiers repeat from one ledger to
the next, so a report written for another repository, another ledger or an
earlier issue of the same identifier would otherwise be accepted. The digest
does not repeat, and a report carrying any other packet's digest is refused.
A packet is consumed by the first report accepted for it and its identifier
is never reused within a ledger, so a report cannot be replayed against a
later state. Submitting the same report again succeeds and changes nothing.

## 16. The gate

`metron gate` evaluates five conditions and opens only if all of them hold:

| | Condition | Evaluated as |
| --- | --- | --- |
| C | Acceptance criteria satisfied | Every applicable requirement is `satisfied` in the reviewer report of the latest round |
| B | No blockers | No unresolved `T0` or `T1` finding, none carried by this candidate, and no open escalation |
| E | Evidence sufficient | Every applicable requirement with a `command` has a passing result recorded for this candidate and this command text |
| I | Integrity | The current candidate is the reviewed one, the tree is clean, the contract is the pinned one, and the ledger verifies under this protocol version |
| V | Independent verdict | The latest round's verdict is `GREEN`, for this candidate under this contract, by an author who never acted as implementer |

Anything else, including an internal error, an unreadable file or a missing
tool, keeps the gate closed. No iteration limit, timeout or failure ever
produces approval.

A GREEN gate approves one candidate for one stage and the activity named in
`authorizes`. It records that an independent reviewer found the mandatory
requirements satisfied with evidence and nothing blocking, inside the stated
operating envelope. It does not claim that the implementation is free of
defects, that it is optimal, or that it is suitable for conditions outside
the envelope or for a later stage. The non-blocking findings recorded along
the way remain the backlog for the next stage.
