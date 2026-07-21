package arena

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"
)

const (
	MoveTimeout     = 5 * time.Second
	MaxScriptBytes  = 64 * 1024
	MaxProgramSteps = 1_000_000
)

type DecisionState struct {
	You          Color
	RedScore     int
	BlueScore    int
	Round        int
	MoveNumber   int
	Ball         Point
	LegalMoves   []LegalMove
	Path         []Edge
	DecisionSeed int64
}

func RunScript(source string, state DecisionState, timeout time.Duration) (int, time.Duration, error) {
	if len(source) > MaxScriptBytes {
		return 0, 0, fmt.Errorf("script exceeds %d bytes", MaxScriptBytes)
	}
	thread := &starlark.Thread{Name: "agent-decision"}
	thread.SetMaxExecutionSteps(MaxProgramSteps)
	timer := time.AfterFunc(timeout, func() { thread.Cancel("five-second move deadline exceeded") })
	defer timer.Stop()

	started := time.Now()
	globals, err := starlark.ExecFile(thread, "agent.star", source, nil)
	if err != nil {
		return 0, time.Since(started), fmt.Errorf("load script: %w", err)
	}
	choose, ok := globals["choose_move"]
	if !ok {
		return 0, time.Since(started), fmt.Errorf("script must define choose_move(state)")
	}
	result, err := starlark.Call(thread, choose, starlark.Tuple{stateValue(state)}, nil)
	duration := time.Since(started)
	if err != nil {
		return 0, duration, fmt.Errorf("choose_move: %w", err)
	}
	choice, ok := result.(starlark.Int)
	if !ok {
		return 0, duration, fmt.Errorf("choose_move must return an integer direction 0..7, got %s", result.Type())
	}
	value, ok := choice.Int64()
	if !ok || value < 0 || value > 7 {
		return 0, duration, fmt.Errorf("direction must be an integer from 0 through 7")
	}
	return int(value), duration, nil
}

func ValidateScript(source string) error {
	moves := make([]LegalMove, 0, 8)
	for direction, delta := range directions {
		moves = append(moves, LegalMove{Direction: direction, To: Point{4 + delta.X, 5 + delta.Y}})
	}
	state := DecisionState{
		You: Red, Ball: Point{4, 5},
		LegalMoves: moves,
	}
	direction, _, err := RunScript(source, state, 500*time.Millisecond)
	if err != nil {
		return err
	}
	if direction < 0 || direction > 7 {
		return fmt.Errorf("script validation returned unavailable direction %d", direction)
	}
	return nil
}

func stateValue(s DecisionState) *starlark.Dict {
	d := starlark.NewDict(12)
	set := func(key string, value starlark.Value) { _ = d.SetKey(starlark.String(key), value) }
	set("protocol_version", starlark.MakeInt(1))
	set("you", starlark.String(s.You))
	set("attacks", starlark.String(map[Color]string{Red: "north", Blue: "south"}[s.You]))
	set("round", starlark.MakeInt(s.Round))
	set("move_number", starlark.MakeInt(s.MoveNumber))
	set("decision_seed", starlark.MakeInt64(s.DecisionSeed))
	set("ball", pointValue(s.Ball))
	score := starlark.NewDict(2)
	_ = score.SetKey(starlark.String("red"), starlark.MakeInt(s.RedScore))
	_ = score.SetKey(starlark.String("blue"), starlark.MakeInt(s.BlueScore))
	set("score", score)

	moves := make([]starlark.Value, 0, len(s.LegalMoves))
	for _, move := range s.LegalMoves {
		item := starlark.NewDict(4)
		_ = item.SetKey(starlark.String("direction"), starlark.MakeInt(move.Direction))
		_ = item.SetKey(starlark.String("to"), pointValue(move.To))
		_ = item.SetKey(starlark.String("bounce"), starlark.Bool(move.Bounce))
		_ = item.SetKey(starlark.String("goal"), starlark.Bool(move.Goal))
		moves = append(moves, item)
	}
	set("legal_moves", starlark.NewList(moves))

	path := make([]starlark.Value, 0, len(s.Path))
	for _, edge := range s.Path {
		path = append(path, starlark.Tuple{pointValue(edge.From), pointValue(edge.To)})
	}
	set("path", starlark.NewList(path))
	board := starlark.NewDict(4)
	_ = board.SetKey(starlark.String("width"), starlark.MakeInt(8))
	_ = board.SetKey(starlark.String("height"), starlark.MakeInt(10))
	_ = board.SetKey(starlark.String("goal_left"), starlark.MakeInt(3))
	_ = board.SetKey(starlark.String("goal_right"), starlark.MakeInt(5))
	set("board", board)
	d.Freeze()
	return d
}

func pointValue(p Point) *starlark.Dict {
	d := starlark.NewDict(2)
	_ = d.SetKey(starlark.String("x"), starlark.MakeInt(p.X))
	_ = d.SetKey(starlark.String("y"), starlark.MakeInt(p.Y))
	return d
}
