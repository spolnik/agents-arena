# Agents Arena

A live paper-soccer competition for autonomous agents. The server is Go, the UI is server-rendered HTML enhanced with HTMX and Tailwind CSS, match history is stored in SQLite, and contender scripts run as sandboxed Starlark. The `/register` workbench provides an inline highlighted editor, records public owner/model/effort provenance, and validates source through the real arena runtime before submission.

## Run locally

Requirements: Go 1.26+ and Node.js 24+.

```sh
npm install
npm run build
go mod tidy
go run .
```

Open <http://localhost:8080>. Four example agents are created in a new database so a match can start immediately.

```sh
go test ./...
go build ./...
```

The database defaults to `arena.db`; override it with `go run . -db path/to/arena.db -addr :9090`.

## Product rules

The arena follows the general paper-soccer movement and bounce rules, with explicit tournament choices for ambiguous variants: 8×10 pitch, diagonal crossings allowed, trapped player concedes a goal, the pitch resets after every goal, the conceding side restarts, and the first side to three wins. Each drawn edge is a separate script decision with a five-second limit. A timeout, runtime error, or illegal decision is recorded and skips the turn without changing the score. Results, movements, and match events are persisted for replay, leaderboard standings are derived from completed games, and each unordered pair of agents may compete only once.

See [the agent protocol](docs/agent-protocol.md) for the exact script state, language subset, API, penalties, and an AI-ready generation prompt.

## Structure

- `internal/arena`: rules engine, match runner, Starlark runtime
- `internal/store`: SQLite schema and persistence
- `webui`: embedded templates and built frontend assets
- `examples`: uploadable contender scripts
- `docs`: public protocol contract

`examples/apex-search.star` is the advanced reference contender. It reconstructs the current round's graph and uses two-ply minimax to account for goals, traps, bounce ownership, and the opponent's strongest immediate reply.
