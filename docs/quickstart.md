# Quick start

This guide takes a small repository through one complete Metron loop: a
contract, a failed review, a fix, the checks on the fix, a verification, a
second review, and a gate that opens. It takes about fifteen minutes.

Every command and every piece of output below comes from a real run. A `…`
marks where a long digest or some output was cut; most commands end with a
`Next step` block, which is shown only in part. When you already know the
loop, the [cheat sheet](cheatsheet.md) has the same material as tables.

## The idea

- You write **one file**, the acceptance contract: what the project is for,
  which stage you want to enter, and what must hold before you do.
- From then on you run **`metron next`** and do the one thing it names,
  until the gate is open or the next step is a decision only governance can
  make.
- Work is handed out as a **packet** and comes back as a **report**. Metron
  validates each report and records it.
- **`metron gate`** exits 0 when one specific commit is approved, and not
  before.

Metron does not run models and does not judge code. People and agents do
that, each in one of five roles:

| Role | What it does |
| --- | --- |
| governance | Writes the contract, decides what it means, and decides whether a suspended campaign resumes. Usually the repository owner. |
| implementer | Writes and fixes the code. |
| reviewer | Reviews one commit, read-only, and gives the verdict. |
| verifier | Checks that each fix does what it claims, and no more than its finding needs. |
| auditor | Steps in when the loop stops converging. |

One rule decides who may be whom: **whoever implements does not review,
verify or audit.** A common setup is you as governance, your coding agent as
implementer, and a second agent or a colleague as reviewer and verifier.

## Install

There are no releases yet, so build it. You need Go and
[`just`](https://just.systems) to build, and `git` to run it.

```
git clone https://github.com/patacca/metron && cd metron
just build            # produces bin/metron
```

Put `bin/metron` somewhere on your `PATH`.

## The example

The repository in this guide holds a small script that turns an article title
into a URL slug, and a test for it.

`slugify.sh`:

```sh
#!/bin/sh
# Turn a title into a URL slug: "Hello, World!" -> "hello-world".
printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9' '-'
echo
```

`test.sh`:

```sh
#!/bin/sh
# Each case: input, expected slug.
fail=0
check() {
  got=$(sh slugify.sh "$1")
  if [ "$got" != "$2" ]; then
    echo "FAIL: '$1' -> '$got', expected '$2'"
    fail=1
  fi
}
check "hello" "hello"
check "Hello World" "hello-world"
exit $fail
```

Both files are committed. `sh test.sh` passes, and the script is still wrong:
`sh slugify.sh "Hello, World!"` prints `hello--world-`, and the tests do not
cover punctuation. Passing checks are one condition of the gate, not the gate.

To follow along, create a repository with these two files (`git init`, then
commit them) and run the commands as you read.

## 1. Set up the repository

```
$ metron init
Created .metron/contract.yaml.
Created .metron/.gitignore.
Created .metron/.gitattributes.
Created .metron/state/ledger.jsonl, protocol metron.protocol/v1.
…
```

Open `.metron/contract.yaml`. It is a commented template in which every empty
value is a decision for you to make. For the example, replace it with this:

```yaml
schema: metron.contract/v1

project:
  name: "slugify"
  objectives:
    - "Turn an article title into a slug that is safe to use in a URL"
  non_objectives:
    - "Transliterating non-ASCII characters"

stage: prototype

authorizes: "Using slugify.sh in the blog's draft preview, on our own machines"

scope:
  - "slugify.sh and its tests in test.sh"

operating_envelope:
  - "ASCII titles of up to 200 characters, one per invocation"

requirements:
  - id: REQ-1
    domain: functional-correctness
    statement: "A slug contains only a-z, 0-9 and single dashes, and never starts or ends with a dash"
    criterion: "test.sh passes and covers punctuation, repeated spaces and leading or trailing spaces"
    command: "sh test.sh"
  - id: REQ-2
    domain: objective-alignment
    statement: "Two titles that differ only in case or punctuation give the same slug"
    criterion: "test.sh contains at least one such pair and passes"

risks:
  unacceptable:
    - id: RISK-1
      statement: "A slug contains a character that must be escaped in a URL"
  accepted:
    - id: ACC-1
      statement: "A title with no letters or digits gives an empty slug"

not_applicable:
  - domain: architecture
    rationale: "A single ten-line script; there is no structure to assess yet"
```

Three things are worth knowing while you write yours:

- **A requirement is a property, not a task.** "Importing the same file twice
  creates no duplicates", not "add deduplication". Its `criterion` says how
  anyone can decide that it holds.
- **A `command` is run by `metron check`** and must exit 0 on the exact commit
  under review. Requirements without one are judged by the reviewer.
- **Only what is written here can block.** A reviewer may raise anything, but
  a finding blocks the gate only when it cites a requirement or an
  unacceptable risk. Each stage asks you to cover a few review domains or to
  waive them with a reason, as `architecture` is waived above.

Ask Metron whether the contract is complete, then authorize it and commit:

```
$ metron contract
Contract: .metron/contract.yaml
Contract digest: sha256:2f6ae8b6…
Raw digest:      sha256:22615d22…
Pinned digest:   none
Result: valid, not pinned. Governance authorizes it with: metron contract pin -m <reason>

$ metron contract pin -m "initial contract"
Pinned the contract for stage prototype.
  contract.pinned        contract sha256:2f6ae8b6bf12, stage prototype, pinned by demo <demo@example.com>: initial contract
…

$ git add -A && git commit -m "chore: adopt metron"
```

While something is missing, `metron contract` lists each problem with its line
number. A pin records your git `user.name` and `user.email` as the governance
identity.

## 2. Ask what is next

```
$ metron next
Next step:    run-checks (owner: implementer)
Reason:       REQ-1 has no recorded result on this candidate: make the checks pass on a committed candidate and record them
Requirements: REQ-1
Run:          metron check
Then:         if a check fails, change the code, commit it and run the checks again
```

That is the whole interface. `metron next` names one step, who owns it, why,
and the command for it. You never choose the step yourself.

## 3. Run the checks

```
$ metron check
REQ-1    passed  exit 0   sh test.sh
Recorded 1 result for commit fc211ed53a5a (candidate sha256:ca215ded…).

Next step:    review (owner: reviewer)
…
Run:          metron packet
…
```

Most commands end by printing the next step, so you rarely need to type
`metron next`.

## 4. Review

`metron packet` hands out the work. It has three forms:

```
metron packet --format prompt                                 # a brief to read
metron packet --format skeleton -o .metron/work/report.json   # the report to fill in
metron packet                                                 # the packet as JSON, for tools
```

The prompt is self-contained: instructions for the role, the contract, the
findings so far, and the report skeleton. A person reads it; an agent receives
it as its task.

The first request issues the packet and records it in the ledger. Asking
again, in any format, returns the same packet until its report is accepted.
Each time, a one-line note says which packet you got; it is left out of the
listings below.

The reviewer examines the commit without changing it and fills in the
skeleton. The parts that carry the judgment look like this:

```json
"author": { "id": "alice", "kind": "human" },
```

`author` is how Metron knows who acted in a role: each report declares it, and
nothing else about you is consulted.

```json
"requirements": [
  {
    "requirement": "REQ-1",
    "status": "violated",
    "evidence": "sh slugify.sh 'Hello, World!' prints hello--world- (see finding dashes)."
  },
  {
    "requirement": "REQ-2",
    "status": "satisfied",
    "evidence": "slugify.sh reads $1 and writes one line to standard output; it touches nothing else."
  }
],
"findings": [
  {
    "key": "dashes",
    "tier": "T1",
    "domain": "functional-correctness",
    "title": "Punctuation and spaces produce repeated and trailing dashes",
    "components": ["slugify.sh"],
    "description": "Every character outside a-z0-9 is replaced by its own dash, so a run of such characters gives a run of dashes, and one at either end of the title gives a leading or trailing dash.",
    "evidence": "sh slugify.sh 'Hello, World!' prints hello--world-",
    "impact": "Most real titles contain punctuation, so most slugs break REQ-1.",
    "requirement": "REQ-1",
    "blocking_rationale": "The draft preview would publish these slugs as they are.",
    "verification": "sh slugify.sh 'Hello, World!' prints hello-world, and sh slugify.sh '  a  b  ' prints a-b."
  },
  {
    "key": "usage",
    "tier": "T2",
    "domain": "maintainability",
    "title": "No usage message when slugify.sh is called without a title",
    "components": ["slugify.sh"],
    "description": "Called with no argument, the script prints an empty line and exits 0.",
    "evidence": "sh slugify.sh; echo $? prints an empty line and 0.",
    "impact": "A caller that forgets the argument gets an empty slug instead of an error.",
    "expected_benefit": "A wrong call is noticed at once.",
    "deferral_rationale": "The preview always passes a title, and ACC-1 accepts an empty slug."
  }
],
"verdict": "RED",
"justification": "The finding dashes is T1 and violates REQ-1."
```

The report also has a `coverage` list with one entry for each of the 18 review
domains, marked `assessed` or `not-applicable` with a sentence each. The
skeleton already contains all 18.

The two findings show the distinction Metron exists for. The first is `T1`
and cites `REQ-1`, so it blocks. The second is `T2`: a real improvement that
the contract does not require, so it is recorded and blocks nothing.

Submit the report:

```
$ metron ingest .metron/work/report.json
Accepted the reviewer report for packet P-0001.
  report.accepted        P-0001: reviewer report by alice (human)
  finding.opened         F-0001 (T1, functional-correctness): Punctuation and spaces produce repeated and trailing dashes
  finding.opened         F-0002 (T2, maintainability): No usage message when slugify.sh is called without a title
  verdict.recorded       round 1: RED by alice

Next step:    fix (owner: implementer)
Reason:       blocking finding F-0001 is open: fix or dispute it
Findings:     F-0001
Run:          metron packet
…
```

## 5. Fix

The implementer takes the next packet, changes the code and **commits**. What
Metron reviews is always a commit, never the working tree.

```
$ metron packet --format skeleton -o .metron/work/report.json
$ $EDITOR slugify.sh test.sh
$ git add -A && git commit -m "fix: one dash per run of other characters, none at the ends"
```

The packet holds the blocking finding F-0001 and nothing else. The `T2`
finding is not part of a correction: a correction changes what its blocking
findings need, and improvements are made in ordinary commits, outside it. A
correction covers the whole batch, so a later fix packet also lists every
finding whose fix is already claimed.

The fix squeezes each run of other characters into one dash and trims both
ends, and `test.sh` gains the two cases the finding names, before its `exit`
line:

```sh
#!/bin/sh
# Turn a title into a URL slug: "Hello, World!" -> "hello-world".
printf '%s\n' "$1" |
  tr '[:upper:]' '[:lower:]' |
  tr -cs 'a-z0-9\n' '-' |
  sed -e 's/^-//' -e 's/-$//'
```

```sh
check "Hello, World!" "hello-world"
check "  a  b  " "a-b"
```

The report says which commits fix which finding. Commit hashes are written in
full; `git rev-parse HEAD` prints one. Every commit made since the reviewed
one must be listed under a fix: Metron lists them itself and refuses a report
that leaves one out.

```json
"author": { "id": "bob", "kind": "human" },
"body": {
  "fixes": [
    {
      "finding": "F-0001",
      "commits": ["cb80625c1a7849c091c46d24c145c39e9507c59c"],
      "summary": "tr -s now squeezes each run of other characters into one dash, and sed removes a dash at either end.",
      "components": ["slugify.sh", "test.sh"],
      "tests": "Added the two cases from the finding to test.sh; sh test.sh passes."
    }
  ]
}
```

```
$ metron ingest .metron/work/report.json
Accepted the implementer report for packet P-0002.
  report.accepted        P-0002: implementer report by bob (human)
  finding.fix-claimed    F-0001: tr -s now squeezes each run of other characters into one dash, and sed removes a dash at either end. (commits cb80625c1a78)

Next step:    run-checks (owner: implementer)
…
```

The fix is *claimed*, not accepted. An implementer who thinks a finding is
wrong puts it under `disputes` with a rationale instead, and an auditor
decides.

## 6. Run the checks on the fix

Nobody verifies a fix until the mandatory checks pass on the commit that
holds it:

```
$ metron check
REQ-1    passed  exit 0   sh test.sh
Recorded 1 result for commit cb80625c1a78 (candidate sha256:09d193e6…).

Next step:    verify (owner: verifier)
…
```

If a check fails, correct the code in another commit. The fix was claimed on
the earlier commit, so the claim has lapsed: `metron next` asks for the fix
again, the new report claims it with every commit it had before and the new
one, and then the checks run on the result. Nothing reaches the verifier that
no fix accounts for, and every fix it sees was claimed on the commit it
verifies.

## 7. Verify

The verifier applies the `verification` the finding asked for, and judges the
fix's scope: `proportionate` when its commits change only what the finding
needs, `excessive` when they change more. The verifier may be the reviewer,
but never the implementer.

```
$ metron packet --format skeleton -o .metron/work/report.json
```

```json
"author": { "id": "alice", "kind": "human" },
"body": {
  "verifications": [
    {
      "finding": "F-0001",
      "result": "confirmed",
      "evidence": "sh slugify.sh 'Hello, World!' prints hello-world and sh slugify.sh '  a  b  ' prints a-b, as the finding asks.",
      "scope": "proportionate",
      "scope_evidence": "The commit changes the pipeline in slugify.sh and adds the two cases the finding names to test.sh. Nothing else changed."
    }
  ]
}
```

```
$ metron ingest .metron/work/report.json
Accepted the verifier report for packet P-0003.
  report.accepted        P-0003: verifier report by alice (human)
  finding.fix-confirmed  F-0001: sh slugify.sh 'Hello, World!' prints hello-world and sh slugify.sh '  a  b  ' prints a-b, as the finding asks.

Next step:    review (owner: reviewer)
…
```

A `rejected` result sends the finding back to the implementer, and so does an
`excessive` scope: Metron refuses a report that confirms an excessive fix.

## 8. Review the new commit

The code changed, so the earlier verdict no longer applies and the new commit
gets a full review of its own. Its packet names the commit the fix was
verified on, under `verified`; here it is the commit under review, so the
reviewer knows that no change since has gone unverified. This time both
requirements are `satisfied`, `findings` stays empty, and the verdict is
`GREEN`.

```
$ metron packet --format skeleton -o .metron/work/report.json
$ metron ingest .metron/work/report.json
Accepted the reviewer report for packet P-0004.
  report.accepted        P-0004: reviewer report by alice (human)
  verdict.recorded       round 2: GREEN by alice

Next step:    none, the gate is open
…
```

The verdict follows from the content. Metron refuses a report that says
`GREEN` while it opens a blocking finding, or `RED` without one.

## 9. The gate

```
$ metron gate
Gate: OPEN for stage prototype
Authorizes: Using slugify.sh in the blog's draft preview, on our own machines
Candidate: commit cb80625c1a7849c091c46d24c145c39e9507c59c, sha256:09d193e6…

[ok]   C  Every applicable requirement is satisfied in the reviewer report of the latest round
[ok]   B  No unresolved T0 or T1 finding, none that was recorded on this candidate and resolved on another, and no open escalation
[ok]   E  Every applicable requirement with a command has a passing result for this candidate and this command text
[ok]   I  The candidate is the reviewed one, the tree is clean, the contract is the pinned one, and the ledger verifies under this protocol version; the report carrying the verdict is stored intact
[ok]   V  The latest round's verdict is GREEN, for this candidate, under this contract, by an author who never acted as implementer
```

`metron gate` exits 0, which is what a script or a CI job tests. Commit the
review record:

```
$ git add -A && git commit -m "chore: record the review"
```

The gate stays open after that commit, because `.metron/` is not part of what
was reviewed. Change anything else and it closes until the new commit has been
checked and reviewed.

Commit `.metron/` whenever you commit code, as `git add -A` did in step 5, and
again when a loop ends. Git history is the only backup the ledger has.

The improvement that did not block is still on record:

```
$ metron findings
F-0002  T2  open      maintainability: No usage message when slugify.sh is called without a title
        round 1, reported by the reviewer; components slugify.sh
```

It is worked on like any other change, in ordinary commits and never inside a
correction. When a later review finds that the commit under review no longer
has the problem, the reviewer lists the finding under `addressed`, with the
evidence, and that resolves it.

## Doing it with agents

Nothing changes except who reads the packet. Write the brief to a file and
give it to the agent that plays the role:

```
metron packet --format prompt -o .metron/work/brief.md
```

Then tell the agent to read `.metron/work/brief.md` and do what it says. The
brief ends with how to hand the report back. An agent that can run commands
writes `.metron/work/report.json` and runs `metron ingest` itself. One that
cannot returns the report as a single JSON document, and you submit it.
`metron ingest -` reads the report from standard input, so an agent's output
can be piped straight in.

Three things to get right:

- **One identity per role.** Each report declares an `author`, for example
  `{"id": "review-agent", "kind": "agent", "model": "<model id>"}`. The
  implementer's `id` must differ from the reviewer's and the verifier's.
- **One session per role.** A reviewer that shares a conversation with the
  implementer is not independent, whatever its `id` says. Start the reviewer
  and the verifier from the brief alone.
- **Reviewers and verifiers do not edit the code.** If the working tree
  changes while one of them works, its packet becomes invalid and the report
  is refused.

An orchestrator drives the same loop through `--json`. Right after the checks
in step 3, for example:

```
$ metron next --json
{
  "schema": "metron.next/v1",
  "step": "review",
  "owner": "reviewer",
  "reason": "candidate sha256:ca215ded… has no GREEN verdict under the pinned contract (no round of stage prototype has a verdict): review it",
  "command": "metron packet"
}
```

An orchestrator runs the step `metron next` names and stops in two cases:
`step` is `none`, because the gate is open, or `owner` is `governance`. The
governance steps, `repair-ledger`, `pin-contract` and `adjudicate`, are
decisions for the repository's owner, never steps to automate. While
governance owns the next step, `metron packet` issues nothing, and while the
campaign is suspended `metron check` runs nothing and records nothing either;
both then exit with status 1. A campaign stopped there has not been approved:
only `metron gate` exiting 0 means that.

Two more things an unattended run needs:

- **Bound your own retries.** A rejected report changes nothing, so Metron
  cannot tell an agent that keeps failing from one that has not answered yet.
  Give each packet a limited number of attempts, then stop and hand it to a
  person.
- **Bound the checks.** `metron check` stops a check command still running
  after 30 minutes, or after `--timeout`, and records it as failed with exit
  code 124. Set a limit that fits your checks.

The campaign itself is bounded by the contract. Failed steps (a review that
is not `GREEN`, a rejected fix, a failed check) bring in an auditor, and
after a process audit, at an auditor's request, or once the campaign has
issued 20 packets without an approval, it is suspended until governance
resumes it with `metron resolve`.

Metron does not start agents itself yet. Identities are declared, not proven:
read the [trust model](trust-model.md) before you rely on the gate against
agents you do not trust.

## When Metron says no

A refusal says what is wrong and changes nothing, so you can correct and
retry.

| You see | What it means | What to do |
| --- | --- | --- |
| `Next step: commit-changes` | Files outside `.metron/` are modified or untracked. A review is bound to a commit. | Commit or discard them. Keep reports under `.metron/work/`. |
| `Next step: commit-changes` with `blocking finding F-0001 was recorded on candidate …` | You are back on a commit where a blocking finding was recorded, after its fix was confirmed on another, for example because the fix was reverted. | Move to a commit that contains the fix. The same tree is never reviewed again in the hope of another verdict. |
| `Next step: pin-contract` | The contract is incomplete, or it changed since it was pinned. | `metron contract`, fix what it lists, then `metron contract pin -m "<reason>"`. An open packet is voided. |
| `report rejected by the structural layer` | The report does not have the right shape; the message names each member. | Fix those members and run `metron ingest` again. |
| `the verdict is GREEN but the content implies RED` | The verdict must follow from the findings and the requirement statuses. | Correct the verdict or the content. |
| `author "bob" acted as implementer in this ledger and cannot act as verifier` | Role separation. | Have someone else, or another agent, do it under their own `id`. |
| `commit … changes the candidate since the reviewed commit …, and no fix lists it` | A commit made during the correction belongs to no fix. | List it under the finding it serves. A change that serves no finding does not belong in a correction: take it out of the candidate's history and make it outside the correction. |
| `Next step: fix` with `claim it again in a report that accounts for every commit since the reviewed one` | The commit changed after the fix was claimed, for example to make a check pass, so the claim has lapsed. | `metron packet`, then claim the fix again with every commit it had before and the new ones. |
| `/body/fixes: the fix of F-0001 was claimed on candidate … and the candidate is now …` | The report is on another commit than the one a claimed fix stands on, for example because an audit upheld a second finding and you committed its fix. A correction is verified as one batch on one commit. | Claim F-0001 again in the same report, next to the new fix. |
| `commit … belonged to the fix of F-0001 when it was claimed before and is still part of the candidate` | A fix claimed again must keep the commits it was first claimed with. | Add them to its `commits`. |
| `the fix of F-0001 is confirmed although its scope is excessive` | A fix that changes more than its finding needs cannot be confirmed. | Reject it. The implementer takes out what the finding does not need. |
| `Next step: adjudicate` | Governance owes a decision: the review was `INDETERMINATE`, the reviewer found a risk the contract does not cover, or the campaign is suspended. | Supply the evidence or amend and pin the contract, then `metron resolve E-0001 -m "<decision>"`. After an `INDETERMINATE` review the commit is then reviewed again. Resolving a suspension resumes the campaign; leaving it open ends it. |
| `Next step: audit` | The loop is not converging: a disputed finding, three failed steps without `GREEN` (a review that is not `GREEN`, a rejected fix, a failed check), the same files changed fix after fix. | `metron packet` issues the auditor's packet. The bar does not drop. |
| `Next step: submit-report` | A packet is open and waiting for its report. | `metron ingest <file>`. `metron packet` prints the packet again if it was lost. |

## Where to go next

- [Cheat sheet](cheatsheet.md): commands, steps, report fields and exit codes
  on one page.
- `metron protocol`: the rules every role works under, including how to
  classify a finding.
- [Architecture](architecture.md): the full design, for when you need the
  exact rule.
- [Trust model](trust-model.md): what the gate does and does not guarantee.
