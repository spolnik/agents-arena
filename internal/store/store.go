package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jacek/agents-arena/internal/arena"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;`); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    author TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
	owner_email TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	effort TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS matches (
    id TEXT PRIMARY KEY,
    red_agent_id TEXT NOT NULL REFERENCES agents(id),
    blue_agent_id TEXT NOT NULL REFERENCES agents(id),
	pair_key TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    red_score INTEGER NOT NULL DEFAULT 0,
    blue_score INTEGER NOT NULL DEFAULT 0,
    winner TEXT NOT NULL DEFAULT '',
    round INTEGER NOT NULL DEFAULT 1,
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS moves (
    match_id TEXT NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    number INTEGER NOT NULL,
    round INTEGER NOT NULL,
    player TEXT NOT NULL,
    from_x INTEGER NOT NULL,
    from_y INTEGER NOT NULL,
    to_x INTEGER NOT NULL,
    to_y INTEGER NOT NULL,
    direction INTEGER NOT NULL,
    bounced INTEGER NOT NULL,
    goal INTEGER NOT NULL,
    duration_ns INTEGER NOT NULL,
    occurred_at TEXT NOT NULL,
    PRIMARY KEY (match_id, number)
);
CREATE TABLE IF NOT EXISTS match_events (
    match_id TEXT NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    number INTEGER NOT NULL,
    type TEXT NOT NULL,
    round INTEGER NOT NULL,
    player TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL,
    red_score INTEGER NOT NULL,
    blue_score INTEGER NOT NULL,
    occurred_at TEXT NOT NULL,
    PRIMARY KEY (match_id, number)
);
CREATE INDEX IF NOT EXISTS matches_created_idx ON matches(created_at DESC);
`)
	if err != nil {
		return err
	}
	for _, statement := range []string{
		`ALTER TABLE agents ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN owner_email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN model TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN effort TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE matches ADD COLUMN pair_key TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.Exec(statement); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	if _, err := s.db.Exec(`
UPDATE matches
SET pair_key = CASE
    WHEN red_agent_id < blue_agent_id THEN red_agent_id || ':' || blue_agent_id
    ELSE blue_agent_id || ':' || red_agent_id
END
WHERE pair_key = '' AND id IN (
    SELECT MIN(id)
    FROM matches
    GROUP BY CASE
        WHEN red_agent_id < blue_agent_id THEN red_agent_id || ':' || blue_agent_id
        ELSE blue_agent_id || ':' || red_agent_id
    END
);
CREATE UNIQUE INDEX IF NOT EXISTS matches_pair_key_unique
ON matches(pair_key) WHERE pair_key <> '';
`); err != nil {
		return err
	}
	return nil
}

func (s *Store) CreateAgent(name, author, description, ownerEmail, model, effort, source string) (arena.Agent, error) {
	agent := arena.Agent{
		ID: id("agent"), Name: name, Author: author, Description: description,
		OwnerEmail: ownerEmail, Model: model, Effort: effort, Source: source, CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.Exec(`INSERT INTO agents(id,name,author,description,owner_email,model,effort,source,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		agent.ID, agent.Name, agent.Author, agent.Description, agent.OwnerEmail, agent.Model, agent.Effort, agent.Source, agent.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return arena.Agent{}, err
	}
	return agent, nil
}

func (s *Store) GetAgent(id string) (arena.Agent, error) {
	var agent arena.Agent
	var created string
	err := s.db.QueryRow(`SELECT id,name,author,description,owner_email,model,effort,source,created_at FROM agents WHERE id=?`, id).
		Scan(&agent.ID, &agent.Name, &agent.Author, &agent.Description, &agent.OwnerEmail, &agent.Model, &agent.Effort, &agent.Source, &created)
	if err != nil {
		return arena.Agent{}, err
	}
	agent.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return agent, nil
}

func (s *Store) ListAgents() ([]arena.Agent, error) {
	rows, err := s.db.Query(`SELECT id,name,author,description,owner_email,model,effort,created_at FROM agents ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agents := make([]arena.Agent, 0)
	for rows.Next() {
		var agent arena.Agent
		var created string
		if err := rows.Scan(&agent.ID, &agent.Name, &agent.Author, &agent.Description, &agent.OwnerEmail, &agent.Model, &agent.Effort, &created); err != nil {
			return nil, err
		}
		agent.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

type MatchRow struct {
	ID             string
	RedName        string
	RedAuthor      string
	RedOwnerEmail  string
	RedModel       string
	RedEffort      string
	BlueName       string
	BlueAuthor     string
	BlueOwnerEmail string
	BlueModel      string
	BlueEffort     string
	Status         string
	RedScore       int
	BlueScore      int
	Winner         string
	CreatedAt      time.Time
}

type LeaderboardRow struct {
	Rank         int    `json:"rank"`
	AgentID      string `json:"agent_id"`
	Name         string `json:"name"`
	Author       string `json:"author"`
	OwnerEmail   string `json:"owner_email"`
	Model        string `json:"model"`
	Effort       string `json:"effort"`
	Played       int    `json:"played"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	GoalsFor     int    `json:"goals_for"`
	GoalsAgainst int    `json:"goals_against"`
	GoalDiff     int    `json:"goal_difference"`
	Points       int    `json:"points"`
	WinPercent   int    `json:"win_percent"`
}

type PlayedPair struct {
	AgentAID string `json:"agent_a_id"`
	AgentBID string `json:"agent_b_id"`
	MatchID  string `json:"match_id"`
}

func (s *Store) ListMatches(limit int) ([]MatchRow, error) {
	rows, err := s.db.Query(`
SELECT m.id,
       r.name,r.author,r.owner_email,r.model,r.effort,
       b.name,b.author,b.owner_email,b.model,b.effort,
       m.status,m.red_score,m.blue_score,m.winner,m.created_at
FROM matches m JOIN agents r ON r.id=m.red_agent_id JOIN agents b ON b.id=m.blue_agent_id
ORDER BY m.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]MatchRow, 0)
	for rows.Next() {
		var row MatchRow
		var created string
		if err := rows.Scan(
			&row.ID,
			&row.RedName, &row.RedAuthor, &row.RedOwnerEmail, &row.RedModel, &row.RedEffort,
			&row.BlueName, &row.BlueAuthor, &row.BlueOwnerEmail, &row.BlueModel, &row.BlueEffort,
			&row.Status, &row.RedScore, &row.BlueScore, &row.Winner, &created,
		); err != nil {
			return nil, err
		}
		row.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) Leaderboard() ([]LeaderboardRow, error) {
	rows, err := s.db.Query(`
WITH results AS (
    SELECT red_agent_id AS agent_id, red_score AS goals_for, blue_score AS goals_against,
           CASE WHEN winner='red' THEN 1 ELSE 0 END AS won
    FROM matches WHERE status='finished'
    UNION ALL
    SELECT blue_agent_id, blue_score, red_score,
           CASE WHEN winner='blue' THEN 1 ELSE 0 END
    FROM matches WHERE status='finished'
)
SELECT a.id, a.name, a.author, a.owner_email, a.model, a.effort,
       COUNT(r.agent_id), COALESCE(SUM(r.won), 0),
       COALESCE(SUM(r.goals_for), 0), COALESCE(SUM(r.goals_against), 0)
FROM agents a
LEFT JOIN results r ON r.agent_id=a.id
GROUP BY a.id, a.name, a.author, a.owner_email, a.model, a.effort
ORDER BY COALESCE(SUM(r.won), 0) DESC,
         COALESCE(SUM(r.goals_for-r.goals_against), 0) DESC,
         COALESCE(SUM(r.goals_for), 0) DESC,
         a.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	leaders := make([]LeaderboardRow, 0)
	for rows.Next() {
		var row LeaderboardRow
		if err := rows.Scan(
			&row.AgentID, &row.Name, &row.Author, &row.OwnerEmail, &row.Model, &row.Effort,
			&row.Played, &row.Wins, &row.GoalsFor, &row.GoalsAgainst,
		); err != nil {
			return nil, err
		}
		row.Rank = len(leaders) + 1
		row.Losses = row.Played - row.Wins
		row.GoalDiff = row.GoalsFor - row.GoalsAgainst
		row.Points = row.Wins * 3
		if row.Played > 0 {
			row.WinPercent = row.Wins * 100 / row.Played
		}
		leaders = append(leaders, row)
	}
	return leaders, rows.Err()
}

func (s *Store) ListPlayedPairs() ([]PlayedPair, error) {
	rows, err := s.db.Query(`
SELECT CASE WHEN red_agent_id < blue_agent_id THEN red_agent_id ELSE blue_agent_id END,
       CASE WHEN red_agent_id < blue_agent_id THEN blue_agent_id ELSE red_agent_id END,
       MIN(id)
FROM matches
GROUP BY CASE WHEN red_agent_id < blue_agent_id THEN red_agent_id ELSE blue_agent_id END,
         CASE WHEN red_agent_id < blue_agent_id THEN blue_agent_id ELSE red_agent_id END
ORDER BY MIN(created_at)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pairs := make([]PlayedPair, 0)
	for rows.Next() {
		var pair PlayedPair
		if err := rows.Scan(&pair.AgentAID, &pair.AgentBID, &pair.MatchID); err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}
	return pairs, rows.Err()
}

func (s *Store) GetMatch(id string) (*arena.Game, error) {
	game := &arena.Game{}
	var status, winner, created string
	err := s.db.QueryRow(`
SELECT m.id,
       r.id,r.name,r.author,r.owner_email,r.model,r.effort,
       b.id,b.name,b.author,b.owner_email,b.model,b.effort,
       m.status,m.red_score,m.blue_score,
       m.winner, m.round, m.message, m.created_at
FROM matches m
JOIN agents r ON r.id=m.red_agent_id
JOIN agents b ON b.id=m.blue_agent_id
WHERE m.id=?`, id).Scan(
		&game.ID,
		&game.RedAgent.ID, &game.RedAgent.Name, &game.RedAgent.Author, &game.RedAgent.OwnerEmail, &game.RedAgent.Model, &game.RedAgent.Effort,
		&game.BlueAgent.ID, &game.BlueAgent.Name, &game.BlueAgent.Author, &game.BlueAgent.OwnerEmail, &game.BlueAgent.Model, &game.BlueAgent.Effort,
		&status, &game.RedScore, &game.BlueScore, &winner, &game.Round, &game.LastMessage, &created,
	)
	if err != nil {
		return nil, err
	}
	game.Status = arena.Status(status)
	game.Winner = arena.Color(winner)
	game.Ball = arena.Point{X: 4, Y: 5}
	game.Turn = arena.Red
	game.Moves = make([]arena.Move, 0)
	game.Events = make([]arena.MatchEvent, 0)

	rows, err := s.db.Query(`
SELECT number,round,player,from_x,from_y,to_x,to_y,direction,bounced,goal,duration_ns,occurred_at
FROM moves WHERE match_id=? ORDER BY number`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var move arena.Move
		var player, occurred string
		if err := rows.Scan(&move.Number, &move.Round, &player, &move.From.X, &move.From.Y, &move.To.X, &move.To.Y,
			&move.Direction, &move.Bounce, &move.Goal, &move.Duration, &occurred); err != nil {
			return nil, err
		}
		move.Player = arena.Color(player)
		move.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
		game.Moves = append(game.Moves, move)
		game.MoveNumber = move.Number
		if move.Round == game.Round {
			game.Path = append(game.Path, arena.Edge{From: move.From, To: move.To})
			game.Ball = move.To
		}
		if move.Bounce {
			game.Turn = move.Player
		} else {
			game.Turn = move.Player.Opponent()
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	eventRows, err := s.db.Query(`
SELECT number,type,round,player,message,red_score,blue_score,occurred_at
FROM match_events WHERE match_id=? ORDER BY number`, id)
	if err != nil {
		return nil, err
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var event arena.MatchEvent
		var player, occurred string
		if err := eventRows.Scan(&event.Number, &event.Type, &event.Round, &player, &event.Message,
			&event.RedScore, &event.BlueScore, &occurred); err != nil {
			return nil, err
		}
		event.Player = arena.Color(player)
		event.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
		game.Events = append(game.Events, event)
	}
	if err := eventRows.Err(); err != nil {
		return nil, err
	}
	return game, nil
}

func (s *Store) CreateMatch(game *arena.Game) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`INSERT INTO matches(id,red_agent_id,blue_agent_id,pair_key,status,red_score,blue_score,winner,round,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		game.ID, game.RedAgent.ID, game.BlueAgent.ID, pairKey(game.RedAgent.ID, game.BlueAgent.ID), game.Status, game.RedScore, game.BlueScore, game.Winner, game.Round, game.LastMessage, now, now)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "matches.pair_key") {
		return arena.ErrPairAlreadyPlayed
	}
	return err
}

func pairKey(first, second string) string {
	if first > second {
		first, second = second, first
	}
	return first + ":" + second
}

func (s *Store) SaveGame(game *arena.Game, move *arena.Move, event *arena.MatchEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE matches SET status=?,red_score=?,blue_score=?,winner=?,round=?,message=?,updated_at=? WHERE id=?`,
		game.Status, game.RedScore, game.BlueScore, game.Winner, game.Round, game.LastMessage, time.Now().UTC().Format(time.RFC3339Nano), game.ID)
	if err != nil {
		return err
	}
	if move != nil {
		_, err = tx.Exec(`INSERT INTO moves(match_id,number,round,player,from_x,from_y,to_x,to_y,direction,bounced,goal,duration_ns,occurred_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			game.ID, move.Number, move.Round, move.Player, move.From.X, move.From.Y, move.To.X, move.To.Y,
			move.Direction, move.Bounce, move.Goal, move.Duration.Nanoseconds(), move.OccurredAt.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
	}
	if event != nil {
		_, err = tx.Exec(`INSERT INTO match_events(match_id,number,type,round,player,message,red_score,blue_score,occurred_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			game.ID, event.Number, event.Type, event.Round, event.Player, event.Message,
			event.RedScore, event.BlueScore, event.OccurredAt.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SeedExamples() error {
	for _, seed := range []struct{ name, author, description, ownerEmail, model, effort, source string }{
		{"North Star", "Arena Labs", "Pushes relentlessly toward the opposing goal and takes every direct scoring chance.", "jacek@engineering.ai", "OpenAI GPT-5 Codex", "high", northStarScript},
		{"Bounce Hunter", "Arena Labs", "Prefers connected vertices to extend turns, then uses a seeded tie-break between equal moves.", "jacek@engineering.ai", "OpenAI GPT-5 Codex", "high", bounceHunterScript},
		{"Full Press", "Arena Labs", "A full-forward strategy that maximizes goalward progress, stays central, takes scoring edges immediately, and extends attacks through bounces.", "jacek@engineering.ai", "OpenAI GPT-5 Codex", "high", fullPressScript},
		{"Iron Curtain", "Arena Labs", "A blocking strategy that clears its defensive half, consumes connected vertices, pressures the sidelines, and uses bounces to close lanes.", "jacek@engineering.ai", "OpenAI GPT-5 Codex", "high", ironCurtainScript},
	} {
		if err := arena.ValidateScript(seed.source); err != nil {
			return fmt.Errorf("validate seeded agent %s: %w", seed.name, err)
		}
		var existing int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE name=?`, seed.name).Scan(&existing); err != nil {
			return err
		}
		if existing > 0 {
			if _, err := s.db.Exec(`UPDATE agents SET
description=CASE WHEN description='' THEN ? ELSE description END,
owner_email=CASE WHEN owner_email='' THEN ? ELSE owner_email END,
model=CASE WHEN model='' THEN ? ELSE model END,
effort=CASE WHEN effort='' THEN ? ELSE effort END
WHERE name=?`, seed.description, seed.ownerEmail, seed.model, seed.effort, seed.name); err != nil {
				return err
			}
			continue
		}
		if _, err := s.CreateAgent(seed.name, seed.author, seed.description, seed.ownerEmail, seed.model, seed.effort, seed.source); err != nil {
			return err
		}
	}
	return nil
}

func id(prefix string) string {
	var value [8]byte
	_, _ = rand.Read(value[:])
	return prefix + "_" + hex.EncodeToString(value[:])
}

func IsConflict(err error) bool {
	return err != nil && (errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "UNIQUE constraint failed"))
}

const northStarScript = `def choose_move(state):
    moves = state["legal_moves"]
    attacks_north = state["attacks"] == "north"
    best = moves[0]
    for move in moves:
        if move["goal"]:
            return move["direction"]
        if attacks_north and move["to"]["y"] < best["to"]["y"]:
            best = move
        if not attacks_north and move["to"]["y"] > best["to"]["y"]:
            best = move
    return best["direction"]
`

const bounceHunterScript = `def choose_move(state):
    moves = state["legal_moves"]
    for move in moves:
        if move["goal"]:
            return move["direction"]
    for move in moves:
        if move["bounce"]:
            return move["direction"]
    seed = state["decision_seed"]
    return moves[seed % len(moves)]["direction"]
`

const fullPressScript = `def choose_move(state):
    moves = state["legal_moves"]
    north = state["attacks"] == "north"
    best = moves[0]
    best_score = -1000000
    for move in moves:
        x = move["to"]["x"]
        y = move["to"]["y"]
        scores_for_us = (north and y == -1) or (not north and y == 11)
        scores_against_us = (north and y == 11) or (not north and y == -1)
        center_distance = x - 4
        if center_distance < 0:
            center_distance = -center_distance
        progress = -y if north else y
        score = progress * 100 - center_distance * 8
        if move["bounce"]:
            score += 35
        if scores_for_us:
            score += 100000
        if scores_against_us:
            score -= 100000
        if score > best_score:
            best = move
            best_score = score
    return best["direction"]
`

const ironCurtainScript = `def point_matches(point, x, y):
    return point["x"] == x and point["y"] == y

def choose_move(state):
    moves = state["legal_moves"]
    north = state["attacks"] == "north"
    ball_y = state["ball"]["y"]
    in_danger = ball_y >= 5 if north else ball_y <= 5
    best = moves[0]
    best_score = -1000000
    for move in moves:
        x = move["to"]["x"]
        y = move["to"]["y"]
        scores_for_us = (north and y == -1) or (not north and y == 11)
        scores_against_us = (north and y == 11) or (not north and y == -1)
        clearance = 10 - y if north else y
        edge_pressure = x - 4
        if edge_pressure < 0:
            edge_pressure = -edge_pressure
        connections = 0
        for edge in state["path"]:
            if point_matches(edge[0], x, y) or point_matches(edge[1], x, y):
                connections += 1
        score = clearance * (110 if in_danger else 45)
        score += edge_pressure * 22
        score += connections * 28
        if move["bounce"]:
            score += 90
        if scores_for_us:
            score += 100000
        if scores_against_us:
            score -= 100000
        if score > best_score:
            best = move
            best_score = score
    return best["direction"]
`
