package arena

import (
	"testing"
	"time"
)

func runningGame() *Game {
	g := NewGame("test", AgentSummary{ID: "r", Name: "Red"}, AgentSummary{ID: "b", Name: "Blue"})
	g.Status = Running
	g.Turn = Red
	return g
}

func TestInitialBoardHasEightMoves(t *testing.T) {
	g := runningGame()
	if got := len(g.LegalMoves()); got != 8 {
		t.Fatalf("initial legal moves = %d, want 8", got)
	}
}

func TestNewPointEndsTurn(t *testing.T) {
	g := runningGame()
	move, err := g.Apply(0, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if move.Bounce {
		t.Fatal("first edge unexpectedly bounced")
	}
	if g.Turn != Blue {
		t.Fatalf("turn = %s, want blue", g.Turn)
	}
}

func TestBoundaryPointBounces(t *testing.T) {
	g := runningGame()
	g.Ball = Point{2, 1}
	move, err := g.Apply(0, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !move.Bounce {
		t.Fatal("landing on the pitch perimeter should bounce")
	}
	if g.Turn != Red {
		t.Fatalf("turn = %s, want red", g.Turn)
	}
}

func TestNorthGoalScoresForRedAndResets(t *testing.T) {
	g := runningGame()
	g.Ball = Point{4, 0}
	move, err := g.Apply(0, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !move.Goal || g.RedScore != 1 {
		t.Fatalf("goal=%v score=%d, want true and 1", move.Goal, g.RedScore)
	}
	if g.Ball != (Point{4, 5}) || g.Turn != Blue || g.Round != 2 {
		t.Fatalf("reset state ball=%v turn=%s round=%d", g.Ball, g.Turn, g.Round)
	}
}

func TestOwnGoalCreditsOpponent(t *testing.T) {
	g := runningGame()
	g.Turn = Blue
	g.Ball = Point{4, 0}
	if _, err := g.Apply(0, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if g.RedScore != 1 {
		t.Fatalf("red score = %d, want 1", g.RedScore)
	}
}

func TestFirstToThreeFinishes(t *testing.T) {
	g := runningGame()
	g.RedScore = 2
	g.Ball = Point{4, 0}
	if _, err := g.Apply(0, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if g.Status != Finished || g.Winner != Red {
		t.Fatalf("status=%s winner=%s", g.Status, g.Winner)
	}
}

func TestUsedEdgeCannotBeReused(t *testing.T) {
	g := runningGame()
	if _, err := g.Apply(0, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	for _, move := range g.LegalMoves() {
		if move.Direction == 4 {
			t.Fatal("reverse direction reused the edge just drawn")
		}
	}
}

func TestDecisionFailureSkipsTurnWithoutScoring(t *testing.T) {
	g := runningGame()
	ball := g.Ball
	event := g.SkipTurn(Red, "Red turn skipped: deadline exceeded")
	if g.RedScore != 0 || g.BlueScore != 0 || g.Status != Running {
		t.Fatalf("skip changed result: status=%s score=%d:%d", g.Status, g.RedScore, g.BlueScore)
	}
	if g.Turn != Blue || g.Ball != ball {
		t.Fatalf("skip state: turn=%s ball=%v", g.Turn, g.Ball)
	}
	if event.Type != "turn_skipped" || event.Player != Red || event.Message == "" || len(g.Events) != 1 {
		t.Fatalf("skip event = %#v", event)
	}
}

func TestGoalRecordsReasonEvent(t *testing.T) {
	g := runningGame()
	g.Ball = Point{4, 0}
	if _, err := g.Apply(0, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(g.Events) != 1 || g.Events[0].Type != "goal" || g.Events[0].RedScore != 1 {
		t.Fatalf("goal events = %#v", g.Events)
	}
}
