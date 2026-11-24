<h1 align="center">
  M E T R O N
  <br>
  <sub>Convergent Engineering Review Framework</sub>
</h1>

<p align="center"><em>Quality without excess. Progress without compromise.</em></p>

Metron decides whether a specific commit of your repository is approved for a
specific lifecycle stage, and it keeps the review process that gets you there
from looping forever.

It is a single deterministic executable. It does not run models. Humans and
agents supply the engineering judgment; Metron supplies the rules they cannot
bend: what must be reviewed, what may block, when a finding may be reopened,
when repeated failure must change the investigation, and when the gate is
open.

> **Status: under construction.** The design is settled and documented; the
> implementation is landing. Nothing here is released yet.

## Why

Agent-driven review loops tend to fail in one of two ways. Either the bar is
whatever the last reviewer felt like, or the bar is "no further improvement is
possible", which no codebase ever meets. Metron separates the two questions:

- *Is this candidate sufficiently correct, reliable, efficient, maintainable
  and safe for what it is about to be used for?* That is the gate.
- *Could it be better?* That is a backlog.

## Documentation

- [Architecture](docs/architecture.md): the normative design.
- [Trust model](docs/trust-model.md): what Metron does and does not guarantee.
- [Decisions](docs/decisions/): why it is built the way it is.

![Architecture](docs/assets/architecture.svg)

## Development

Requires Go (see `go.mod`), `git` and [`just`](https://just.systems).

```
just check    # gofmt, vet, tests
just build    # bin/metron
just fuzz     # run every fuzz target briefly
just cross    # CGO-free binaries for Linux, macOS and Windows
```

## License

Apache-2.0. See [LICENSE](LICENSE).
