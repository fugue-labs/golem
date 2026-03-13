package tools

import (
	"context"
	"sync"

	"github.com/fugue-labs/golem/internal/codegraph"
	"github.com/fugue-labs/gollem/core"
)

// graphCache holds the indexed graph so we only parse once per session.
var (
	graphMu    sync.Mutex
	graphCache *codegraph.Graph
	graphDir   string
)

func getGraph(workingDir string) (*codegraph.Graph, error) {
	graphMu.Lock()
	defer graphMu.Unlock()

	if graphCache != nil && graphDir == workingDir {
		return graphCache, nil
	}

	idx := codegraph.NewIndexer(workingDir)
	g, err := idx.Index()
	if err != nil {
		return nil, err
	}
	graphCache = g
	graphDir = workingDir
	return g, nil
}

// CodeSymbolsParams for the code_symbols tool.
type CodeSymbolsParams struct {
	Package  string `json:"package,omitempty" jsonschema:"description=Filter by package name"`
	Kind     string `json:"kind,omitempty" jsonschema:"description=Filter by kind: function, method, struct, interface, type, variable, constant"`
	Pattern  string `json:"pattern,omitempty" jsonschema:"description=Regex pattern to match symbol names"`
	File     string `json:"file,omitempty" jsonschema:"description=Filter by file path (relative to project root)"`
	Exported string `json:"exported,omitempty" jsonschema:"description=Filter by exported status: true, false, or empty for all"`
}

// CodeSymbolsTool lists symbols in the codebase knowledge graph.
func CodeSymbolsTool(workingDir string) core.Tool {
	return core.FuncTool[CodeSymbolsParams](
		"code_symbols",
		"Search the codebase knowledge graph for symbols (functions, types, interfaces, structs, methods, variables). "+
			"Returns definitions with their locations. Filter by package, kind, name pattern, or file. "+
			"Use this instead of grep when you need to understand code structure.",
		func(ctx context.Context, params CodeSymbolsParams) (string, error) {
			g, err := getGraph(workingDir)
			if err != nil {
				return "", err
			}

			filter := codegraph.SymbolFilter{
				Package: params.Package,
				Kind:    codegraph.SymbolKind(params.Kind),
				Pattern: params.Pattern,
				File:    params.File,
			}
			if params.Exported == "true" {
				t := true
				filter.Exported = &t
			} else if params.Exported == "false" {
				f := false
				filter.Exported = &f
			}

			return codegraph.QuerySymbols(g, filter)
		},
	)
}

// CodeCallersParams for the code_callers tool.
type CodeCallersParams struct {
	Function   string `json:"function" jsonschema:"description=Function or method name to find callers for. Can be unqualified (FuncName), qualified (pkg.FuncName), or method (Type.Method)."`
	Transitive bool   `json:"transitive,omitempty" jsonschema:"description=If true, find transitive callers (callers of callers). Default false."`
	MaxDepth   int    `json:"max_depth,omitempty" jsonschema:"description=Maximum depth for transitive search (default 10)"`
}

// CodeCallersTool finds who calls a function transitively.
func CodeCallersTool(workingDir string) core.Tool {
	return core.FuncTool[CodeCallersParams](
		"code_callers",
		"Find all callers of a function or method in the codebase. "+
			"Answers 'who calls this function?' with direct or transitive call chains. "+
			"Supports unqualified names (NewIndexer), qualified (codegraph.NewIndexer), "+
			"or method syntax (Graph.AddSymbol).",
		func(ctx context.Context, params CodeCallersParams) (string, error) {
			if params.Function == "" {
				return "function name is required", nil
			}
			g, err := getGraph(workingDir)
			if err != nil {
				return "", err
			}
			return codegraph.QueryCallers(g, params.Function, params.Transitive, params.MaxDepth)
		},
	)
}

// CodeDepsParams for the code_deps tool.
type CodeDepsParams struct {
	Package   string `json:"package" jsonschema:"description=Package name to query dependencies for. Use empty string to list all packages."`
	Direction string `json:"direction,omitempty" jsonschema:"description=Direction: 'imports' (what this package imports), 'imported_by' (what depends on this), or 'both'. Default: imports."`
}

// CodeDepsTool shows package dependency relationships.
func CodeDepsTool(workingDir string) core.Tool {
	return core.FuncTool[CodeDepsParams](
		"code_deps",
		"Show package import dependency graph. Answers 'what depends on this package?' "+
			"and 'what does this package import?'. Use empty package name to list all packages. "+
			"Direction: imports, imported_by, or both.",
		func(ctx context.Context, params CodeDepsParams) (string, error) {
			g, err := getGraph(workingDir)
			if err != nil {
				return "", err
			}
			return codegraph.QueryDeps(g, params.Package, params.Direction)
		},
	)
}

// CodeTypesParams for the code_types tool.
type CodeTypesParams struct {
	Type  string `json:"type" jsonschema:"description=Type or interface name to query"`
	Query string `json:"query,omitempty" jsonschema:"description=Query type: 'implementations' (what implements this interface), 'methods' (methods on type), 'hierarchy' (embeds, embedded-by, implements). Default: implementations."`
}

// CodeTypesTool shows type hierarchy and interface implementations.
func CodeTypesTool(workingDir string) core.Tool {
	return core.FuncTool[CodeTypesParams](
		"code_types",
		"Query the type hierarchy. Find interface implementations, methods on a type, "+
			"or the full type hierarchy (embeds, embedded-by, implements). "+
			"Answers 'what implements this interface?' and 'what methods does this type have?'.",
		func(ctx context.Context, params CodeTypesParams) (string, error) {
			if params.Type == "" {
				return "type name is required", nil
			}
			g, err := getGraph(workingDir)
			if err != nil {
				return "", err
			}
			return codegraph.QueryTypes(g, params.Type, params.Query)
		},
	)
}

// CodeRefsParams for the code_refs tool.
type CodeRefsParams struct {
	Symbol string `json:"symbol" jsonschema:"description=Symbol name to find references for"`
}

// CodeRefsTool finds all references to a symbol.
func CodeRefsTool(workingDir string) core.Tool {
	return core.FuncTool[CodeRefsParams](
		"code_refs",
		"Find all references to a symbol across the codebase. "+
			"Shows where a function, type, or variable is used. "+
			"More precise than grep because it understands code structure.",
		func(ctx context.Context, params CodeRefsParams) (string, error) {
			if params.Symbol == "" {
				return "symbol name is required", nil
			}
			g, err := getGraph(workingDir)
			if err != nil {
				return "", err
			}
			return codegraph.QueryRefs(g, params.Symbol)
		},
	)
}

// CodeGraphStatsTool returns graph statistics.
func CodeGraphStatsTool(workingDir string) core.Tool {
	return core.FuncTool[struct{}](
		"code_graph_stats",
		"Show statistics about the codebase knowledge graph: number of packages, "+
			"functions, types, call edges, etc. Useful for understanding codebase size and complexity.",
		func(ctx context.Context, _ struct{}) (string, error) {
			g, err := getGraph(workingDir)
			if err != nil {
				return "", err
			}
			return codegraph.QueryStats(g), nil
		},
	)
}

// CodeGraphTools returns all codebase knowledge graph tools.
func CodeGraphTools(workingDir string) []core.Tool {
	return []core.Tool{
		CodeSymbolsTool(workingDir),
		CodeCallersTool(workingDir),
		CodeDepsTool(workingDir),
		CodeTypesTool(workingDir),
		CodeRefsTool(workingDir),
		CodeGraphStatsTool(workingDir),
	}
}
