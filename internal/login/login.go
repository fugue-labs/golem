// Package login implements the `golem login` subcommand for authenticating
// with LLM providers. ChatGPT uses OAuth PKCE; other providers prompt for
// an API key and save it to ~/.golem/credentials.json.
//
// After a successful login, the chosen provider is saved to ~/.golem/config.json
// so it takes precedence over env-var auto-detection on subsequent runs.
package login

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	openaiauth "github.com/fugue-labs/gollem/auth/openai"
)

// supportedProviders lists the providers available for login.
var supportedProviders = []struct {
	Name  string
	Label string
}{
	{"chatgpt", "ChatGPT (subscription — OAuth browser login)"},
	{"anthropic", "Anthropic (API key)"},
	{"openai", "OpenAI (API key)"},
	{"xai", "xAI / Grok (API key)"},
}

// SavedConfig stores the user's provider preference from `golem login`.
type SavedConfig struct {
	Provider string `json:"provider"` // chatgpt, anthropic, openai, xai
}

// Run executes the login flow for the given provider name.
// If provider is empty, an interactive picker is shown.
func Run(ctx context.Context, provider string) error {
	if provider == "" {
		var err error
		provider, err = pickProvider()
		if err != nil {
			return err
		}
	}
	provider = strings.ToLower(strings.TrimSpace(provider))

	switch provider {
	case "chatgpt":
		if err := loginChatGPT(ctx); err != nil {
			return err
		}
	case "anthropic", "openai", "xai":
		if err := loginAPIKey(provider); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown provider %q — supported: chatgpt, anthropic, openai, xai", provider)
	}

	// Save provider preference so config.Load() uses it on next run.
	return SaveConfig(&SavedConfig{Provider: provider})
}

func pickProvider() (string, error) {
	fmt.Println("Choose a provider to log in to:")
	fmt.Println()
	for i, p := range supportedProviders {
		fmt.Printf("  %d) %s\n", i+1, p.Label)
	}
	fmt.Println()
	fmt.Print("Enter number (1-4): ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("no input")
	}
	input := strings.TrimSpace(scanner.Text())

	// Accept either the number or the name.
	for i, p := range supportedProviders {
		if input == fmt.Sprintf("%d", i+1) || strings.EqualFold(input, p.Name) {
			return p.Name, nil
		}
	}
	return "", fmt.Errorf("invalid selection %q", input)
}

func loginChatGPT(ctx context.Context) error {
	fmt.Println("Logging in to ChatGPT via browser...")
	fmt.Println()
	creds, err := openaiauth.Login(ctx, openaiauth.LoginConfig{})
	if err != nil {
		return fmt.Errorf("ChatGPT login failed: %w", err)
	}
	if err := openaiauth.SaveCredentials(creds); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}
	fmt.Println("ChatGPT login successful! Credentials saved.")
	return nil
}

func loginAPIKey(provider string) error {
	var envHint string
	switch provider {
	case "anthropic":
		envHint = "ANTHROPIC_API_KEY"
	case "openai":
		envHint = "OPENAI_API_KEY"
	case "xai":
		envHint = "XAI_API_KEY"
	}

	fmt.Printf("Enter your %s API key (or set %s env var instead):\n", provider, envHint)
	fmt.Print("> ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("no input")
	}
	key := strings.TrimSpace(scanner.Text())
	if key == "" {
		return fmt.Errorf("empty API key")
	}

	if err := SaveAPIKey(provider, key); err != nil {
		return fmt.Errorf("saving API key: %w", err)
	}
	fmt.Printf("%s API key saved!\n", strings.ToUpper(provider[:1])+provider[1:])
	return nil
}

// Logout clears all saved credentials and config.
func Logout() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".golem")

	removed := false
	for _, name := range []string{"config.json", "credentials.json", "auth.json"} {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err == nil {
			removed = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", name, err)
		}
	}
	if removed {
		fmt.Println("Logged out. Saved credentials and config removed.")
	} else {
		fmt.Println("No saved credentials found.")
	}
	fmt.Println("Note: environment variables (ANTHROPIC_API_KEY, etc.) are unaffected.")
	return nil
}

// --- Config file (provider preference) ---

func golemDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".golem"), nil
}

func configPath() (string, error) {
	dir, err := golemDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadConfig reads the saved provider config from ~/.golem/config.json.
// Returns nil (not an error) if no config file exists.
func LoadConfig() *SavedConfig {
	path, err := configPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var sc SavedConfig
	if json.Unmarshal(data, &sc) != nil {
		return nil
	}
	if sc.Provider == "" {
		return nil
	}
	return &sc
}

// SaveConfig writes the provider preference to ~/.golem/config.json.
func SaveConfig(sc *SavedConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// --- Credentials file (API keys) ---

func credentialsPath() (string, error) {
	dir, err := golemDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// LoadAPIKeys reads saved API keys from ~/.golem/credentials.json.
// Returns an empty map if the file doesn't exist.
func LoadAPIKeys() (map[string]string, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var keys map[string]string
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	return keys, nil
}

// SaveAPIKey persists a single API key for the given provider.
func SaveAPIKey(provider, key string) error {
	keys, err := LoadAPIKeys()
	if err != nil {
		keys = map[string]string{}
	}
	keys[provider] = key

	path, err := credentialsPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
