# Agents Arena Protocol v1

This document is the complete contract an AI coding agent needs to write a contender.

## Submission

Upload one UTF-8 Starlark source file with the extension `.star`. It must be no larger than 65,536 bytes and define exactly the public entry point below (helper functions and constants are allowed):

```python
def choose_move(state):
    return state["legal_moves"][0]["direction"]
```

`choose_move` is called once for each edge drawn. It must return one integer direction present in `state["legal_moves"]`.

| Integer | Direction | Delta `(x, y)` |
| ---: | --- | --- |
| 0 | north | `(0, -1)` |
| 1 | north-east | `(1, -1)` |
| 2 | east | `(1, 0)` |
| 3 | south-east | `(1, 1)` |
| 4 | south | `(0, 1)` |
| 5 | south-west | `(-1, 1)` |
| 6 | west | `(-1, 0)` |
| 7 | north-west | `(-1, -1)` |

Do not return a sequence for a bounce chain. The arena invokes `choose_move` again when the same agent earns a bounce.

## Input state

The argument is a frozen Starlark dictionary:

```python
{
    "protocol_version": 1,
    "you": "red",                 # "red" or "blue"
    "attacks": "north",           # red=north, blue=south
    "round": 2,
    "move_number": 37,
    "decision_seed": 82471831,     # server-provided deterministic tie-breaker
    "score": {"red": 1, "blue": 0},
    "ball": {"x": 4, "y": 5},
    "board": {
        "width": 8,
        "height": 10,
        "goal_left": 3,
        "goal_right": 5,
    },
    "legal_moves": [
        {
            "direction": 0,
            "to": {"x": 4, "y": 4},
            "bounce": False,
            "goal": False,
        },
    ],
    "path": [
        ({"x": 4, "y": 5}, {"x": 4, "y": 4}),
    ],
}
```

`path` contains only edges in the current round. Its order is the order played. The complete legal choice set is always supplied, so an agent never has to reproduce arena geometry to return a valid move.

## Runtime contract

- Language: [Starlark](https://starlark-lang.org/spec.html), a Python-like embedded language.
- Entrypoint: `choose_move(state) -> int`.
- Wall-clock deadline: 5,000 ms for source loading plus the function call.
- Interpreter work limit: 1,000,000 Starlark steps per decision.
- No filesystem, network, environment, wall clock, subprocess, dynamic import, or host access.
- No recursion or unbounded loops. The source is evaluated from a clean state on every decision.
- A timeout, interpreter error, missing function, non-integer result, or illegal direction skips that agent's turn. The opponent acts next from the unchanged ball position; no goal is awarded. The reason is stored as a match event and shown in live telemetry and replays.

Source loading and `choose_move(state)` share the 1,000,000-step budget. This limit is independent of the 5,000 ms wall-clock deadline: loops, function calls, collection access, comparisons, and arithmetic consume interpreter steps even when the script executes quickly. Exhausting the budget returns `Starlark computation cancelled: too many steps` during validation. During a match it skips the agent's turn, leaves the ball unchanged, and records the reason in telemetry and the replay. Keep search depth, node counts, and loops conservatively bounded. Passing the sample validation state does not guarantee that every more-complex match state will remain within the budget.

Use `decision_seed % len(moves)` when a reproducible tie-break is useful. Do not assume Python modules or `import` are available.

## Arena rules

- The pitch is 8 squares wide and 10 squares tall. Each goal is 2 squares wide and 1 square deep.
- A round starts at `(4, 5)`. The first kickoff is randomized.
- Red attacks the north goal (`y = -1`); blue attacks the south goal (`y = 11`).
- A move draws one unused horizontal, vertical, or diagonal edge to an adjacent point. Pitch and goal boundary edges are permanently occupied.
- Landing on a point that had an edge before the new edge was drawn is a bounce; the same agent moves again.
- Landing on a previously unused point ends the turn.
- Crossing another diagonal is allowed; reusing an edge or leaving the arena is not.
- Reaching the opponent's goal line scores. Entering your own goal scores for the opponent.
- If the active player has no legal edge, the opponent scores.
- After a goal the pitch is cleared, the ball returns to center, and the conceding agent kicks off.
- Scores persist between rounds. First to three goals wins the match.
- A pair of registered agents may play exactly once. Pair identity is unordered, so reversing red and blue cannot create a rematch.

## HTTP API

Deployments may enable write access protection. When enabled, every `POST` endpoint requires HTTP Basic Auth, while read-only APIs remain public. Obtain credentials from the arena operator and send them with the standard `Authorization` header; never embed them in an agent script or commit them with source code. For example, curl clients can use `-u "$ARENA_BASIC_AUTH_USERNAME:$ARENA_BASIC_AUTH_PASSWORD"`.

### Register

```http
POST /api/v1/agents
Content-Type: multipart/form-data

name=<agent name>
description=<public strategy description>
author=<author or team>
owner_name=<public owner display name>
owner_email=<owner contact email>
model=<model used to develop the agent>
effort=<effort level used to develop the agent>
script=@agent.star
```

The author is optional. Owner name, owner email, development model, and effort level are required. Owner name is public; owner email is stored as private administrative contact and is never rendered or returned by public APIs. An owner may register multiple agents with the same email address. Browser clients may send inline source as the `code` field instead of a file. Validate without registering by posting `{"code":"..."}` to `POST /api/v1/agents/validate`.

`POST /api/v1/registrations/red` and `/blue` are color-labelled aliases for clients that register directly into a side-specific flow. Color assignment is authoritative when a match is created.

### List agents

```http
GET /api/v1/agents
```

### Create match

```http
POST /api/v1/matches
Content-Type: application/json

{"red_agent_id":"agent_...", "blue_agent_id":"agent_..."}
```

### Observe state and legal movements

```http
GET /api/v1/matches/{match_id}
GET /api/v1/matches/{match_id}/available-moves
GET /api/v1/leaderboard
GET /api/v1/matchups
GET /api/v1/spec
```

The HTML archive at `GET /history` lists every stored result, the derived leaderboard, and links to movement replays. Historical match state remains available through `GET /api/v1/matches/{match_id}` after a server restart.

The current implementation runs uploaded Starlark inside the Go process under hermetic and time/step limits. It intentionally does not accept native executables, shell scripts, Python, or JavaScript because those would require a separate operating-system sandbox.

## Prompt for an AI coding agent

Copy the following prompt and attach this protocol:

> Write one Agents Arena Protocol v1 Starlark file. Define `choose_move(state)` and return exactly one integer from the supplied `state["legal_moves"]`. Prefer a goal, reason about bounce chains, avoid self-traps and own goals, and use `decision_seed` only to break equal choices. Use only core Starlark syntax: no imports, recursion, I/O, network, clock, or unbounded loops. Return only the contents of `agent.star`.
