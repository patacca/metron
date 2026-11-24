# 0003: Qualification of external integrations

Status: research complete, integration not started. Facts as of 2026-11-24.

Every candidate below is optional and replaceable. None is a dependency of the
core. Versions must be pinned when an integration is built, and each item
marked *unverified* must be tested first.

## Agent Client Protocol (ACP)

The editor-to-agent protocol that originated at Zed
(<https://agentclientprotocol.com>). Not the unrelated IBM/BeeAI "Agent
Communication Protocol".

- License Apache-2.0. Latest stable schema `schema-v1.35.0` (2026-11-23).
  Several releases per week; v1 grows additively.
- Transport: the client launches the agent as a subprocess and exchanges
  newline-delimited JSON-RPC over stdio.
- One headless turn in v1: `initialize`, `session/new`, `session/prompt`,
  streamed `session/update` notifications, optional `session/request_permission`
  requests the client must answer, then a `session/prompt` response carrying
  `stopReason`.
- **v2 is in preview** (2026-11-20) and changes the turn shape: completion
  arrives as a state update instead of the prompt response. Pin v1.
- **Permissions are advisory.** The specification states that the agent
  performs its own operations and is responsible for honouring the decision,
  and that ACP does no sandboxing. Capability flags (`fs.writeTextFile`,
  `terminal`) are likewise requests.
- Go client library: `github.com/coder/acp-go-sdk` v0.20.0, Apache-2.0, still
  0.x with breaking changes. Not needed if an external client is used.

## acpx

A headless command-line ACP client (<https://github.com/openclaw/acpx>).

- v0.22.0 (2026-11-07), MIT, pre-1.0 with roughly weekly releases. Needs
  Node.js 22.13 or later. Agent adapters are fetched through `npx` on demand,
  which is a supply-chain and availability concern to pin around.
- One-shot run: `acpx [options] <agent> exec -f <prompt-file>`.
- `--format json --json-strict` emits raw ACP messages as NDJSON; `--format
  quiet` prints only the final text; the default text format is not stable.
- Exit status is documented as stable: 0 success, 1 agent or runtime error,
  2 usage, 3 timeout, 4 no session, 5 permission denied, 130 interrupted.
- Restriction flags: `--approve-reads`, `--deny-all`, `--no-fs`,
  `--no-terminal`, `--timeout`, `--cwd`. Its own documentation says these are
  not a sandbox.
- Its flows API (`acpx flow run`) is labelled experimental. Do not use it.
- Oh My Pi is not a built-in agent profile; use `--agent 'omp acp'`.

## Oh My Pi (OMP)

A coding-agent harness (<https://github.com/can1357/oh-my-pi>), a fork of
`badlogic/pi-mono`.

- v19.1.1 (2026-11-23), MIT. Releases every one to two days, with major
  versions weekly. Treat its event formats as unstable.
- ACP agent: `omp acp`. Other headless modes: `omp -p "<prompt>"`, and
  `omp -p --mode json` for JSON-lines events.
- **The default approval mode auto-approves everything.** Always pass tool and
  approval restrictions explicitly (`--no-tools --tools ...`,
  `--approval-mode ...`). The exact read-only tool names are *unverified*.

## Dagu

A single-binary YAML workflow engine (<https://github.com/dagucloud/dagu>).

- v2.27.0 (2026-11-16). **GPL-3.0-or-later.** Its own licensing notes
  distinguish running the CLI as a separate program from importing its Go API.
  Metron may ship YAML templates and invoke the `dagu` executable. Metron must
  never import its Go packages.
- Headless run: `dagu start <dag.yaml> -- params`. Whether a failed run makes
  `dagu start` exit non-zero is *unverified*.
- Relevant schema features, v2 syntax: `run:` steps, `output:` capture with
  `decode: json`, `preconditions`, `continue_on.exit_code`, `retry_policy`, and
  `repeat_policy` with `limit` and `on_limit: fail` for bounded loops.
- The v1 step syntax (`command:`) still loads but is legacy. Validate
  templates with `dagu validate` in tests.

## Consequences for Metron's interface

- The smallest interface that serves all three mechanisms is file exchange:
  Metron writes a packet, one process runs with a working directory and a
  timeout, and Metron reads a report file. Manual exchange is the same contract
  with a person as the process.
- The report file, validated against Metron's schema, is the only
  authoritative output. An agent's final message is informational.
- No integration can enforce a read-only role. Metron checks the candidate
  digest and working tree after every read-only action, and rejects the report
  if either changed.
- Provenance worth capturing from an ACP run: the agent's name and version from
  `initialize`, the session identifier, and the model.
