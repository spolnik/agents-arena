package arena

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
)

var (
	ErrPairAlreadyPlayed  = errors.New("these agents have already played; each pairing is allowed only once")
	ErrMatchAlreadyActive = errors.New("match is already active")
	ErrMatchNotResumable  = errors.New("only interrupted waiting or running matches can be resumed")
)

type Agent struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Author      string    `json:"author"`
	Description string    `json:"description"`
	OwnerName   string    `json:"owner_name"`
	OwnerEmail  string    `json:"-"`
	Model       string    `json:"model"`
	Effort      string    `json:"effort"`
	Source      string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

type Repository interface {
	GetAgent(id string) (Agent, error)
	GetMatch(id string) (*Game, error)
	CreateMatch(game *Game) error
	SaveGame(game *Game, move *Move, event *MatchEvent) error
}

type Manager struct {
	mu     sync.RWMutex
	games  map[string]*Game
	repo   Repository
	logger *slog.Logger
}

func NewManager(repo Repository, logger *slog.Logger) *Manager {
	return &Manager{games: make(map[string]*Game), repo: repo, logger: logger}
}

func (m *Manager) Create(redID, blueID string) (*Game, error) {
	if redID == blueID {
		return nil, errors.New("choose two different agents")
	}
	red, err := m.repo.GetAgent(redID)
	if err != nil {
		return nil, fmt.Errorf("red agent: %w", err)
	}
	blue, err := m.repo.GetAgent(blueID)
	if err != nil {
		return nil, fmt.Errorf("blue agent: %w", err)
	}
	id := newID("match")
	game := NewGame(id,
		AgentSummary{ID: red.ID, Name: red.Name, Author: red.Author, OwnerName: red.OwnerName, Model: red.Model, Effort: red.Effort},
		AgentSummary{ID: blue.ID, Name: blue.Name, Author: blue.Author, OwnerName: blue.OwnerName, Model: blue.Model, Effort: blue.Effort},
	)
	if err := m.repo.CreateMatch(game); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.games[id] = game
	m.mu.Unlock()
	go m.play(id, red, blue, true)
	return cloneGame(game), nil
}

func (m *Manager) Resume(id string) (*Game, error) {
	game, err := m.repo.GetMatch(id)
	if err != nil {
		return nil, fmt.Errorf("load interrupted match: %w", err)
	}
	if game.Status != Running && game.Status != Waiting {
		return nil, ErrMatchNotResumable
	}
	if err := game.RestoreRuntime(); err != nil {
		return nil, fmt.Errorf("restore interrupted match: %w", err)
	}
	red, err := m.repo.GetAgent(game.RedAgent.ID)
	if err != nil {
		return nil, fmt.Errorf("red agent: %w", err)
	}
	blue, err := m.repo.GetAgent(game.BlueAgent.ID)
	if err != nil {
		return nil, fmt.Errorf("blue agent: %w", err)
	}

	m.mu.Lock()
	if _, active := m.games[id]; active {
		m.mu.Unlock()
		return nil, ErrMatchAlreadyActive
	}
	m.games[id] = game
	event := game.RecordResumed()
	if err := m.repo.SaveGame(game, nil, &event); err != nil {
		delete(m.games, id)
		m.mu.Unlock()
		return nil, fmt.Errorf("save resumed match: %w", err)
	}
	m.mu.Unlock()
	go m.play(id, red, blue, false)
	return cloneGame(game), nil
}

func (m *Manager) Get(id string) (*Game, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	game, ok := m.games[id]
	if !ok {
		return nil, false
	}
	return cloneGame(game), true
}

func (m *Manager) play(id string, red, blue Agent, initialize bool) {
	var game *Game
	if initialize {
		m.mu.Lock()
		game = m.games[id]
		game.Status = Running
		game.LastMessage = fmt.Sprintf("%s won the kickoff", game.Turn)
		_ = m.repo.SaveGame(game, nil, nil)
		m.mu.Unlock()
	}

	agents := map[Color]Agent{Red: red, Blue: blue}
	for {
		m.mu.Lock()
		game = m.games[id]
		if game.Status == Finished {
			_ = m.repo.SaveGame(game, nil, nil)
			m.mu.Unlock()
			return
		}
		player := game.Turn
		if !game.DecisionPending {
			game.DecisionSeed = rand.Int64()
			game.DecisionPending = true
		}
		state := DecisionState{
			You: player, RedScore: game.RedScore, BlueScore: game.BlueScore,
			Round: game.Round, MoveNumber: game.MoveNumber, Ball: game.Ball,
			LegalMoves: append([]LegalMove(nil), game.LegalMoves()...),
			Path:       append([]Edge(nil), game.Path...), DecisionSeed: game.DecisionSeed,
		}
		game.Deadline = time.Now().Add(MoveTimeout)
		game.LastMessage = fmt.Sprintf("%s is thinking", agents[player].Name)
		_ = m.repo.SaveGame(game, nil, nil)
		m.mu.Unlock()

		direction, duration, err := RunScript(agents[player].Source, state, MoveTimeout)

		m.mu.Lock()
		game = m.games[id]
		game.Deadline = time.Time{}
		game.DecisionSeed = 0
		game.DecisionPending = false
		if err != nil {
			event := game.SkipTurn(player, fmt.Sprintf("%s turn skipped: %v", agents[player].Name, err))
			_ = m.repo.SaveGame(game, nil, &event)
			m.logger.Warn("agent turn skipped", "match", id, "agent", agents[player].Name, "error", err)
		} else {
			eventsBefore := len(game.Events)
			move, applyErr := game.Apply(direction, duration)
			if applyErr != nil {
				event := game.SkipTurn(player, fmt.Sprintf("%s turn skipped: %v", agents[player].Name, applyErr))
				_ = m.repo.SaveGame(game, nil, &event)
			} else {
				var event *MatchEvent
				if len(game.Events) > eventsBefore {
					event = &game.Events[len(game.Events)-1]
				} else {
					game.LastMessage = fmt.Sprintf("%s moved in %s", agents[player].Name, displayDuration(duration))
				}
				_ = m.repo.SaveGame(game, &move, event)
			}
		}
		finished := game.Status == Finished
		m.mu.Unlock()
		if finished {
			continue
		}
		time.Sleep(650 * time.Millisecond)
	}
}

func displayDuration(duration time.Duration) string {
	if duration < time.Microsecond {
		return "<1 µs"
	}
	if duration < time.Millisecond {
		return fmt.Sprintf("%.1f µs", float64(duration.Nanoseconds())/1000)
	}
	return duration.Round(time.Microsecond).String()
}

func cloneGame(g *Game) *Game {
	copy := *g
	copy.Path = append([]Edge(nil), g.Path...)
	copy.Moves = append([]Move(nil), g.Moves...)
	copy.Events = append([]MatchEvent(nil), g.Events...)
	if g.Status == Running || g.Status == Waiting {
		copy.Available = append([]LegalMove(nil), g.LegalMoves()...)
	} else {
		copy.Available = nil
	}
	copy.used = nil
	copy.degree = nil
	return &copy
}
