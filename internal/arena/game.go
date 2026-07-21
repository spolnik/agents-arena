package arena

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sort"
	"time"
)

type Color string

const (
	Red  Color = "red"
	Blue Color = "blue"
)

func (c Color) Opponent() Color {
	if c == Red {
		return Blue
	}
	return Red
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Edge struct {
	From Point `json:"from"`
	To   Point `json:"to"`
}

type LegalMove struct {
	Direction int   `json:"direction"`
	To        Point `json:"to"`
	Bounce    bool  `json:"bounce"`
	Goal      bool  `json:"goal"`
}

type Move struct {
	Number     int           `json:"number"`
	Round      int           `json:"round"`
	Player     Color         `json:"player"`
	From       Point         `json:"from"`
	To         Point         `json:"to"`
	Direction  int           `json:"direction"`
	Bounce     bool          `json:"bounce"`
	Goal       bool          `json:"goal"`
	Duration   time.Duration `json:"duration_ns"`
	OccurredAt time.Time     `json:"occurred_at"`
}

type MatchEvent struct {
	Number     int       `json:"number"`
	Type       string    `json:"type"`
	Round      int       `json:"round"`
	Player     Color     `json:"player,omitempty"`
	Message    string    `json:"message"`
	RedScore   int       `json:"red_score"`
	BlueScore  int       `json:"blue_score"`
	OccurredAt time.Time `json:"occurred_at"`
}

type Status string

const (
	Waiting  Status = "waiting"
	Running  Status = "running"
	Finished Status = "finished"
)

var directions = [8]Point{
	{X: 0, Y: -1},  // north
	{X: 1, Y: -1},  // north-east
	{X: 1, Y: 0},   // east
	{X: 1, Y: 1},   // south-east
	{X: 0, Y: 1},   // south
	{X: -1, Y: 1},  // south-west
	{X: -1, Y: 0},  // west
	{X: -1, Y: -1}, // north-west
}

type Game struct {
	ID          string       `json:"id"`
	RedAgent    AgentSummary `json:"red_agent"`
	BlueAgent   AgentSummary `json:"blue_agent"`
	Status      Status       `json:"status"`
	Turn        Color        `json:"turn"`
	Ball        Point        `json:"ball"`
	RedScore    int          `json:"red_score"`
	BlueScore   int          `json:"blue_score"`
	Round       int          `json:"round"`
	MoveNumber  int          `json:"move_number"`
	Winner      Color        `json:"winner,omitempty"`
	Deadline    time.Time    `json:"deadline,omitempty"`
	LastMessage string       `json:"last_message,omitempty"`
	Path        []Edge       `json:"path"`
	Available   []LegalMove  `json:"legal_moves"`
	Moves       []Move       `json:"moves"`
	Events      []MatchEvent `json:"events"`
	createdAt   time.Time
	eventNumber int
	used        map[string]bool
	degree      map[Point]int
}

type AgentSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Author     string `json:"author"`
	OwnerEmail string `json:"owner_email"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
}

func NewGame(id string, red, blue AgentSummary) *Game {
	first := Red
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil && binary.LittleEndian.Uint64(buf[:])%2 == 1 {
		first = Blue
	}
	g := &Game{
		ID:        id,
		RedAgent:  red,
		BlueAgent: blue,
		Status:    Waiting,
		Turn:      first,
		RedScore:  0,
		BlueScore: 0,
		Round:     1,
		createdAt: time.Now().UTC(),
		Moves:     make([]Move, 0, 128),
		Events:    make([]MatchEvent, 0, 16),
	}
	g.resetPitch(first)
	return g
}

func (g *Game) resetPitch(starter Color) {
	g.Turn = starter
	g.Ball = Point{X: 4, Y: 5}
	g.Path = nil
	g.used = make(map[string]bool)
	g.degree = make(map[Point]int)
	g.addBoundary()
}

func (g *Game) addBoundary() {
	for y := 0; y < 10; y++ {
		g.markUsed(Point{0, y}, Point{0, y + 1})
		g.markUsed(Point{8, y}, Point{8, y + 1})
	}
	for x := 0; x < 3; x++ {
		g.markUsed(Point{x, 0}, Point{x + 1, 0})
		g.markUsed(Point{x, 10}, Point{x + 1, 10})
	}
	for x := 5; x < 8; x++ {
		g.markUsed(Point{x, 0}, Point{x + 1, 0})
		g.markUsed(Point{x, 10}, Point{x + 1, 10})
	}
	for _, y := range []int{-1, 10} {
		g.markUsed(Point{3, y}, Point{3, y + 1})
		g.markUsed(Point{5, y}, Point{5, y + 1})
	}
	for x := 3; x < 5; x++ {
		g.markUsed(Point{x, -1}, Point{x + 1, -1})
		g.markUsed(Point{x, 11}, Point{x + 1, 11})
	}
}

func (g *Game) markUsed(a, b Point) {
	g.used[edgeKey(a, b)] = true
	g.degree[a]++
	g.degree[b]++
}

func edgeKey(a, b Point) string {
	if a.X > b.X || (a.X == b.X && a.Y > b.Y) {
		a, b = b, a
	}
	return fmt.Sprintf("%d,%d:%d,%d", a.X, a.Y, b.X, b.Y)
}

func (g *Game) LegalMoves() []LegalMove {
	moves := make([]LegalMove, 0, 8)
	for direction, delta := range directions {
		to := Point{X: g.Ball.X + delta.X, Y: g.Ball.Y + delta.Y}
		if !segmentInsideArena(g.Ball, to) || g.used[edgeKey(g.Ball, to)] {
			continue
		}
		moves = append(moves, LegalMove{
			Direction: direction,
			To:        to,
			Bounce:    g.degree[to] > 0,
			Goal:      to.Y == -1 || to.Y == 11,
		})
	}
	return moves
}

func segmentInsideArena(a, b Point) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	if dx < -1 || dx > 1 || dy < -1 || dy > 1 || (dx == 0 && dy == 0) {
		return false
	}
	if !validNode(a) || !validNode(b) {
		return false
	}
	// Coordinates are doubled so midpoint tests remain exact integers.
	mx, my := a.X+b.X, a.Y+b.Y
	inPitch := mx >= 0 && mx <= 16 && my >= 0 && my <= 20
	inNorthGoal := mx >= 6 && mx <= 10 && my >= -2 && my <= 0
	inSouthGoal := mx >= 6 && mx <= 10 && my >= 20 && my <= 22
	return inPitch || inNorthGoal || inSouthGoal
}

func validNode(p Point) bool {
	if p.X >= 0 && p.X <= 8 && p.Y >= 0 && p.Y <= 10 {
		return true
	}
	return p.X >= 3 && p.X <= 5 && (p.Y == -1 || p.Y == 11)
}

// Apply advances exactly one edge. Bounce chains deliberately request another
// decision, so every edge receives its own five-second budget.
func (g *Game) Apply(direction int, duration time.Duration) (Move, error) {
	if g.Status != Running {
		return Move{}, fmt.Errorf("match is not running")
	}
	var selected *LegalMove
	for _, candidate := range g.LegalMoves() {
		if candidate.Direction == direction {
			copy := candidate
			selected = &copy
			break
		}
	}
	if selected == nil {
		return Move{}, fmt.Errorf("direction %d is not legal", direction)
	}

	player, from := g.Turn, g.Ball
	preexistingDegree := g.degree[selected.To]
	g.markUsed(from, selected.To)
	g.Path = append(g.Path, Edge{From: from, To: selected.To})
	g.Ball = selected.To
	g.MoveNumber++
	move := Move{
		Number: g.MoveNumber, Round: g.Round, Player: player,
		From: from, To: selected.To, Direction: direction,
		Bounce: preexistingDegree > 0, Goal: selected.Goal,
		Duration: duration, OccurredAt: time.Now().UTC(),
	}
	g.Moves = append(g.Moves, move)

	if selected.Goal {
		scorer := Blue
		if selected.To.Y == -1 {
			scorer = Red
		}
		g.awardGoal(scorer, fmt.Sprintf("%s scores", scorer))
		return move, nil
	}

	if preexistingDegree == 0 {
		g.Turn = player.Opponent()
	}
	if len(g.LegalMoves()) == 0 {
		g.awardGoal(g.Turn.Opponent(), fmt.Sprintf("%s is trapped", g.Turn))
	}
	return move, nil
}

func (g *Game) Penalize(player Color, reason string) {
	g.awardGoal(player.Opponent(), reason)
}

func (g *Game) SkipTurn(player Color, reason string) MatchEvent {
	g.Turn = player.Opponent()
	g.LastMessage = reason
	return g.recordEvent("turn_skipped", player, reason)
}

func (g *Game) awardGoal(scorer Color, message string) {
	if scorer == Red {
		g.RedScore++
	} else {
		g.BlueScore++
	}
	g.LastMessage = message
	g.recordEvent("goal", scorer, message)
	if g.RedScore >= 3 || g.BlueScore >= 3 {
		g.Status = Finished
		g.Winner = scorer
		g.Deadline = time.Time{}
		return
	}
	g.Round++
	g.resetPitch(scorer.Opponent())
}

func (g *Game) recordEvent(eventType string, player Color, message string) MatchEvent {
	g.eventNumber++
	event := MatchEvent{
		Number: g.eventNumber, Type: eventType, Round: g.Round, Player: player,
		Message: message, RedScore: g.RedScore, BlueScore: g.BlueScore, OccurredAt: time.Now().UTC(),
	}
	g.Events = append(g.Events, event)
	return event
}

func (g *Game) DirectionNames() []string {
	return []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
}

func (g *Game) UsedEdgeKeys() []string {
	keys := make([]string, 0, len(g.used))
	for key := range g.used {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
