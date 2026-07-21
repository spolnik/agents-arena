package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacek/agents-arena/internal/arena"
	_ "modernc.org/sqlite"
)

func TestOpenMigratesAgentMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE agents (
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
	INSERT INTO agents(id,name,author,description,owner_email,model,effort,source,created_at)
	VALUES(
		'agent_legacy', 'Legacy Agent', 'Legacy Team', 'Created before public owner names.',
		'legacy@example.com', 'OpenAI GPT-5 Codex', 'high',
		'def choose_move(state): return 0', '2026-01-01T00:00:00Z'
	);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacyAgent, err := db.GetAgent("agent_legacy")
	if err != nil {
		t.Fatal(err)
	}
	if legacyAgent.OwnerName != "Arena Labs" || legacyAgent.Model != "gpt-5.6-sol" {
		t.Fatalf("legacy provenance = owner %q, model %q", legacyAgent.OwnerName, legacyAgent.Model)
	}
	if legacyAgent.OwnerEmail != "legacy@example.com" {
		t.Fatalf("legacy owner email was not preserved: %q", legacyAgent.OwnerEmail)
	}
	agent, err := db.CreateAgent(
		"Migrated", "Test", "A persisted description.",
		"Arena Labs", "owner@example.com", "gpt-5.6-sol", "high",
		"def choose_move(state): return 0",
	)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := db.GetAgent(agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Description != "A persisted description." {
		t.Fatalf("description = %q", loaded.Description)
	}
	if loaded.OwnerName != "Arena Labs" || loaded.OwnerEmail != "owner@example.com" || loaded.Model != "gpt-5.6-sol" || loaded.Effort != "high" {
		t.Fatalf("provenance = owner %q, email %q, model %q, effort %q", loaded.OwnerName, loaded.OwnerEmail, loaded.Model, loaded.Effort)
	}
}

func TestSeedExamplesIncludeProvenance(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SeedExamples(); err != nil {
		t.Fatal(err)
	}
	agents, err := db.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 4 {
		t.Fatalf("seeded agents = %d", len(agents))
	}
	for _, agent := range agents {
		if agent.OwnerName != "Arena Labs" || agent.OwnerEmail != "jacek@engineering.ai" || agent.Model != "gpt-5.6-sol" || agent.Effort != "high" {
			t.Fatalf("%s provenance = owner %q, email %q, model %q, effort %q", agent.Name, agent.OwnerName, agent.OwnerEmail, agent.Model, agent.Effort)
		}
	}
}

func TestMatchPairCanOnlyPlayOnceAndBuildsLeaderboard(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "competition.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := "def choose_move(state): return state[\"legal_moves\"][0][\"direction\"]"
	red, err := db.CreateAgent("Red Test", "Tests", "Red test agent.", "Red Owner", "red@example.com", "test-model", "high", source)
	if err != nil {
		t.Fatal(err)
	}
	blue, err := db.CreateAgent("Blue Test", "Tests", "Blue test agent.", "Blue Owner", "blue@example.com", "test-model", "high", source)
	if err != nil {
		t.Fatal(err)
	}

	game := arena.NewGame("match_first", arena.AgentSummary{ID: red.ID, Name: red.Name}, arena.AgentSummary{ID: blue.ID, Name: blue.Name})
	if err := db.CreateMatch(game); err != nil {
		t.Fatal(err)
	}
	game.Status = arena.Running
	move, err := game.Apply(0, 125)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveGame(game, &move, nil); err != nil {
		t.Fatal(err)
	}
	event := game.SkipTurn(game.Turn, "Blue Test turn skipped: deadline exceeded")
	if err := db.SaveGame(game, nil, &event); err != nil {
		t.Fatal(err)
	}
	game.DecisionSeed = 987654321
	game.DecisionPending = true
	if err := db.SaveGame(game, nil, nil); err != nil {
		t.Fatal(err)
	}
	loadedRunning, err := db.GetMatch(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedRunning.Turn != game.Turn || loadedRunning.Ball != game.Ball || loadedRunning.DecisionSeed != game.DecisionSeed || !loadedRunning.DecisionPending {
		t.Fatalf("durable running state = %#v", loadedRunning)
	}
	legal := loadedRunning.LegalMoves()
	if len(legal) == 0 {
		t.Fatal("restored running match has no legal moves")
	}
	if _, err := loadedRunning.Apply(legal[0].Direction, time.Millisecond); err != nil {
		t.Fatalf("apply after restoration: %v", err)
	}
	if resumedEvent := loadedRunning.RecordResumed(); resumedEvent.Number != 2 {
		t.Fatalf("restored event number = %d, want 2", resumedEvent.Number)
	}
	game.DecisionSeed = 0
	game.DecisionPending = false
	game.Status = arena.Finished
	game.RedScore = 3
	game.BlueScore = 1
	game.Winner = arena.Red
	if err := db.SaveGame(game, nil, nil); err != nil {
		t.Fatal(err)
	}

	reverse := arena.NewGame("match_reverse", arena.AgentSummary{ID: blue.ID, Name: blue.Name}, arena.AgentSummary{ID: red.ID, Name: red.Name})
	if err := db.CreateMatch(reverse); !errors.Is(err, arena.ErrPairAlreadyPlayed) {
		t.Fatalf("reverse rematch error = %v", err)
	}

	loaded, err := db.GetMatch(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Moves) != 1 || loaded.Moves[0].Direction != 0 || loaded.RedScore != 3 || loaded.Winner != arena.Red {
		t.Fatalf("loaded replay = %#v", loaded)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].Type != "turn_skipped" || loaded.Events[0].Message == "" {
		t.Fatalf("loaded events = %#v", loaded.Events)
	}
	if loaded.RedAgent.OwnerName != "Red Owner" || loaded.RedAgent.Model != "test-model" || loaded.RedAgent.Effort != "high" || loaded.RedAgent.Author != "Tests" {
		t.Fatalf("loaded red provenance = %#v", loaded.RedAgent)
	}
	if loaded.BlueAgent.OwnerName != "Blue Owner" || loaded.BlueAgent.Model != "test-model" || loaded.BlueAgent.Effort != "high" || loaded.BlueAgent.Author != "Tests" {
		t.Fatalf("loaded blue provenance = %#v", loaded.BlueAgent)
	}
	leaders, err := db.Leaderboard()
	if err != nil {
		t.Fatal(err)
	}
	if len(leaders) != 2 || leaders[0].Name != red.Name || leaders[0].Wins != 1 || leaders[0].Points != 3 || leaders[0].GoalDiff != 2 {
		t.Fatalf("leaderboard = %#v", leaders)
	}
	if leaders[0].OwnerName != "Red Owner" || leaders[0].Model != "test-model" || leaders[0].Effort != "high" || leaders[0].Author != "Tests" {
		t.Fatalf("leaderboard provenance = %#v", leaders[0])
	}
	pairs, err := db.ListPlayedPairs()
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 || pairs[0].MatchID != game.ID {
		t.Fatalf("played pairs = %#v", pairs)
	}
}
