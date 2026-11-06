# ADR: the Agent and Terminal boundary

Decided by the architect, 2026-11-03 (roadmap P5-1). Status: settled. Builds on D2, D4 and D7 in
[vendor-extension-decisions.md](vendor-extension-decisions.md) and on `design/03-vendor-driver.md`
section 3 and `design/04-vendor-decoupling.md` section 2; those are not reopened here.

## Context

Vendor behavior was spread over `lifecycle.Driver`, `term.Driver`, `term.Turns` and per-vendor
functions in `team/vendors.go`. Adding a vendor meant touching all of them.

## Decision

- One interface, `agent.Agent`, in a new package `internal/agent` (interface and shared types only).
  It replaces the four pieces above.
- Implementations live in `internal/agent/claudecode`, later `agent/opencode`.
- **Terminal stays `term.Backend`, unchanged.** An Agent starts the CLI, reads its state, and puts
  messages in, using the Terminal it is handed to open sessions, send keys and read the screen.
- The split test: if it does not change when the terminal changes, it is Agent. If it does not
  change when the agent CLI changes, it is Terminal. If neither, it is a verdict
  ([adr-judgement-functions.md](adr-judgement-functions.md)).

## Layers

- `internal/agent` sits at the layer of `term` and may import `term`, nothing above it.
- `agent/claudecode` may import `claude`, `claudecli`, `peerinbox`, `peerport` and `turns`.
- `lifecycle` imports `agent` only, never an implementation. `compose` picks the implementation
  by vendor name.
- These rules go into the layers table ([adr-layer-rules.md](adr-layer-rules.md)).
- A message goes through the port of the agent's vendor (`Agent.Deliver`); Claude Code's peer inbox is one of them.
- An account whose vendor reports no usage is unmetered: it takes only the agents whose agent.yaml pins it (`account: <name>`), with no
  room test. A pinned agent runs only on its pin, of its own vendor, and nothing that picks or moves accounts puts it elsewhere.
- The fakes build (`-tags fakes`) adds vendor `fake` (`agent/fake`) and terminal `fake` (`term/fake.go`). The conformance run plays
  them with the tick on (`tools/test-go.sh --fakes`); the shipped binary carries neither.

## Consequences

- A new agent CLI implements `Agent`; a new terminal host implements `Backend`. Neither touches the other.
- Contributor-onboarding docs wait for the public repo.
