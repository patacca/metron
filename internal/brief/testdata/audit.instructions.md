# Auditor brief for packet P-0021

## Your situation

You are the auditor of Courier.
You have been called in after round 5, under a contract that targets
the `pilot` stage. The current candidate is this commit:

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

Each kind of escalation asks a different question, and each escalation gets
its own resolution in your report, even where several share a cause.

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
`F-0008` and `F-0009`. For each one, give one decision:

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

You have the whole ledger. Every finding comes with its status, recurrences,
latest fix and history, and the history records each step with what was said
at the time: fix summaries, the evidence with which verifiers confirmed or
rejected them, the justifications for reopenings, earlier disputes and
contradictions with their resolutions, and the rationale of every withdrawal
and reclassification. The packet also lists the rounds of this stage with
their verdicts, and every decision so far with the packet it came from. Read
the earlier rationales rather than reconstructing them, and check them rather
than taking them as given. Use the repository's history as well, since the
commits between rounds show what each fix actually changed. Base your
conclusions on evidence you can point to, as a reviewer must, and say where
the evidence is incomplete.

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

## Writing the report

The report is a single JSON document. Its envelope says which packet it
answers and who wrote it:

| Member | Content |
| --- | --- |
| `schema` | `metron.report/v1` |
| `packet` | `P-0021`, the packet you are answering |
| `packet_digest` | The packet's digest, copied unchanged from the report skeleton; if you were given the packet without a skeleton, take it from `metron next --json` or from the note `metron packet` printed when it issued the packet. Packet identifiers repeat from one ledger to the next, but the digest does not, so it ties your report to this exact packet; a report that carries any other packet's digest is refused. |
| `role` | `auditor` |
| `author` | Who you are. `id` is a stable name for whoever acts in this role, 1 to 128 printable ASCII characters with no spaces: Metron decides role separation on it, so two identities must not differ only by something invisible. `kind` is `agent` or `human`. An agent should also give `model`, the model identifier, and may give `harness`, the tool that ran it, and `session`, an identifier that lets someone find the session's record later. |
| `body` | Your answer, described below |

Your author `id` must never have been used by an implementer, a reviewer or a
verifier in this ledger.

The body has these members:

| Member | Content |
| --- | --- |
| `resolutions` | Exactly one entry per escalation in this packet, and none for any other. Each has `escalation` (its `E-` identifier), `analysis` (what you examined, what you found, and the evidence for it) and `conclusion` (what you concluded and what the loop must do next). When Metron accepts the report, each conclusion is recorded as its escalation's resolution, discharges it, and is carried into every later packet as a binding decision, so write it to be read on its own. |
| `adjudications` | One entry per adjudicable finding, with `finding` (its `F-` identifier), `decision` (`uphold`, `withdraw` or `reclassify`), `tier` (the new tier; present for, and only for, `reclassify`) and `rationale` (why, with the evidence you checked). Left out when there is nothing to adjudicate. |
| `directives` | Optional. Binding guidance for every later packet, one statement per entry. |
| `contract_recommendations` | Optional. Changes to the contract you recommend to governance, one per entry, each with its reason. |
