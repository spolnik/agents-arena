package arena

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type capturedFault struct {
	event     MatchEvent
	redScore  int
	blueScore int
	ball      Point
}

type faultRepository struct {
	agents  map[string]Agent
	games   map[string]*Game
	faults  chan capturedFault
	resumed chan MatchEvent
}

func (r *faultRepository) GetAgent(id string) (Agent, error) { return r.agents[id], nil }
func (r *faultRepository) GetMatch(id string) (*Game, error) {
	game, ok := r.games[id]
	if !ok {
		return nil, errors.New("match not found")
	}
	return cloneGame(game), nil
}
func (r *faultRepository) CreateMatch(*Game) error { return nil }
func (r *faultRepository) SaveGame(game *Game, _ *Move, event *MatchEvent) error {
	if event != nil && event.Type == "match_resumed" && r.resumed != nil {
		r.resumed <- *event
		game.Status = Finished
	}
	if event != nil && event.Type == "turn_skipped" {
		r.faults <- capturedFault{event: *event, redScore: game.RedScore, blueScore: game.BlueScore, ball: game.Ball}
		game.Status = Finished
	}
	return nil
}

func TestManagerResumesPersistedMatchFromLastCommittedMove(t *testing.T) {
	red := Agent{ID: "red", Name: "Red", Source: `def choose_move(state): return state["legal_moves"][0]["direction"]`}
	blue := Agent{ID: "blue", Name: "Blue", Source: red.Source}
	interrupted := NewGame("match_interrupted", AgentSummary{ID: red.ID, Name: red.Name}, AgentSummary{ID: blue.ID, Name: blue.Name})
	interrupted.Status = Running
	move, err := interrupted.Apply(0, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	interrupted.DecisionSeed = 4242
	interrupted.DecisionPending = true

	repo := &faultRepository{
		agents:  map[string]Agent{red.ID: red, blue.ID: blue},
		games:   map[string]*Game{interrupted.ID: interrupted},
		faults:  make(chan capturedFault, 1),
		resumed: make(chan MatchEvent, 1),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := NewManager(repo, logger)
	resumed, err := manager.Resume(interrupted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.MoveNumber != move.Number || resumed.Ball != move.To || len(resumed.Path) != 1 {
		t.Fatalf("resumed state = %#v", resumed)
	}
	if resumed.DecisionSeed != 4242 || !resumed.DecisionPending {
		t.Fatalf("pending decision was not restored: seed=%d pending=%v", resumed.DecisionSeed, resumed.DecisionPending)
	}
	select {
	case event := <-repo.resumed:
		if event.Type != "match_resumed" || event.Number != 1 {
			t.Fatalf("resume event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resume event")
	}
	if _, err := manager.Resume(interrupted.ID); !errors.Is(err, ErrMatchAlreadyActive) {
		t.Fatalf("second resume error = %v", err)
	}
}

func TestManagerDecisionFailureSkipsTurnWithoutGoal(t *testing.T) {
	invalid := `def choose_move(state): return 99`
	repo := &faultRepository{
		agents: map[string]Agent{
			"red":  {ID: "red", Name: "Broken Red", Author: "Red Team", OwnerName: "Red Owner", OwnerEmail: "red@example.com", Model: "model-red", Effort: "high", Source: invalid},
			"blue": {ID: "blue", Name: "Broken Blue", Author: "Blue Team", OwnerName: "Blue Owner", OwnerEmail: "blue@example.com", Model: "model-blue", Effort: "medium", Source: invalid},
		},
		faults: make(chan capturedFault, 1),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := NewManager(repo, logger)
	game, err := manager.Create("red", "blue")
	if err != nil {
		t.Fatal(err)
	}
	if game.RedAgent.OwnerName != "Red Owner" || game.RedAgent.Model != "model-red" || game.RedAgent.Effort != "high" || game.RedAgent.Author != "Red Team" {
		t.Fatalf("red agent summary = %#v", game.RedAgent)
	}
	if game.BlueAgent.OwnerName != "Blue Owner" || game.BlueAgent.Model != "model-blue" || game.BlueAgent.Effort != "medium" || game.BlueAgent.Author != "Blue Team" {
		t.Fatalf("blue agent summary = %#v", game.BlueAgent)
	}
	select {
	case fault := <-repo.faults:
		if fault.redScore != 0 || fault.blueScore != 0 {
			t.Fatalf("decision fault changed score to %d:%d", fault.redScore, fault.blueScore)
		}
		if fault.ball != (Point{X: 4, Y: 5}) || fault.event.Message == "" {
			t.Fatalf("fault capture = %#v", fault)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for skipped-turn event")
	}
}
