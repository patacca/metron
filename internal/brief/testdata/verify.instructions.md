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
repository, commit, stash, switch branches or check out another revision,
and do not touch `.metron/`. When you submit your report, Metron compares
the candidate digest and the cleanliness of the working tree with their
state when this packet was issued, and rejects the report if either has
changed. Building the code and running tests is fine as long as nothing is
left behind that `git status` would show: send output to a directory outside
the repository or to paths the repository already ignores. To look at
another revision, use `git show` or a separate clone. Write your report file
outside the repository too.

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
| `packet_digest` | The packet's digest, copied unchanged from the report skeleton. Packet identifiers repeat from one ledger to the next, but the digest does not, so it ties your report to this exact packet; a report that carries any other packet's digest is refused. |
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
