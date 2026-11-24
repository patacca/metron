# Working on Metron

Metron is a standalone Go CLI that governs engineering review rounds. Read
`docs/architecture.md` before changing behaviour: it is normative, and a
change in semantics starts there. `docs/trust-model.md` states what Metron may
and may not claim about security.

## Rules

- **Layering.** `internal/core` and `internal/canon` import only the standard
  library and perform no I/O: no files, no clock, no environment, no git, no
  randomness. Everything observed from outside enters the core as an argument.
  `store`, `schema`, `vcs` and `protocol` may import `core` and `canon`, never
  each other's internals and never `cli`.
- **Determinism.** The same inputs give the same bytes. Never let map
  iteration order reach an output, an event list or an error message.
- **Fail closed.** An error, a missing file, an unknown value or an
  unexpected state never results in approval. When unsure, reject and say why.
- **Dependencies.** The standard library first. The only approved third-party
  modules are `github.com/santhosh-tekuri/jsonschema/v6` and
  `go.yaml.in/yaml/v3`. Adding another needs a record in `docs/decisions/`.
  Do not edit `go.mod` or run `go mod tidy` as part of unrelated work.
- **Portability.** Builds are CGO-free and must compile for Linux, macOS and
  Windows. Platform-specific code goes in build-tagged files. Use
  `path/filepath` for file system paths and forward slashes for paths inside
  git and inside documents.
- **No placeholders.** No stubs, no `TODO` branches, no skipped tests. If
  something is out of scope, leave it out entirely.
- **Proportion.** No abstraction without a second user, no concurrency without
  a measured need, no option without a requirement. This project reviews
  others for over-engineering.

## Conventions

- JSON members are `snake_case`. Decode strictly: unknown members are errors.
- Identifiers a user sees (stages, tiers, domains, event types) are the
  constants in `internal/core`; never retype the strings.
- An error message says what is wrong and what to do next.
- Tests are table-driven and use the standard `testing` package only. Parsers
  and the state machine have fuzz targets. Tests never need the network.
- `just check` (gofmt, vet, tests) passes before work is considered done.
  Tasks live in the `justfile`; there is no Makefile.
- Commit messages follow Conventional Commits. No `Co-Authored-By` trailers.
