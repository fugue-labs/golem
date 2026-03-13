package automations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// WebhookServer listens for GitHub webhook deliveries and dispatches matching
// events to the configured automations.
type WebhookServer struct {
	port          int
	secret        string
	automations   []webhookAutomation
	handler       func(Event)
	server        *http.Server
}

type webhookAutomation struct {
	name   string
	events map[string]bool // "pull_request.opened" etc.
	repos  map[string]bool // "org/repo" etc. empty = match all
}

// NewWebhookServer creates a webhook server from the automations config.
func NewWebhookServer(cfg *Config, handler func(Event)) *WebhookServer {
	var automations []webhookAutomation
	for _, a := range cfg.Automations {
		if !a.IsEnabled() || a.Trigger.Type != "github_webhook" {
			continue
		}
		events := make(map[string]bool)
		for _, e := range a.Trigger.Events {
			events[strings.ToLower(e)] = true
		}
		repos := make(map[string]bool)
		for _, r := range a.Trigger.Repos {
			repos[strings.ToLower(r)] = true
		}
		automations = append(automations, webhookAutomation{
			name:   a.Name,
			events: events,
			repos:  repos,
		})
	}

	return &WebhookServer{
		port:        cfg.Server.Port,
		secret:      cfg.Server.WebhookSecret,
		automations: automations,
		handler:     handler,
	}
}

// Start begins listening for webhook deliveries. Blocks until ctx is cancelled.
func (s *WebhookServer) Start(ctx context.Context) error {
	if len(s.automations) == 0 {
		// No webhook automations configured; don't start the server.
		<-ctx.Done()
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/github", s.handleGitHub)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutCtx)
	}()

	log.Printf("webhook server listening on :%d", s.port)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("webhook server: %w", err)
	}
	return nil
}

func (s *WebhookServer) handleGitHub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Verify signature if secret is configured.
	if s.secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyGitHubSignature(body, sig, s.secret) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Parse event type and action.
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	action, _ := payload["action"].(string)
	fullEvent := eventType
	if action != "" {
		fullEvent = eventType + "." + action
	}

	// Extract repo full name for filtering.
	repoName := ""
	if repo, ok := payload["repository"].(map[string]any); ok {
		repoName, _ = repo["full_name"].(string)
	}

	// Check each automation for a match.
	matched := false
	for _, a := range s.automations {
		if !a.matchesEvent(fullEvent, eventType, repoName) {
			continue
		}
		matched = true
		s.handler(Event{
			Type:       "github_webhook",
			Name:       a.name,
			Timestamp:  time.Now(),
			Raw:        body,
			Properties: payload,
		})
	}

	if matched {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "accepted")
	} else {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "no matching automation")
	}
}

func (a webhookAutomation) matchesEvent(fullEvent, baseEvent, repo string) bool {
	fullLower := strings.ToLower(fullEvent)
	baseLower := strings.ToLower(baseEvent)

	eventMatch := a.events[fullLower] || a.events[baseLower]
	if !eventMatch {
		return false
	}

	// If repos filter is empty, match all repos.
	if len(a.repos) == 0 {
		return true
	}
	return a.repos[strings.ToLower(repo)]
}

// verifyGitHubSignature verifies the HMAC-SHA256 signature from GitHub.
func verifyGitHubSignature(payload []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}
