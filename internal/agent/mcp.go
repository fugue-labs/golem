package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/mcp"
)

// MCPServerSpec defines an MCP server in the config file.
type MCPServerSpec struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPConfig holds the list of MCP servers to connect to.
type MCPConfig struct {
	Servers []MCPServerSpec `json:"servers"`
}

// LoadMCPConfig loads MCP server configuration from disk and environment.
// Checks ~/.golem/mcp.json and GOLEM_MCP_SERVERS env var.
func LoadMCPConfig() (*MCPConfig, error) {
	config := &MCPConfig{}

	// Load from config file.
	home, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(home, ".golem", "mcp.json")
		if data, err := os.ReadFile(configPath); err == nil {
			if err := json.Unmarshal(data, config); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", configPath, err)
			}
		}
	}

	// Load from env var (format: "name:command:arg1,arg2;name2:command2:arg1")
	if envServers := os.Getenv("GOLEM_MCP_SERVERS"); envServers != "" {
		for _, spec := range strings.Split(envServers, ";") {
			parts := strings.SplitN(strings.TrimSpace(spec), ":", 3)
			if len(parts) < 2 {
				continue
			}
			s := MCPServerSpec{
				Name:    parts[0],
				Command: parts[1],
			}
			if len(parts) == 3 && parts[2] != "" {
				s.Args = strings.Split(parts[2], ",")
			}
			config.Servers = append(config.Servers, s)
		}
	}

	return config, nil
}

// ConnectMCPServers connects to all configured MCP servers and returns
// the manager and any tools discovered. Errors on individual servers are
// logged but do not prevent other servers from connecting.
func ConnectMCPServers(ctx context.Context, config *MCPConfig) (*mcp.Manager, []core.Tool, []string, error) {
	if config == nil || len(config.Servers) == 0 {
		return nil, nil, nil, nil
	}

	manager := mcp.NewManager()
	var connected []string
	var errs []string

	for _, spec := range config.Servers {
		args := spec.Args
		client, err := mcp.NewStdioClient(ctx, spec.Command, args...)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", spec.Name, err))
			continue
		}
		if err := manager.AddServer(spec.Name, client); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", spec.Name, err))
			continue
		}
		connected = append(connected, spec.Name)
	}

	if len(connected) == 0 {
		return nil, nil, nil, fmt.Errorf("no MCP servers connected: %s", strings.Join(errs, "; "))
	}

	tools, err := manager.Tools(ctx)
	if err != nil {
		return manager, nil, connected, err
	}

	return manager, tools, connected, nil
}
