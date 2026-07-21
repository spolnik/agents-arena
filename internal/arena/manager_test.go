package arena

import (
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
	agents map[string]Agent
	faults chan capturedFault
}

func (r *faultRepository) GetAgent(id string) (Agent, error) { return r.agents[id], nil }
func (r *faultRepository) CreateMatch(*Game) error           { return nil }
func (r *faultRepository) SaveGame(game *Game, _ *Move, event *MatchEvent) error {
	if event != nil && event.Type == "turn_skipped" {
		r.faults <- capturedFault{event: *event, redScore: game.RedScore, blueScore: game.BlueScore, ball: game.Ball}
		game.Status = Finished
	}
	return nil
}

func TestManagerDecisionFailureSkipsTurnWithoutGoal(t *testing.T) {
	invalid := `def choose_move(state): return 99`
	repo := &faultRepository{
		agents: map[string]Agent{
			"red":  {ID: "red", Name: "Broken Red", Author: "Red Team", OwnerEmail: "red@example.com", Model: "Model Red", Effort: "high", Source: invalid},
			"blue": {ID: "blue", Name: "Broken Blue", Author: "Blue Team", OwnerEmail: "blue@example.com", Model: "Model Blue", Effort: "medium", Source: invalid},
		},
		faults: make(chan capturedFault, 1),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := NewManager(repo, logger)
	game, err := manager.Create("red", "blue")
	if err != nil {
		t.Fatal(err)
	}
	if game.RedAgent.OwnerEmail != "red@example.com" || game.RedAgent.Model != "Model Red" || game.RedAgent.Effort != "high" || game.RedAgent.Author != "Red Team" {
		t.Fatalf("red agent summary = %#v", game.RedAgent)
	}
	if game.BlueAgent.OwnerEmail != "blue@example.com" || game.BlueAgent.Model != "Model Blue" || game.BlueAgent.Effort != "medium" || game.BlueAgent.Author != "Blue Team" {
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
