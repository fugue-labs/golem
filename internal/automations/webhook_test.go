package automations

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookServerHandleGitHub(t *testing.T) {
	var received []Event

	cfg := &Config{
		Automations: []Automation{
			{
				Name: "pr-review",
				Trigger: Trigger{
					Type:   "github_webhook",
					Events: []string{"pull_request.opened"},
					Repos:  []string{"fugue-labs/golem"},
				},
				Workflow: Workflow{Prompt: "review PR"},
			},
			{
				Name: "all-push",
				Trigger: Trigger{
					Type:   "github_webhook",
					Events: []string{"push"},
				},
				Workflow: Workflow{Prompt: "check push"},
			},
		},
		Server: ServerConfig{Port: 0},
	}

	handler := func(e Event) {
		received = append(received, e)
	}
	srv := NewWebhookServer(cfg, handler)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/github", srv.handleGitHub)

	t.Run("matching PR event", func(t *testing.T) {
		received = nil
		body := `{"action":"opened","repository":{"full_name":"fugue-labs/golem"},"pull_request":{"title":"test PR","number":42,"html_url":"https://github.com/fugue-labs/golem/pull/42"}}`
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", "pull_request")
		w := httptest.NewRecorder()

		srv.handleGitHub(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if len(received) != 1 {
			t.Fatalf("expected 1 event, got %d", len(received))
		}
		if received[0].Name != "pr-review" {
			t.Fatalf("expected pr-review, got %s", received[0].Name)
		}
	})

	t.Run("push event matches all-push", func(t *testing.T) {
		received = nil
		body := `{"ref":"refs/heads/main","repository":{"full_name":"other-org/other-repo"},"commits":[{"id":"abc"}]}`
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", "push")
		w := httptest.NewRecorder()

		srv.handleGitHub(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if len(received) != 1 {
			t.Fatalf("expected 1 event, got %d", len(received))
		}
		if received[0].Name != "all-push" {
			t.Fatalf("expected all-push, got %s", received[0].Name)
		}
	})

	t.Run("non-matching event", func(t *testing.T) {
		received = nil
		body := `{"action":"created","repository":{"full_name":"fugue-labs/golem"}}`
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", "issue_comment")
		w := httptest.NewRecorder()

		srv.handleGitHub(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if len(received) != 0 {
			t.Fatalf("expected 0 events, got %d", len(received))
		}
		respBody, _ := io.ReadAll(w.Body)
		if !strings.Contains(string(respBody), "no matching") {
			t.Fatalf("expected 'no matching' response, got %q", string(respBody))
		}
	})

	t.Run("wrong repo filtered", func(t *testing.T) {
		received = nil
		body := `{"action":"opened","repository":{"full_name":"other-org/other-repo"},"pull_request":{"title":"test"}}`
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", "pull_request")
		w := httptest.NewRecorder()

		srv.handleGitHub(w, req)

		if len(received) != 0 {
			t.Fatalf("expected 0 events for wrong repo, got %d", len(received))
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/webhook/github", nil)
		w := httptest.NewRecorder()
		srv.handleGitHub(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", w.Code)
		}
	})

	t.Run("missing event header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader("{}"))
		w := httptest.NewRecorder()
		srv.handleGitHub(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestWebhookSignatureVerification(t *testing.T) {
	secret := "test-secret-key"
	payload := []byte(`{"action":"opened"}`)

	// Compute valid signature.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	t.Run("valid signature", func(t *testing.T) {
		if !verifyGitHubSignature(payload, validSig, secret) {
			t.Fatal("expected valid signature to pass")
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		if verifyGitHubSignature(payload, "sha256=deadbeef", secret) {
			t.Fatal("expected invalid signature to fail")
		}
	})

	t.Run("missing prefix", func(t *testing.T) {
		if verifyGitHubSignature(payload, "invalid", secret) {
			t.Fatal("expected missing prefix to fail")
		}
	})

	t.Run("webhook server rejects bad signature", func(t *testing.T) {
		cfg := &Config{
			Automations: []Automation{{
				Name:     "test",
				Trigger:  Trigger{Type: "github_webhook", Events: []string{"push"}},
				Workflow: Workflow{Prompt: "test"},
			}},
			Server: ServerConfig{Port: 0, WebhookSecret: secret},
		}
		srv := NewWebhookServer(cfg, func(Event) {})

		req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(`{"ref":"main"}`))
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-Hub-Signature-256", "sha256=wrong")
		w := httptest.NewRecorder()
		srv.handleGitHub(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})
}
