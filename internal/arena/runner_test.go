package arena

import (
	"strings"
	"testing"
	"time"
)

func TestRunScript(t *testing.T) {
	source := `def choose_move(state):
    return state["legal_moves"][0]["direction"]
`
	direction, _, err := RunScript(source, DecisionState{You: Red, LegalMoves: []LegalMove{{Direction: 6}}}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if direction != 6 {
		t.Fatalf("direction = %d, want 6", direction)
	}
}

func TestRunScriptRejectsWrongResult(t *testing.T) {
	_, _, err := RunScript(`def choose_move(state): return "north"`, DecisionState{}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "integer") {
		t.Fatalf("error = %v, want integer validation", err)
	}
}

func TestValidateScriptRejectsMissingEntrypoint(t *testing.T) {
	err := ValidateScript(`def other(state): return 0`)
	if err == nil || !strings.Contains(err.Error(), "choose_move") {
		t.Fatalf("error = %v, want missing choose_move", err)
	}
}
