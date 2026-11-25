# Trust model

Metron defines two trust modes. Review semantics and gate evaluation are
identical in both; what differs is how much an adversary must be trusted not
to exist.

Only **local mode** is implemented. Do not rely on Metron as a deployment gate
against untrusted or adversarial agents until authenticated mode exists.

## Local mode (cooperative)

Local mode assumes that every party with write access to the repository and
the state directory is cooperating: a developer and the agents they run, none
of which is trying to forge an approval.

What it provides:

- A hash-chained event ledger and deterministic replay of state from it.
- Reports bound to a packet, and packets bound to a candidate digest, a
  contract digest and a protocol version.
- Post-hoc detection of a read-only role modifying the candidate.
- Declared role identities, recorded with every report.
- A gate that fails closed on any inconsistency it can observe.

What it does not provide:

- **Tamper resistance.** The hash chain detects accidental corruption, edits
  to a line, removed or reordered lines, and a ledger cut off in the middle of
  a command's batch of events. It does not detect a ledger rolled back to the
  end of an earlier command, and it is not anchored to anything outside the
  state directory, so an actor who can write the ledger can rewrite it
  consistently. Git history and review of `.metron/state/` are the only
  external check.
- **Reviewer identity.** `author.id` is whatever the submitter declares. A
  report carries the digest of the packet it answers, which shows that it was
  written for that exact packet and not for another ledger or an earlier
  issue. It does not show who wrote it. Role separation is therefore a rule
  cooperating parties follow, not a guarantee.
- **Protected evaluation.** Whoever runs `metron gate` can also run a modified
  binary. The exit status is only as trustworthy as the environment it ran in.
- **Role isolation.** An agent with shell access can request any packet and
  submit any report.

No keys are needed and none are managed.

## Authenticated enforcement mode (planned)

Authenticated mode keeps every local-mode mechanism and adds the properties
required to use the gate against agents that are not trusted.

| Property | Requirement |
| --- | --- |
| Authenticated provenance | A report is accepted only with an attestation, verified against trust anchors, that it was produced under an identity authorized for its role |
| Protected state and contract | The state directory and the pinned contract are writable only by the trusted environment, never by agents |
| Checkpoints and rollback protection | Signed ledger checkpoints, held outside agent control, let the gate reject a truncated or rewritten ledger |
| Protected gate evaluation | The gate runs in the trusted environment, from a known binary |
| Role isolation | Implementation and review roles run with separate credentials and cannot act for each other |

### The authentication boundary

The boundary is fixed now so that adding this mode does not change the
protocol.

- **What is attested** is the raw digest of the report file exactly as
  submitted. No canonicalization is involved, so any tool that signs a blob can
  produce an attestation.
- **Who attests** is the trusted execution environment that ran the role, for
  example a CI job. Signing credentials are never given to an agent.
- **Who verifies** is Metron, at `metron ingest`, before the semantic and transition
  layers. A report without a valid attestation is rejected like any other
  malformed report.
- **What is recorded** is the `provenance` member of the `report.accepted`
  event. In local mode it is `{"mode":"declared"}`. Authenticated mode adds a
  second mode carrying the verified identity and mechanism.
- **What the gate adds** is that condition V also requires the GREEN report's
  provenance to be authenticated for the reviewer role.

Ed25519 from the Go standard library is the default mechanism. Verification
sits behind a narrow interface so that existing CI identity and signing
infrastructure can be used instead. Metron will not define new cryptographic
constructions.
