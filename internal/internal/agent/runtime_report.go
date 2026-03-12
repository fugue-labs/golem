package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/golem/internal/config"
)

var (
	repoToolSurface = []string{"bash", "bash_status", "bash_kill", "view", "edit", "write", "multi_edit", "glob", "grep", "ls", "lsp"}
	workflowTools   = []string{"planning", "invariants", "verification"}
)

type ToolSurfaceReport struct {
	RepoTools        []string `json:"repo_tools"`
	WorkflowTools    []string `json:"workflow_tools"`
	Delegate         string   `json:"delegate"`
	ExecuteCode      string   `json:"execute_code"`
	ExecuteCodeNote  string   `json:"execute_code_note,omitempty"`
	OpenImage        string   `json:"open_image"`
	WebSearch        string   `json:"web_search"`
	FetchURL         string   `json:"fetch_url"`
	AskUser          string   `json:"ask_user"`
	AvailabilityNote string   `json:"availability_note,omitempty"`
}

type RuntimeReport struct {
	Provider             string                  `json:"provider"`
	ProviderSource       string                  `json:"provider_source,omitempty"`
	LoginProvider        string                  `json:"login_provider,omitempty"`
	Model                string                  `json:"model"`
	AuthMode             string                  `json:"auth_mode"`
	AuthSummary          string                  `json:"auth_summary"`
	BaseURL              string                  `json:"base_url,omitempty"`
	RouterModel          string                  `json:"router_model,omitempty"`
	EffectiveRouterModel string                  `json:"effective_router_model,omitempty"`
	Timeout              string                  `json:"timeout,omitempty"`
	TeamMode             string                  `json:"team_mode,omitempty"`
	EffectiveTeamMode    string                  `json:"effective_team_mode,omitempty"`
	TeamModeReason       string                  `json:"team_mode_reason,omitempty"`
	ReasoningEffort      string                  `json:"reasoning_effort,omitempty"`
	ThinkingBudget       int                     `json:"thinking_budget,omitempty"`
	AutoContextMaxTokens int                     `json:"auto_context_max_tokens,omitempty"`
	AutoContextKeepLastN int                     `json:"auto_context_keep_last_n,omitempty"`
	TopLevelPersonality  bool                    `json:"top_level_personality"`
	GitRepo              string                  `json:"git_repo"`
	PermissionMode       string                  `json:"permission_mode,omitempty"`
	GitBranch            string                  `json:"git_branch,omitempty"`
	InstructionFiles     []string                `json:"instruction_files,omitempty"`
	RuntimeError         string                  `json:"runtime_error,omitempty"`
	ToolSurfaces         ToolSurfaceReport       `json:"tool_surfaces"`
	Validation           config.ValidationResult `json:"validation,omitempty"`
}

func BuildRuntimeReport(cfg *config.Config, runtime RuntimeState, validation config.ValidationResult, runtimeErr error) RuntimeReport {
	report := RuntimeReport{
		Validation: validation,
		ToolSurfaces: ToolSurfaceReport{
			RepoTools:        append([]string(nil), repoToolSurface...),
			WorkflowTools:    append([]string(nil), workflowTools...),
			ExecuteCode:      fallbackStatus(runtime.CodeModeStatus),
			ExecuteCodeNote:  strings.TrimSpace(runtime.CodeModeError),
			OpenImage:        fallbackStatus(runtime.OpenImageStatus),
			WebSearch:        fallbackStatus(runtime.WebSearchStatus),
			FetchURL:         fallbackStatus(runtime.FetchURLStatus),
			AskUser:          fallbackStatus(runtime.AskUserStatus),
			AvailabilityNote: "Environment-dependent capabilities should only be trusted when surfaced by the active runtime/tool list.",
		},
	}
	if runtimeErr != nil {
		report.RuntimeError = compactError(runtimeErr, 200)
	}
	if cfg == nil {
		return report
	}

	authMode, authSummary := cfg.AuthStatus()
	report.Provider = string(cfg.Provider)
	report.ProviderSource = string(cfg.ProviderSource)
	report.LoginProvider = cfg.LoginProvider
	report.Model = cfg.Model
	report.AuthMode = authMode
	report.AuthSummary = authSummary
	report.BaseURL = cfg.BaseURL
	report.RouterModel = cfg.RouterModel
	report.EffectiveRouterModel = firstNonEmptyString(strings.TrimSpace(runtime.RouterModelName), strings.TrimSpace(cfg.RouterModel), strings.TrimSpace(cfg.Model))
	report.Timeout = cfg.Timeout.String()
	report.TeamMode = cfg.TeamMode
	report.EffectiveTeamMode = onOff(runtime.EffectiveTeamMode)
	report.TeamModeReason = strings.TrimSpace(runtime.TeamModeReason)
	report.ReasoningEffort = cfg.ReasoningEffort
	report.ThinkingBudget = cfg.ThinkingBudget
	report.AutoContextMaxTokens = cfg.AutoContextMaxTokens
	report.AutoContextKeepLastN = cfg.AutoContextKeepLastN
	report.TopLevelPersonality = cfg.TopLevelPersonality
	report.GitRepo = onOff(isGitRepo(cfg.WorkingDir))
	report.PermissionMode = cfg.PermissionMode
	if runtime.Git != nil {
		report.GitBranch = runtime.Git.BranchDisplay()
	}
	for _, f := range runtime.Instructions {
		report.InstructionFiles = append(report.InstructionFiles, shortFilePath(f.Path))
	}
	report.ToolSurfaces.Delegate = onOff(!cfg.DisableDelegate && runtime.EffectiveTeamMode)
	return report
}

func RenderStatusSummary(report RuntimeReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Provider:  %s", report.Provider)
	if report.LoginProvider != "" {
		fmt.Fprintf(&b, " (%s)", report.LoginProvider)
	}
	fmt.Fprintf(&b, "\nSource:    %s", report.ProviderSource)
	fmt.Fprintf(&b, "\nModel:     %s", report.Model)
	fmt.Fprintf(&b, "\nAuth:      %s", report.AuthSummary)
	if report.BaseURL != "" {
		fmt.Fprintf(&b, "\nBase URL:  %s", report.BaseURL)
	}
	if report.RouterModel != "" {
		fmt.Fprintf(&b, "\nRouter:    %s", report.RouterModel)
	}
	if report.Timeout != "" {
		fmt.Fprintf(&b, "\nTimeout:   %s", report.Timeout)
	}
	writeValidationSummary(&b, report.Validation)
	if report.RuntimeError != "" {
		fmt.Fprintf(&b, "\nRuntime:   %s", report.RuntimeError)
	}
	return b.String()
}

func RenderRuntimeSummary(report RuntimeReport) string {
	var b strings.Builder
	b.WriteString("**Runtime profile**\n\n")
	writeBulletLines(&b, runtimeProfileLines(report))
	b.WriteString("\n**Tool surfaces**\n\n")
	writeBulletLines(&b, toolSurfaceLines(report))
	writeValidationSections(&b, report.Validation, "**Validation errors**", "**Validation warnings**")
	if report.RuntimeError != "" {
		b.WriteString("\n**Runtime note**\n\n")
		fmt.Fprintf(&b, "- %s\n", report.RuntimeError)
	}
	return strings.TrimRight(b.String(), "\n")
}

func RenderRuntimePrompt(report RuntimeReport) string {
	var b strings.Builder
	b.WriteString("# Golem Runtime Profile\n\n")
	b.WriteString("## Effective runtime\n")
	writeBulletLines(&b, runtimeProfileLines(report))
	b.WriteString("\n## Tool surfaces\n")
	writeBulletLines(&b, toolSurfaceLines(report))
	writeValidationSections(&b, report.Validation, "## Validation errors", "## Validation warnings")
	if report.RuntimeError != "" {
		b.WriteString("\n## Runtime note\n")
		fmt.Fprintf(&b, "- %s\n", report.RuntimeError)
	}
	return strings.TrimRight(b.String(), "\n")
}

func runtimeProfileLines(report RuntimeReport) []string {
	lines := []string{fmt.Sprintf("Provider/model: `%s/%s`", report.Provider, report.Model)}
	if report.ProviderSource != "" {
		lines = append(lines, fmt.Sprintf("Provider source: `%s`", report.ProviderSource))
	}
	if report.RouterModel != "" {
		lines = append(lines, fmt.Sprintf("Router model: `%s`", report.RouterModel))
	}
	if report.EffectiveRouterModel != "" {
		lines = append(lines, fmt.Sprintf("Effective router model: `%s`", report.EffectiveRouterModel))
	}
	if report.Timeout != "" {
		lines = append(lines, fmt.Sprintf("Timeout: `%s`", report.Timeout))
	}
	if report.TeamMode != "" {
		lines = append(lines, fmt.Sprintf("Team mode: `%s` (effective: `%s`)", report.TeamMode, report.EffectiveTeamMode))
	}
	if report.TeamModeReason != "" {
		lines = append(lines, "Team mode reason: "+report.TeamModeReason)
	}
	if report.ReasoningEffort != "" {
		lines = append(lines, fmt.Sprintf("Reasoning effort: `%s`", report.ReasoningEffort))
	}
	if report.ThinkingBudget > 0 {
		lines = append(lines, fmt.Sprintf("Thinking budget: `%d`", report.ThinkingBudget))
	}
	if report.AutoContextMaxTokens > 0 {
		lines = append(lines, fmt.Sprintf("Auto-context: `%d` tokens, keep last `%d` turns", report.AutoContextMaxTokens, report.AutoContextKeepLastN))
	}
	lines = append(lines, fmt.Sprintf("Top-level personality: `%t`", report.TopLevelPersonality))
	lines = append(lines, fmt.Sprintf("Git repo: `%s`", report.GitRepo))
	if report.PermissionMode != "" {
		lines = append(lines, fmt.Sprintf("Permission mode: `%s`", report.PermissionMode))
	}
	if report.GitBranch != "" {
		lines = append(lines, fmt.Sprintf("Git branch: `%s`", report.GitBranch))
	}
	if len(report.InstructionFiles) > 0 {
		lines = append(lines, fmt.Sprintf("Project instructions: `%s`", strings.Join(report.InstructionFiles, "`, `")))
	}
	return lines
}

func toolSurfaceLines(report RuntimeReport) []string {
	lines := []string{
		fmt.Sprintf("Guaranteed repo tools: `%s`", strings.Join(report.ToolSurfaces.RepoTools, "`, `")),
		fmt.Sprintf("Guaranteed workflow tools: `%s`", strings.Join(report.ToolSurfaces.WorkflowTools, "`, `")),
		fmt.Sprintf("Delegate: `%s`", report.ToolSurfaces.Delegate),
		fmt.Sprintf("Execute code: `%s`", report.ToolSurfaces.ExecuteCode),
	}
	if report.ToolSurfaces.ExecuteCodeNote != "" {
		lines = append(lines, "Execute code note: "+report.ToolSurfaces.ExecuteCodeNote)
	}
	lines = append(lines,
		fmt.Sprintf("Open image: `%s`", report.ToolSurfaces.OpenImage),
		fmt.Sprintf("Web search: `%s`", report.ToolSurfaces.WebSearch),
		fmt.Sprintf("Fetch URL: `%s`", report.ToolSurfaces.FetchURL),
		fmt.Sprintf("Ask user: `%s`", report.ToolSurfaces.AskUser),
	)
	if report.ToolSurfaces.AvailabilityNote != "" {
		lines = append(lines, report.ToolSurfaces.AvailabilityNote)
	}
	return lines
}

func writeBulletLines(b *strings.Builder, lines []string) {
	for _, line := range lines {
		fmt.Fprintf(b, "- %s\n", line)
	}
}

func writeValidationSections(b *strings.Builder, validation config.ValidationResult, errorHeading, warningHeading string) {
	if len(validation.Errors) > 0 {
		fmt.Fprintf(b, "\n%s\n\n", errorHeading)
		for _, item := range validation.Errors {
			fmt.Fprintf(b, "- %s\n", item)
		}
	}
	if len(validation.Warnings) > 0 {
		fmt.Fprintf(b, "\n%s\n\n", warningHeading)
		for _, item := range validation.Warnings {
			fmt.Fprintf(b, "- %s\n", item)
		}
	}
}

func writeValidationSummary(b *strings.Builder, validation config.ValidationResult) {
	if len(validation.Errors) > 0 {
		b.WriteString("\nValidation errors:")
		for _, item := range validation.Errors {
			fmt.Fprintf(b, "\n  - %s", item)
		}
	}
	if len(validation.Warnings) > 0 {
		b.WriteString("\nValidation warnings:")
		for _, item := range validation.Warnings {
			fmt.Fprintf(b, "\n  - %s", item)
		}
	}
}

func fallbackStatus(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "pending"
	}
	return trimmed
}

func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
