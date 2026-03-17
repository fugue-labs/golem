package docsqa

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type docRequirement struct {
	file    string
	needles []string
}

type contract struct {
	name          string
	sourceFile    string
	sourceNeedles []string
	docs          []docRequirement
}

func TestDocumentationContracts(t *testing.T) {
	root := repoRoot(t)
	cache := map[string]string{}

	read := func(rel string) string {
		t.Helper()
		if text, ok := cache[rel]; ok {
			return text
		}
		path := filepath.Join(root, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(data)
		cache[rel] = text
		return text
	}

	// Keep this intentionally narrow. It is a drift alarm for a few high-risk,
	// operator-facing contracts rather than an exhaustive documentation linter.
	contracts := []contract{
		{
			name:          "quickstart shell onboarding stays documented",
			sourceFile:    "main.go",
			sourceNeedles: []string{`case "login":`},
			docs: []docRequirement{
				{file: "README.md", needles: []string{"golem login", "golem"}},
			},
		},
		{
			name:          "in-app onboarding commands stay documented",
			sourceFile:    "internal/ui/commands.go",
			sourceNeedles: []string{"/help", "/runtime", "/doctor", "/search <query>"},
			docs: []docRequirement{
				{file: "README.md", needles: []string{"/help", "/runtime", "/doctor", "/search <query>"}},
				{file: "docs/command-reference.md", needles: []string{"| `/help` | `/help` |", "| `/runtime` | `/runtime` |", "| `/doctor` | `/doctor` |", "| `/search <query>` | `/search <query>` |"}},
			},
		},
		{
			name:          "shell command reference tracks implemented cli entrypoints",
			sourceFile:    "main.go",
			sourceNeedles: []string{`case "logout":`, `case "dashboard":`, `case "status", "runtime":`, `case "automations":`},
			docs: []docRequirement{
				{file: "README.md", needles: []string{"golem status --json", "golem runtime --json", "golem dashboard", "golem automations list"}},
				{file: "docs/command-reference.md", needles: []string{"### `golem logout`", "### `golem status`", "### `golem runtime`", "### `golem dashboard`", "### `golem automations`"}},
			},
		},
		{
			name:          "mission and model discoverability remain documented",
			sourceFile:    "internal/ui/commands.go",
			sourceNeedles: []string{"/model [name]", "/mission [new|status|tasks|plan|approve|start|pause|cancel|list]", "/config", "/cost"},
			docs: []docRequirement{
				{file: "README.md", needles: []string{"/cost", "/mission new <goal>"}},
				{file: "docs/command-reference.md", needles: []string{"| `/model [name]` | `/model` or `/model <name>` |", "| `/config` | `/config` |", "| `/cost` | `/cost` |", "/mission [new|status|tasks|plan|approve|start|pause|cancel|list]"}},
			},
		},
		{
			name:          "critical runtime env vars stay documented",
			sourceFile:    "internal/config/config.go",
			sourceNeedles: []string{"GOLEM_PROVIDER", "GOLEM_MODEL", "GOLEM_ROUTER_MODEL", "GOLEM_BASE_URL", "GOLEM_API_KEY", "GOLEM_TIMEOUT", "GOLEM_PERMISSION_MODE", "GOLEM_SESSION_BUDGET", "GOLEM_PROJECT_BUDGET", "GOLEM_BUDGET_WARN_PCT", "GOLEM_FALLBACK_MODEL", "GOLEM_REASONING_EFFORT", "GOLEM_THINKING_BUDGET", "VERTEX_PROJECT"},
			docs: []docRequirement{
				{file: "README.md", needles: []string{"GOLEM_PROVIDER", "GOLEM_MODEL", "GOLEM_ROUTER_MODEL", "GOLEM_BASE_URL", "GOLEM_API_KEY", "GOLEM_TIMEOUT", "GOLEM_PERMISSION_MODE", "GOLEM_SESSION_BUDGET", "GOLEM_PROJECT_BUDGET", "GOLEM_BUDGET_WARN_PCT", "GOLEM_FALLBACK_MODEL", "GOLEM_REASONING_EFFORT", "GOLEM_THINKING_BUDGET", "VERTEX_PROJECT"}},
				{file: "docs/configuration.md", needles: []string{"GOLEM_PROVIDER", "GOLEM_MODEL", "GOLEM_ROUTER_MODEL", "GOLEM_BASE_URL", "GOLEM_API_KEY", "GOLEM_TIMEOUT", "GOLEM_PERMISSION_MODE", "GOLEM_SESSION_BUDGET", "GOLEM_PROJECT_BUDGET", "GOLEM_BUDGET_WARN_PCT", "GOLEM_FALLBACK_MODEL", "GOLEM_REASONING_EFFORT", "GOLEM_THINKING_BUDGET", "VERTEX_PROJECT"}},
			},
		},
	}

	for _, tc := range contracts {
		t.Run(tc.name, func(t *testing.T) {
			source := read(tc.sourceFile)
			for _, needle := range tc.sourceNeedles {
				requireContains(t, tc.sourceFile, source, needle)
			}
			for _, doc := range tc.docs {
				text := read(doc.file)
				for _, needle := range doc.needles {
					requireContains(t, doc.file, text, needle)
				}
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller for repo root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func requireContains(t *testing.T, file, text, needle string) {
	t.Helper()
	if strings.Contains(text, needle) {
		return
	}
	t.Fatalf("%s is missing %q", file, needle)
}
