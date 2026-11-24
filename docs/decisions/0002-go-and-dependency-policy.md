# 0002: Go, and a two-dependency budget

Status: accepted, 2026-11-25

## Decision

Metron is written in Go and distributed as a self-contained, CGO-free
executable. Adopters need no Go, Python or Node.js runtime. The only runtime
requirement is `git`.

The standard library is preferred wherever practical. The command line uses
`flag`, hashing uses `crypto/sha256`, templates use `text/template`, and git
is driven through its executable with `os/exec`.

Two third-party modules are approved because the standard library has no
equivalent and the requirement is real:

| Module | Version | License | Why | Footprint |
| --- | --- | --- | --- | --- |
| `github.com/santhosh-tekuri/jsonschema/v6` | v6.0.3 | Apache-2.0 | The JSON Schemas are the canonical public protocol, so the binary must validate against the very files it publishes | One transitive module, `golang.org/x/text` (BSD-3-Clause) |
| `go.yaml.in/yaml/v3` | v3.0.6 | MIT and Apache-2.0 | The contract is human-authored and needs comments | None |

Alternatives considered:

- `github.com/google/jsonschema-go` (v0.4.3): no runtime dependencies, but
  still 0.x with a moving API.
- `gopkg.in/yaml.v3`: unmaintained since 2022; `go.yaml.in/yaml/v3` is its
  maintained continuation under the YAML organization.
- `go.yaml.in/yaml/v4` (v4.0.0, released 2026-11-04): too new to pin.
- `github.com/goccy/go-yaml`: no dependencies, but a different API with less
  direct control over the node tree that strict parsing relies on.

## Consequences

- JSON Schemas under `schemas/` are versioned and language-independent. Go
  types mirror them, and tests keep the two in agreement.
- Structural validation, semantic validation and state-transition enforcement
  are separate layers with separate tests.
- The state machine and gate are pure domain logic, tested with unit tests,
  fuzzing and invariant checks over generated command sequences.
- A new dependency needs a record here covering maintenance, license,
  transitive footprint and cross-platform behaviour.
- No performance work or concurrency is introduced without a measured need.
