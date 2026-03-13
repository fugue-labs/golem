package agent

import (
	"os"
	"path/filepath"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/memory"
)

// MemoryDir returns the persistent memory directory for a project.
// The directory is at ~/.golem/memory/<project-hash>/.
func MemoryDir(workDir string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".golem", "memory", projectHash(workDir))
}

// SetupMemory creates a persistent memory store and returns the memory tool
// and knowledge base for agent integration. The SQLite database is stored
// per project so memories are project-scoped.
func SetupMemory(workDir string) (store memory.Store, tool core.Tool, kb core.KnowledgeBase, err error) {
	dir := MemoryDir(workDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, core.Tool{}, nil, err
	}

	dbPath := filepath.Join(dir, "memory.db")
	store, err = memory.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, core.Tool{}, nil, err
	}

	namespace := []string{"golem", projectHash(workDir)}
	tool = memory.MemoryTool(store, namespace...)
	kb = memory.StoreKnowledgeBase(store, namespace...)
	return store, tool, kb, nil
}
