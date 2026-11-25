# 0004: Convergence and correction discipline

Status: accepted, 2026-11-25

## Context

An architectural review of the protocol, held before it is stabilized, found
five defects and seven risks.

Defects:

- **D1.** The gate opened on a candidate with a blocking finding that an
  auditor had upheld, once its fix was confirmed on another candidate and the
  tree was reverted.
- **D2.** An audit could reclassify to a blocking tier a finding that cites
  nothing, so the gate stayed closed on a finding with no requirement, risk,
  blocking rationale or verification.
- **D3.** Any resolved escalation, even one upholding the findings, allowed
  another review of an unchanged RED candidate.
- **D4.** A rejected fix could be claimed again by alternating between two
  rejected trees, because only the latest rejection was compared.
- **D5.** The regression spiral compared blocking regressions with every
  confirmed fix, so a confirmed fix of a non-blocking finding could hide one.

Risks:

- **R1.** Nothing halted a campaign: a process audit fired every fifth failed
  round, an auditor discharged it with free text, and the loop went on.
- **R2.** The failure streak counted reviewer verdicts only, so a fix rejected
  again and again, or a check that could never pass, raised nothing.
- **R3.** A correction did not control what changed: fix packets invited
  optional backlog fixes, only the commits a report listed were examined, and
  the verification ran before the checks.
- **R4.** Directives and resolutions went into every later packet, across
  re-pins and stage changes, and nothing could supersede one.
- **R5.** The reviewer was shown the round number, the earlier verdicts and
  the reviewer identities, which the Independence principle says must not
  matter.
- **R6.** Evidence that governance supplied for one candidate was carried, as
  binding, into the review of another.
- **R7.** Component churn matched free-text component names exactly, and a
  finding was tied to its requirement by identifier only.

A related gap: `metron check` waited for a check command as long as it ran,
so a check that hung blocked an unattended campaign and left no record.

An independent review of the corrective work then found four more defects:
the budget was reset only when the next round opened, so a packet issued
after an approval counted against the campaign already approved; a GREEN
round counted as an approval although the gate never opened, for instance
with a contract gap still open; lapse was tracked by the latest claim only,
so an earlier claim survived a later one on another candidate; and a claim
made again could list only the new commits.

## Decision

[Architecture](../architecture.md) sections 5 to 12 state the rules. This
record says what changed and why.

**Every step of a finding is bound to its candidate** (D1, D3, D4). A
candidate is its tree, so undoing a change returns to a candidate the ledger
has already seen. Each step of a finding's history names the candidate it was
made on, and a fix claim the candidate it was claimed on. A finding is
*recorded on* every candidate on which it was opened, reopened or had a fix
rejected. A candidate that a resolved blocking finding was recorded on
*carries* it: gate condition B fails, and `metron next` names
`commit-changes` instead of a review. A fix is refused on every candidate the
finding was recorded on. The rule that let an unchanged candidate be reviewed
again once any escalation was resolved is gone: a RED candidate is reviewed
again only when none of the findings its verdict rested on stands.

**Reclassification changes the tier and nothing else** (D2). An audit can
move a finding to T0 or T1 only if the finding, as recorded, cites an
applicable requirement or an unacceptable risk of the pinned contract and
states its blocking rationale, its verification and, in the domains that
require it, its proportionality. Otherwise the auditor upholds it, and a
reviewer opens a blocking finding that stands on the contract.

**The regression spiral compares like with like** (D5): blocking regressions
against the confirmed fixes of blocking findings.

**Every campaign is bounded** (R1, R2).

- One failure streak counts every failed step: a review whose verdict is not
  GREEN, a verification that rejects a fix or finds a blocking regression,
  and a check recording with a failure. A step is one command, however many of
  its events record a failure. Only a GREEN verdict or governance resuming a
  suspended campaign restarts the streak; a pin, a change of stage, a new
  candidate or a discharged audit does not. Failing checks can make an audit
  due before any round has a verdict; its packet names no round.
- A campaign is the work towards one approval. Its budget, the new threshold
  `suspend_after` (20 by default), counts every packet issued in it. It ends
  approved with the command that leaves the ledger showing an approval: the
  latest round is a GREEN round of the current stage under the pinned
  contract, no escalation is open, and no blocking finding is unresolved or
  carried by that round's candidate. A GREEN verdict that leaves an
  escalation open, or that a pin has set aside, ends nothing.
- A new governance-owned escalation, `suspension`, ends a campaign without
  approval. It is raised when a process audit is discharged, when an auditor
  asks for it (`suspend`), and with the packet that uses up the budget. While
  it is open the gate is closed, no packet is issued, and `metron check`
  refuses before running anything. Resolving it with `metron resolve` is
  governance's decision to resume, and begins a new campaign.
- `metron check --timeout` bounds each check command, 30 minutes by default.
  A command still running at the limit is stopped with what it started and
  recorded as failed with exit code 124, so a hung check is a failed step.
- The orchestrator's stop rule: run the step `metron next` names, and stop
  when it is `none` or its owner is `governance`.

What this guarantees: an orchestrator that performs the steps `metron next`
names reaches an open gate or a step owned by governance after at most
`suspend_after` packets, and after at most `process_audit_after` failed steps
and the audits they raise; no count, budget or time limit opens the gate. What
it does not: that a campaign reaches GREEN, which takes a candidate that can
meet the contract, a contract that can be met, and roles that judge well. A
role whose reports are all rejected changes nothing in the ledger, so an
orchestrator bounds its own retries of one packet.

**A correction changes what its blocking findings need, and nothing else**
(R3).

- The loop runs fix, run-checks, verify, review: the mandatory checks pass on
  a candidate before anyone verifies it, and the verifier sees their results.
- A fix packet holds the blocking findings and nothing else: the whole
  batch, every open blocking finding and every one whose fix is already
  claimed. The backlog is ordinary development work, and a reviewer closes an
  open non-blocking finding that the candidate no longer exhibits by listing
  it in `addressed`, with evidence (`finding.addressed`).
- Every commit since the reviewed one that changes the candidate must be
  listed under a fix, in this report or an earlier accepted one. Metron lists
  the interval itself and refuses a report that leaves a commit out.
- A fix is claimed on a candidate. A report submitted on another candidate
  than the one the claims stand on, because the candidate changed or because
  the implementer commits now, claims every claimed fix again, with every
  commit of the earlier claim that is still part of the candidate. A finding
  an audit upholds while another fix is claimed is therefore fixed in a
  report that restates the earlier fix. So every fix a verifier sees was
  claimed on the candidate it verifies and lists all its commits, and a
  correction is verified as one batch on one candidate.
- The verifier judges the scope of every fix, `proportionate` or
  `excessive`, with evidence. An excessive fix is rejected even when its
  finding is resolved.
- A review packet names the candidate the fixes were verified on
  (`verified`). The review remains the only source of a verdict.

**Decisions apply where they were made** (R4, R6). Audit directives and
escalation resolutions are decisions, identified `D-0001` and on. Each
records the stage and contract pinned when it was made, its candidate and the
requirements it is about. It binds while no audit has superseded it and the
contract it was made under is the pinned one; evidence supplied for an
evidence gap binds only on the candidate the gap was raised on. Packets list
the binding decisions and, separately, the earlier ones with the reason they
no longer bind. An audit can supersede an audit decision (`supersede`,
`decision.superseded`); a governance decision is not an auditor's to
supersede.

**Counting rests on facts** (R7). Component churn is counted on the paths
that the commits of a fix change, recorded with the claim; one escalation
names every path that a report brings to the threshold. A finding records the
contract it was established under, and an escalation the candidate it was
raised on.

**The reviewer is not told how earlier reviews went** (R5). A review packet
carries no round number and no earlier rounds, and the reviewer's brief and
prompt name neither. Its findings come without the round of the finding or of
any step of their history; their order remains. Findings with their history,
decisions, check results and the verified candidate are all still there. This
is about what a reviewer is handed, not secrecy: in local mode the ledger and
`metron status` show the rounds to whoever asks.

**Gate condition E is "automated evidence".** Its evaluation is unchanged; a
requirement without a `command` is established under condition C.

## Alternatives rejected

- **Approving, or lowering a threshold, when a limit runs out.** No count,
  budget or time limit ever produces approval: running out of budget
  suspends.
- **An iteration cap instead of a suspension.** A cap ends a run but records
  no decision and names nobody to take one. A suspension is in the ledger,
  keeps the gate closed, and makes the next step governance's.
- **A budget of failures instead of packets.** Steps that succeed (a
  confirmed fix, a passing check, a discharged audit) and packets nobody
  answers can keep a campaign going without a single failure. Counting every
  packet issued bounds those campaigns too.
- **Restarting the counts on a pin or a change of stage.** Amending the
  contract would erase the record of a loop that is not converging.
- **A scoped or partial review after a correction.** No such review may issue
  GREEN. A verification establishes that fixes hold, never that a candidate
  is acceptable; the review after a correction is of the whole candidate and
  can find what the correction broke.
- **Letting an audit complete the finding it reclassifies.** Reclassification
  cannot add a citation, a rationale or a verification, or a finding would
  block on an auditor's word alone.
- **A dedicated improvement command.** Improvements are ordinary development,
  reviewed like any other change; a reviewer's `addressed` closes the
  findings they resolve. A separate governed cycle for optional work would
  bring optional changes back into mandatory steps.
- **Counting churn on component names.** Two reports rarely spell a
  component alike; the names are descriptions for people.
- **Binding decisions per stage.** A change of contract changes the question a
  decision answered, whether or not the stage changed.
- **Withholding the finding history from the reviewer as well.** Nothing that
  limits what a review can conclude is withheld; only the round number, the
  earlier verdicts, the reviewer identities and the round of each finding
  step are.

## Compatibility and migration

`metron.protocol/v1` is not frozen before the first release, and these
changes alter it and the v1 document shapes incompatibly without changing any
version string.

**Ledgers.** A binary with these changes refuses a ledger written by an
earlier build as malformed as soon as it holds any of these events:

- a `packet.issued` that lists a `backlog`, which every fix packet issued
  while a non-blocking finding was open does;
- any `finding.fix-claimed`, which lacks `candidate` and `paths`;
- any `finding.fix-confirmed` or `finding.fix-rejected`, which lack `scope`
  and `scope_evidence`.

`metron fsck` then reports the event and exits 1, `metron next` and `metron
status` name `repair-ledger`, and `metron gate` exits 3. A ledger that holds
none of these events replays, and its history is counted under the new rules:
an earlier check recording with a failure, for example, is now a failed step.

There is no migration tool. The supported path is:

1. Finish or abandon the current campaign under the old build.
2. Set the state directory aside: move `.metron/state/` out of the
   repository, where git history still holds it if it was committed, or
   point `--state-dir` or `$METRON_STATE_DIR` at a new location.
3. Run `metron init`, which keeps `.metron/contract.yaml` and starts a new
   ledger, and pin the contract.

A contract with a `convergence` block must add `suspend_after` and be pinned
again. A contract without one keeps its contract digest.

Every change to a public document shape or behaviour, and what to do about
it:

**Contract** (`metron.contract/v1`)

- `convergence` gains `suspend_after`, required whenever the block is
  present. Add it, then pin.
- `convergence_audit_after` and `process_audit_after` now count failed steps,
  not rounds without GREEN; neither a pin nor a change of stage restarts the
  count; discharging a process audit suspends the campaign. Review the values
  you set.
- The template `metron init` writes explains the four thresholds. Nothing to
  do.

**Packet** (`metron.packet/v1`)

- `backlog` is removed. Read the non-blocking findings from `metron findings
  --backlog`.
- The `subjects` of a fix packet are the whole batch: every open blocking
  finding and every finding whose fix is already claimed, not only the open
  ones.
- `round` is no longer required. A review packet has none, and neither has an
  audit packet issued before any round has a verdict. A tool that needs the
  round reads it from the ledger.
- A review packet has no `history`. Fix, verify and audit packets keep it.
- A finding's `round`, and the `round` of each step of its history, are no
  longer required. A review packet omits them and keeps the steps in order;
  fix, verify and audit packets, and `metron findings`, still carry them.
- `verified`, in a review packet, names the candidate the fixes were last
  verified on. A reviewer compares it with `candidate`.
- `earlier_decisions` is added, and `decisions` holds only the decisions that
  bind. Read both.
- A decision gains `id`, `stage` and `contract`, all required, and the
  optional `kind`, `candidate`, `requirements` and `superseded_by`.
- A finding gains `contract`, required, and each step of its history gains
  `candidate`.
- An escalation gains `candidate`, and its kind may be `suspension`.

Consumers that decode packets strictly must accept the new members and the
missing ones.

**Report** (`metron.report/v1`)

- Verifier: every verification requires `scope` (`proportionate` or
  `excessive`) and `scope_evidence`, and a report that confirms an excessive
  fix is refused. Add both members; reject an excessive fix.
- Reviewer: optional `addressed`, each entry with `finding` and `evidence`.
  Nothing required.
- Auditor: optional `supersede`, each entry with `decision` and `rationale`,
  and optional `suspend`. A reclassification to T0 or T1 is refused unless the
  finding as recorded meets every rule of a blocking finding; uphold such a
  finding instead.
- Implementer: the shape is unchanged, the rules are not. A report addresses
  the packet's subjects only, so no non-blocking finding; lists every commit
  since the reviewed one under a fix; and cannot fix a finding on a
  candidate it was recorded on. Submitted on another candidate than the one
  the claims stand on, it claims every claimed fix again, with every commit of
  the earlier claim that is still part of the candidate; otherwise it is
  refused at `/body/fixes` or `/body/fixes/N/commits`. A claimed finding
  cannot be disputed. Make improvements outside corrections, list every
  commit, and restate earlier claims whenever you commit during a correction.

**Ledger events** (`event.schema.json`)

- New event types: `finding.addressed` and `decision.superseded`.
- `packet.issued` loses `backlog`.
- `finding.fix-claimed` gains `candidate` and `paths`, both required.
- `finding.fix-confirmed` and `finding.fix-rejected` gain `scope` and
  `scope_evidence`, both required.
- `escalation.raised` gains `candidate`; `packet` becomes optional and is
  absent for an escalation raised by a check recording; its kind may be
  `suspension`; the `components` of a `component-churn` escalation are now
  file paths.
- Event order: accepting a report yields `finding.addressed` after
  reopenings and before new findings, and `decision.superseded` after
  directives. Recording check results can end with `escalation.raised`, and
  issuing a packet with a `suspension`.

Old ledgers: see above. Tools that read events accept the new types and
members.

**Command output documents**

- `metron.status/v1`: `failed_streak` counts failed steps; `campaign_steps`,
  `campaign_budget` and, while the campaign is suspended, `suspended` are
  added. `campaign_steps` is 0 right after the command that leaves the ledger
  showing an approval. In the text, `Rounds:` no longer ends with the failed
  streak, and a new `Campaign:` line follows it. Scripts read the JSON.
- `metron.check/v1`: a result gains `timed_out`, and the document gains
  `escalations_raised`, always present. In the text, a stopped check's line
  names the limit, and a raised escalation is listed as
  `escalation.raised      E-000N (...)`, as `metron ingest` lists events.
- `metron.gate/v1` and the gate text: the description of condition B names
  carried findings.
- `metron.findings/v1`: a finding gains `contract`, and each history step
  `candidate`.
- `metron.next/v1`: the shape is unchanged. `run-checks` now comes before
  `verify`; `commit-changes` also names a candidate that carries a finding;
  `fix` also names lapsed claims and, in its `findings`, the claimed findings
  of the batch; `adjudicate` also names a suspension.
- The note `metron packet` prints for a review packet no longer names the
  round.

**Exit statuses.** No status is added or changes meaning.

- `metron check --timeout` with a zero, negative or unparsable value exits 2.
- `metron check` exits 1 when a check is stopped at its limit, and while the
  campaign is suspended, when it refuses before running any command.
- `metron packet` exits 1 while a suspension is open, as for any step owned
  by governance.
- `metron gate` exits 1 while a suspension is open or while the candidate
  carries a finding.

**Command behaviour**

- `metron check` runs each command within `--timeout` and records a stopped
  one as failed with exit code 124. A recording with a failure is a failed
  step and can raise `non-convergence` or `process-audit`. While the campaign
  is suspended it runs nothing.
- The campaign budget starts again with the command that leaves the ledger
  showing an approval, not when the next round opens.
- `metron packet` counts every packet against the budget, raises a
  `suspension` with the packet that uses it up, and issues nothing while a
  suspension is open.
- `metron ingest` refuses the reports described under **Report**, and an
  audit report raises a `suspension` when it discharges a process audit or
  asks to suspend.
- `metron resolve` accepts a `suspension`; resolving it resumes the campaign
  and begins a new one.
- `metron next` follows the new order of section 8 of the architecture.
- `metron contract pin` no longer restarts any count, and decisions made
  under another contract stop binding until that contract is pinned again.
