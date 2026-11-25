# Implementer brief for packet P-0011

## Your situation

You are the implementer of Courier.
The review of round 2, under a contract that targets the `pilot`
stage, examined the commit below and found problems that must be resolved
before a candidate can be approved for that stage:

- reviewed commit `9b2d4f6a8c0e1357924680ace13579bdf0246813`

The repository has since moved to commit `b8d0f2a4c6e81357924680ace13579bdf0246802`, and your
work goes on top of it.

The review is governed by Metron, a program that runs the loop around you. It
issued this packet, will validate your report and records your fixes in its
ledger. After you, a verifier who is not you checks each fix, and a new,
complete review of the resulting candidate follows. Governance owns the
acceptance contract, and an auditor adjudicates disputes.

Everything this brief refers to is in the packet: the contract, the findings
you work on with their evidence and history (`findings`), the latest check
results (`evidence`), the rounds of this stage (`history`) and the binding
decisions (`decisions`). If you received this brief as a prompt, the same
information follows it, laid out for reading.

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

Each finding's `history` lists every step it went through, with what was said
at the time: the summary of each earlier fix, the evidence with which a
verifier rejected it, the justification for a reopening, an adjudicated
dispute. Read it before you change anything. A fix that repeats an approach a
verifier has already rejected will be rejected again.

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
directory to Metron. A change there is never a fix, and editing the state
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
Every listed commit must be one added after the reviewed commit: reachable
from the commit you submit, and not from the reviewed one. A commit that
contains changes for several findings is listed under each of them.

Every listed commit must also change something outside `.metron/`; a commit
that changes only `.metron/` changes nothing that is reviewed, so it fixes
nothing. And for each finding you fix, the candidate you submit must differ
from the one on which that finding was last opened, reopened or had a fix
rejected; the finding's `history` shows when that was. A fix committed before
this packet was requested therefore counts. Claiming a rejected fix again
without a new change does not: the verifier has already found that change
insufficient.

## Writing the report

The report is a single JSON document. Its envelope says which packet it
answers and who wrote it:

| Member | Content |
| --- | --- |
| `schema` | `metron.report/v1` |
| `packet` | `P-0011`, the packet you are answering |
| `packet_digest` | The packet's digest, copied unchanged from the report skeleton. Packet identifiers repeat from one ledger to the next, but the digest does not, so it ties your report to this exact packet; a report that carries any other packet's digest is refused. |
| `role` | `implementer` |
| `author` | Who you are. `id` is a stable name for whoever acts in this role, 1 to 128 printable ASCII characters with no spaces: Metron decides role separation on it, so two identities must not differ only by something invisible. `kind` is `agent` or `human`. An agent should also give `model`, the model identifier, and may give `harness`, the tool that ran it, and `session`, an identifier that lets someone find the session's record later. |
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
