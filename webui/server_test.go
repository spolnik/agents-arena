package webui

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacek/agents-arena/internal/arena"
	"github.com/jacek/agents-arena/internal/store"
)

func testServer(t *testing.T) http.Handler {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := New(db, arena.NewManager(db, logger), logger)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestValidateAgentAPI(t *testing.T) {
	handler := testServer(t)
	for _, test := range []struct {
		name   string
		code   string
		status int
		valid  bool
	}{
		{"valid", "def choose_move(state): return state[\"legal_moves\"][0][\"direction\"]", http.StatusOK, true},
		{"invalid", "def choose_move(state): return (", http.StatusUnprocessableEntity, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"code": test.code})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/agents/validate", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			var result struct {
				Valid bool `json:"valid"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Valid != test.valid {
				t.Fatalf("valid = %v", result.Valid)
			}
		})
	}
}

func TestInlineAgentRegistration(t *testing.T) {
	handler := testServer(t)
	form := url.Values{
		"name":        {"Inline Agent"},
		"author":      {"Test Team"},
		"description": {"A useful public description."},
		"owner_email": {"owner@example.com"},
		"model":       {"OpenAI GPT-5 Codex"},
		"effort":      {"high"},
		"code":        {"def choose_move(state): return state[\"legal_moves\"][0][\"direction\"]"},
	}
	request := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("HX-Redirect") != "/#agents" {
		t.Fatalf("status=%d redirect=%q body=%s", response.Code, response.Header().Get("HX-Redirect"), response.Body.String())
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if !strings.Contains(listResponse.Body.String(), `"description":"A useful public description."`) {
		t.Fatalf("description missing from API: %s", listResponse.Body.String())
	}
	for _, value := range []string{`"owner_email":"owner@example.com"`, `"model":"OpenAI GPT-5 Codex"`, `"effort":"high"`} {
		if !strings.Contains(listResponse.Body.String(), value) {
			t.Fatalf("%s missing from API: %s", value, listResponse.Body.String())
		}
	}
}

func TestRegistrationRejectsInvalidOwnerEmail(t *testing.T) {
	handler := testServer(t)
	form := url.Values{
		"name":        {"Invalid Owner"},
		"description": {"Tests required provenance validation."},
		"owner_email": {"not-an-email"},
		"model":       {"OpenAI GPT-5 Codex"},
		"effort":      {"high"},
		"code":        {"def choose_move(state): return state[\"legal_moves\"][0][\"direction\"]"},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "valid email address") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHistoryAndLeaderboardRoutes(t *testing.T) {
	handler := testServer(t)
	for _, path := range []string{"/history", "/api/v1/leaderboard", "/api/v1/matchups"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}
