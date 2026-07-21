package webui

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jacek/agents-arena/internal/arena"
	"github.com/jacek/agents-arena/internal/store"
)

//go:embed templates/*.html static/* static/vendor/*
var files embed.FS

type Server struct {
	store     *store.Store
	manager   *arena.Manager
	logger    *slog.Logger
	templates *template.Template
	handler   http.Handler
}

type pageData struct {
	Title       string
	Agents      []arena.Agent
	Matches     []store.MatchRow
	Leaderboard []store.LeaderboardRow
	Game        *arena.Game
	Replay      bool
	Error       string
	Notice      string
	Script      string
	MaxBytes    int
	Timeout     int
}

func New(db *store.Store, manager *arena.Manager, logger *slog.Logger) (http.Handler, error) {
	templates, err := template.New("pages").Funcs(template.FuncMap{
		"lower":  strings.ToLower,
		"addOne": func(value int) int { return value + 1 },
		"ms": func(value time.Duration) string {
			if value == 0 {
				return "—"
			}
			return fmt.Sprintf("%.2f ms", float64(value.Microseconds())/1000)
		},
		"date": func(value time.Time) string { return value.Local().Format("02 Jan 2006 · 15:04") },
	}).ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{store: db, manager: manager, logger: logger, templates: templates}
	mux := http.NewServeMux()
	staticFS, err := fs.Sub(files, "static")
	if err != nil {
		return nil, err
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /spec", s.specPage)
	mux.HandleFunc("GET /register", s.registerPage)
	mux.HandleFunc("GET /history", s.historyPage)
	mux.HandleFunc("POST /agents", s.registerAgent)
	mux.HandleFunc("POST /matches", s.createMatch)
	mux.HandleFunc("GET /matches/{id}", s.matchPage)
	mux.HandleFunc("GET /api/v1/agents", s.listAgentsAPI)
	mux.HandleFunc("GET /api/v1/leaderboard", s.leaderboardAPI)
	mux.HandleFunc("GET /api/v1/matchups", s.matchupsAPI)
	mux.HandleFunc("POST /api/v1/agents", s.registerAgentAPI)
	mux.HandleFunc("POST /api/v1/agents/validate", s.validateAgentAPI)
	mux.HandleFunc("POST /api/v1/registrations/{color}", s.registerColorAPI)
	mux.HandleFunc("POST /api/v1/matches", s.createMatchAPI)
	mux.HandleFunc("GET /api/v1/matches/{id}", s.matchAPI)
	mux.HandleFunc("GET /api/v1/matches/{id}/available-moves", s.availableMovesAPI)
	mux.HandleFunc("GET /api/v1/spec", s.specAPI)
	s.handler = requestLogger(logger, securityHeaders(mux))
	return s.handler, nil
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.ListAgents()
	if err != nil {
		http.Error(w, "could not load agents", http.StatusInternalServerError)
		return
	}
	matches, err := s.store.ListMatches(12)
	if err != nil {
		http.Error(w, "could not load matches", http.StatusInternalServerError)
		return
	}
	leaderboard, err := s.store.Leaderboard()
	if err != nil {
		http.Error(w, "could not load leaderboard", http.StatusInternalServerError)
		return
	}
	s.render(w, "home.html", pageData{
		Title: "Agents Arena", Agents: agents, Matches: matches, Leaderboard: leaderboard,
		Script: exampleScript, MaxBytes: arena.MaxScriptBytes, Timeout: int(arena.MoveTimeout.Seconds()),
	})
}

func (s *Server) historyPage(w http.ResponseWriter, _ *http.Request) {
	matches, err := s.store.ListMatches(1000)
	if err != nil {
		http.Error(w, "could not load match history", http.StatusInternalServerError)
		return
	}
	leaderboard, err := s.store.Leaderboard()
	if err != nil {
		http.Error(w, "could not load leaderboard", http.StatusInternalServerError)
		return
	}
	s.render(w, "history.html", pageData{Title: "Match history & leaderboard", Matches: matches, Leaderboard: leaderboard})
}

func (s *Server) specPage(w http.ResponseWriter, _ *http.Request) {
	s.render(w, "spec.html", pageData{
		Title: "Agent Protocol v1", Script: exampleScript,
		MaxBytes: arena.MaxScriptBytes, Timeout: int(arena.MoveTimeout.Seconds()),
	})
}

func (s *Server) registerPage(w http.ResponseWriter, _ *http.Request) {
	s.render(w, "register.html", pageData{
		Title: "Register an agent", Script: exampleScript,
		MaxBytes: arena.MaxScriptBytes, Timeout: int(arena.MoveTimeout.Seconds()),
	})
}

func (s *Server) registerAgent(w http.ResponseWriter, r *http.Request) {
	agent, err := s.agentFromRequest(w, r)
	if err != nil {
		s.fragmentMessage(w, err.Error(), true)
		return
	}
	w.Header().Set("HX-Redirect", "/#agents")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="notice success">%s is registered.</div>`, template.HTMLEscapeString(agent.Name))
}

func (s *Server) registerAgentAPI(w http.ResponseWriter, r *http.Request) {
	agent, err := s.agentFromRequest(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, agent)
}

func (s *Server) registerColorAPI(w http.ResponseWriter, r *http.Request) {
	color := r.PathValue("color")
	if color != "red" && color != "blue" {
		writeError(w, http.StatusBadRequest, errors.New("color must be red or blue"))
		return
	}
	agent, err := s.agentFromRequest(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"preferred_color": color, "agent": agent})
}

func (s *Server) agentFromRequest(w http.ResponseWriter, r *http.Request) (arena.Agent, error) {
	r.Body = http.MaxBytesReader(w, r.Body, arena.MaxScriptBytes+128*1024)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(arena.MaxScriptBytes + 64*1024); err != nil {
			return arena.Agent{}, fmt.Errorf("invalid registration: %w", err)
		}
	} else if err := r.ParseForm(); err != nil {
		return arena.Agent{}, fmt.Errorf("invalid registration: %w", err)
	}
	name := strings.TrimSpace(r.FormValue("name"))
	author := strings.TrimSpace(r.FormValue("author"))
	description := strings.TrimSpace(r.FormValue("description"))
	ownerEmail := strings.ToLower(strings.TrimSpace(r.FormValue("owner_email")))
	model := strings.TrimSpace(r.FormValue("model"))
	effort := strings.TrimSpace(r.FormValue("effort"))
	if name == "" || len(name) > 48 {
		return arena.Agent{}, errors.New("name is required and must be at most 48 characters")
	}
	if author == "" {
		author = "Anonymous"
	}
	if len(author) > 64 {
		return arena.Agent{}, errors.New("author must be at most 64 characters")
	}
	if description == "" || len(description) > 500 {
		return arena.Agent{}, errors.New("description is required and must be at most 500 characters")
	}
	parsedEmail, err := mail.ParseAddress(ownerEmail)
	if ownerEmail == "" || len(ownerEmail) > 254 || err != nil || parsedEmail.Address != ownerEmail {
		return arena.Agent{}, errors.New("owner email is required and must be a valid email address")
	}
	if model == "" || len(model) > 80 {
		return arena.Agent{}, errors.New("development model is required and must be at most 80 characters")
	}
	if effort == "" || len(effort) > 32 {
		return arena.Agent{}, errors.New("effort level is required and must be at most 32 characters")
	}

	source := r.FormValue("code")
	if source == "" {
		source = r.FormValue("source")
	}
	if source == "" {
		file, _, err := r.FormFile("script")
		if err != nil {
			return arena.Agent{}, errors.New("provide Starlark code or attach a .star script")
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, arena.MaxScriptBytes+1))
		if err != nil {
			return arena.Agent{}, fmt.Errorf("read script: %w", err)
		}
		source = string(content)
	}
	if len(source) == 0 || len(source) > arena.MaxScriptBytes {
		return arena.Agent{}, fmt.Errorf("script is required and must be no larger than %d bytes", arena.MaxScriptBytes)
	}
	if err := arena.ValidateScript(source); err != nil {
		return arena.Agent{}, fmt.Errorf("script validation failed: %w", err)
	}
	agent, err := s.store.CreateAgent(name, author, description, ownerEmail, model, effort, source)
	if err != nil {
		if store.IsConflict(err) {
			return arena.Agent{}, errors.New("an agent with that name already exists")
		}
		return arena.Agent{}, err
	}
	return agent, nil
}

func (s *Server) validateAgentAPI(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, arena.MaxScriptBytes+4096)
	var input struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("expected JSON with a code field"))
		return
	}
	if len(input.Code) == 0 || len(input.Code) > arena.MaxScriptBytes {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"valid": false, "error": fmt.Sprintf("code must contain 1 to %d bytes", arena.MaxScriptBytes),
		})
		return
	}
	if err := arena.ValidateScript(input.Code); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":   true,
		"message": "Syntax, entrypoint, and sample decision are valid.",
		"checks":  []string{"source parsed", "choose_move(state) found", "sample decision returned a legal direction"},
	})
}

func (s *Server) createMatch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fragmentMessage(w, "Invalid match request.", true)
		return
	}
	game, err := s.manager.Create(r.FormValue("red_agent_id"), r.FormValue("blue_agent_id"))
	if err != nil {
		s.fragmentMessage(w, err.Error(), true)
		return
	}
	w.Header().Set("HX-Redirect", "/matches/"+game.ID)
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) createMatchAPI(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RedAgentID  string `json:"red_agent_id"`
		BlueAgentID string `json:"blue_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("expected JSON red_agent_id and blue_agent_id"))
		return
	}
	game, err := s.manager.Create(input.RedAgentID, input.BlueAgentID)
	if err != nil {
		if errors.Is(err, arena.ErrPairAlreadyPlayed) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Location", "/api/v1/matches/"+game.ID)
	writeJSON(w, http.StatusCreated, game)
}

func (s *Server) matchPage(w http.ResponseWriter, r *http.Request) {
	game, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		var err error
		game, err = s.store.GetMatch(r.PathValue("id"))
		if err != nil {
			http.Error(w, "match not found", http.StatusNotFound)
			return
		}
	}
	replay := r.URL.Query().Get("replay") == "1" || !ok
	s.render(w, "match.html", pageData{Title: game.RedAgent.Name + " vs " + game.BlueAgent.Name, Game: game, Replay: replay, Timeout: 5})
}

func (s *Server) matchAPI(w http.ResponseWriter, r *http.Request) {
	game, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		var err error
		game, err = s.store.GetMatch(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, errors.New("match not found"))
			return
		}
	}
	writeJSON(w, http.StatusOK, game)
}

func (s *Server) availableMovesAPI(w http.ResponseWriter, r *http.Request) {
	game, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		var err error
		game, err = s.store.GetMatch(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, errors.New("match not found"))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"match_id": game.ID, "turn": game.Turn, "ball": game.Ball,
		"deadline": game.Deadline, "legal_moves": game.Available,
	})
}

func (s *Server) listAgentsAPI(w http.ResponseWriter, _ *http.Request) {
	agents, err := s.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (s *Server) leaderboardAPI(w http.ResponseWriter, _ *http.Request) {
	leaderboard, err := s.store.Leaderboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"leaderboard": leaderboard})
}

func (s *Server) matchupsAPI(w http.ResponseWriter, _ *http.Request) {
	pairs, err := s.store.ListPlayedPairs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"played_pairs": pairs, "rule": "each unordered agent pairing may play once"})
}

func (s *Server) specAPI(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"protocol_version":     1,
		"language":             "Starlark",
		"entrypoint":           "choose_move(state) -> int",
		"directions":           map[string]int{"N": 0, "NE": 1, "E": 2, "SE": 3, "S": 4, "SW": 5, "W": 6, "NW": 7},
		"timeout_ms":           5000,
		"maximum_script_bytes": arena.MaxScriptBytes,
		"registration_fields":  []string{"name", "description", "owner_email", "model", "effort", "author", "script|code"},
		"pairing_rule":         "each unordered pair of agents may play exactly once",
		"competition_endpoints": []string{
			"GET /api/v1/leaderboard", "GET /api/v1/matchups", "GET /api/v1/matches/{match_id}",
		},
	})
}

func (s *Server) render(w http.ResponseWriter, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		s.logger.Error("render template", "template", name, "error", err)
	}
}

func (s *Server) fragmentMessage(w http.ResponseWriter, message string, isError bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	class := "success"
	if isError {
		class = "error"
	}
	fmt.Fprintf(w, `<div class="notice %s">%s</div>`, class, template.HTMLEscapeString(message))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}

const exampleScript = `def choose_move(state):
    moves = state["legal_moves"]

    # Always take a goal when one is available.
    for move in moves:
        if move["goal"]:
            return move["direction"]

    # Otherwise advance toward the opponent's goal.
    best = moves[0]
    north = state["attacks"] == "north"
    for move in moves:
        if north and move["to"]["y"] < best["to"]["y"]:
            best = move
        if not north and move["to"]["y"] > best["to"]["y"]:
            best = move
    return best["direction"]`
