// Package codegraph provides semantic indexing of Go codebases.
// It builds a knowledge graph of symbols, call relationships, type hierarchies,
// and cross-file references using the standard go/parser and go/ast packages.
package codegraph

import (
	"fmt"
	"strings"
)

// SymbolKind classifies a symbol in the codebase.
type SymbolKind string

const (
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindStruct    SymbolKind = "struct"
	KindInterface SymbolKind = "interface"
	KindTypeAlias SymbolKind = "type"
	KindVariable  SymbolKind = "variable"
	KindConstant  SymbolKind = "constant"
	KindField     SymbolKind = "field"
)

// EdgeKind classifies a relationship between symbols.
type EdgeKind string

const (
	EdgeCalls      EdgeKind = "calls"
	EdgeImplements EdgeKind = "implements"
	EdgeEmbeds     EdgeKind = "embeds"
	EdgeReferences EdgeKind = "references"
	EdgeImports    EdgeKind = "imports"
)

// Symbol represents a named entity in the codebase.
type Symbol struct {
	Name     string     `json:"name"`
	Kind     SymbolKind `json:"kind"`
	Package  string     `json:"package"`  // package name (not import path)
	File     string     `json:"file"`     // relative file path
	Line     int        `json:"line"`
	Receiver string     `json:"receiver,omitempty"` // for methods
	Exported bool       `json:"exported"`
	Doc      string     `json:"doc,omitempty"` // doc comment (truncated)

	// For interfaces: method signatures required
	Methods []string `json:"methods,omitempty"`
	// For structs: embedded types
	Embeds []string `json:"embeds,omitempty"`
	// For functions/methods: parameter and return types
	Signature string `json:"signature,omitempty"`
}

// QualifiedName returns the unique qualified name for this symbol.
func (s *Symbol) QualifiedName() string {
	if s.Receiver != "" {
		return fmt.Sprintf("%s.%s.%s", s.Package, s.Receiver, s.Name)
	}
	return fmt.Sprintf("%s.%s", s.Package, s.Name)
}

// Location returns file:line for display.
func (s *Symbol) Location() string {
	return fmt.Sprintf("%s:%d", s.File, s.Line)
}

// Edge represents a relationship between two symbols.
type Edge struct {
	From     string   `json:"from"`      // qualified name or package
	To       string   `json:"to"`        // qualified name or package
	Kind     EdgeKind `json:"kind"`
	File     string   `json:"file,omitempty"` // where the edge originates
	Line     int      `json:"line,omitempty"`
	CallExpr string   `json:"call_expr,omitempty"` // for calls: the expression
}

// Graph is the codebase knowledge graph.
type Graph struct {
	Symbols map[string]*Symbol // keyed by qualified name
	Edges   []Edge

	// Precomputed indexes for fast lookups.
	byName     map[string][]*Symbol   // name -> symbols (may be ambiguous)
	byFile     map[string][]*Symbol   // file -> symbols
	byPackage  map[string][]*Symbol   // package -> symbols
	callersOf  map[string][]Edge      // qualified name -> edges where To == name
	calleesOf  map[string][]Edge      // qualified name -> edges where From == name
	imports    map[string][]string    // package -> imported packages
	importedBy map[string][]string    // package -> packages that import it
}

// NewGraph creates an empty graph.
func NewGraph() *Graph {
	return &Graph{
		Symbols:    make(map[string]*Symbol),
		byName:     make(map[string][]*Symbol),
		byFile:     make(map[string][]*Symbol),
		byPackage:  make(map[string][]*Symbol),
		callersOf:  make(map[string][]Edge),
		calleesOf:  make(map[string][]Edge),
		imports:    make(map[string][]string),
		importedBy: make(map[string][]string),
	}
}

// AddSymbol registers a symbol in the graph.
func (g *Graph) AddSymbol(s *Symbol) {
	qn := s.QualifiedName()
	g.Symbols[qn] = s
	g.byName[s.Name] = append(g.byName[s.Name], s)
	g.byFile[s.File] = append(g.byFile[s.File], s)
	g.byPackage[s.Package] = append(g.byPackage[s.Package], s)
}

// AddEdge records a relationship between symbols.
func (g *Graph) AddEdge(e Edge) {
	g.Edges = append(g.Edges, e)
	switch e.Kind {
	case EdgeCalls:
		g.callersOf[e.To] = append(g.callersOf[e.To], e)
		g.calleesOf[e.From] = append(g.calleesOf[e.From], e)
	case EdgeImports:
		g.imports[e.From] = appendUnique(g.imports[e.From], e.To)
		g.importedBy[e.To] = appendUnique(g.importedBy[e.To], e.From)
	}
}

// LookupSymbol finds a symbol by qualified name, or by unqualified name if unique.
func (g *Graph) LookupSymbol(name string) []*Symbol {
	// Try exact qualified name first.
	if s, ok := g.Symbols[name]; ok {
		return []*Symbol{s}
	}
	// Try unqualified name.
	if syms, ok := g.byName[name]; ok {
		return syms
	}
	// Try receiver.Method format.
	if strings.Contains(name, ".") {
		parts := strings.SplitN(name, ".", 2)
		var results []*Symbol
		for _, s := range g.byName[parts[1]] {
			if s.Receiver == parts[0] {
				results = append(results, s)
			}
		}
		if len(results) > 0 {
			return results
		}
	}
	return nil
}

// Callers returns all direct callers of a symbol.
func (g *Graph) Callers(qualifiedName string) []Edge {
	return g.callersOf[qualifiedName]
}

// TransitiveCallers returns all callers of a symbol, transitively.
func (g *Graph) TransitiveCallers(qualifiedName string, maxDepth int) []Edge {
	if maxDepth <= 0 {
		maxDepth = 10
	}
	var result []Edge
	visited := map[string]bool{qualifiedName: true}
	queue := []string{qualifiedName}

	for depth := 0; depth < maxDepth && len(queue) > 0; depth++ {
		var next []string
		for _, name := range queue {
			for _, e := range g.callersOf[name] {
				result = append(result, e)
				if !visited[e.From] {
					visited[e.From] = true
					next = append(next, e.From)
				}
			}
		}
		queue = next
	}
	return result
}

// Callees returns all functions called by a symbol.
func (g *Graph) Callees(qualifiedName string) []Edge {
	return g.calleesOf[qualifiedName]
}

// PackageImports returns packages imported by the given package.
func (g *Graph) PackageImports(pkg string) []string {
	return g.imports[pkg]
}

// PackageImportedBy returns packages that import the given package.
func (g *Graph) PackageImportedBy(pkg string) []string {
	return g.importedBy[pkg]
}

// SymbolsByPackage returns all symbols in a package.
func (g *Graph) SymbolsByPackage(pkg string) []*Symbol {
	return g.byPackage[pkg]
}

// SymbolsByFile returns all symbols defined in a file.
func (g *Graph) SymbolsByFile(file string) []*Symbol {
	return g.byFile[file]
}

// Implementations finds types that implement a given interface.
func (g *Graph) Implementations(ifaceName string) []*Symbol {
	var results []*Symbol
	for _, e := range g.Edges {
		if e.Kind == EdgeImplements && e.To == ifaceName {
			if s, ok := g.Symbols[e.From]; ok {
				results = append(results, s)
			}
		}
	}
	return results
}

// InterfacesImplementedBy finds interfaces that a type implements.
func (g *Graph) InterfacesImplementedBy(typeName string) []string {
	var results []string
	for _, e := range g.Edges {
		if e.Kind == EdgeImplements && e.From == typeName {
			results = append(results, e.To)
		}
	}
	return results
}

// AllPackages returns all indexed packages.
func (g *Graph) AllPackages() []string {
	var pkgs []string
	seen := make(map[string]bool)
	for _, s := range g.Symbols {
		if !seen[s.Package] {
			seen[s.Package] = true
			pkgs = append(pkgs, s.Package)
		}
	}
	return pkgs
}

// Stats returns summary statistics about the graph.
func (g *Graph) Stats() GraphStats {
	var stats GraphStats
	stats.TotalSymbols = len(g.Symbols)
	stats.TotalEdges = len(g.Edges)
	stats.Packages = len(g.AllPackages())

	for _, s := range g.Symbols {
		switch s.Kind {
		case KindFunction:
			stats.Functions++
		case KindMethod:
			stats.Methods++
		case KindStruct:
			stats.Structs++
		case KindInterface:
			stats.Interfaces++
		}
	}
	for _, e := range g.Edges {
		if e.Kind == EdgeCalls {
			stats.CallEdges++
		}
	}
	return stats
}

// GraphStats holds summary statistics.
type GraphStats struct {
	TotalSymbols int `json:"total_symbols"`
	TotalEdges   int `json:"total_edges"`
	Packages     int `json:"packages"`
	Functions    int `json:"functions"`
	Methods      int `json:"methods"`
	Structs      int `json:"structs"`
	Interfaces   int `json:"interfaces"`
	CallEdges    int `json:"call_edges"`
}

func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}
