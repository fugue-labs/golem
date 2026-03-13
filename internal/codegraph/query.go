package codegraph

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SymbolFilter defines criteria for filtering symbols.
type SymbolFilter struct {
	Package  string     // filter by package name
	Kind     SymbolKind // filter by kind
	Pattern  string     // regex filter on name
	Exported *bool      // filter by exported status
	File     string     // filter by file path
}

// QuerySymbols returns symbols matching the filter, formatted for display.
func QuerySymbols(g *Graph, filter SymbolFilter) (string, error) {
	var results []*Symbol

	for _, s := range g.Symbols {
		if filter.Package != "" && s.Package != filter.Package {
			continue
		}
		if filter.Kind != "" && s.Kind != filter.Kind {
			continue
		}
		if filter.Exported != nil && s.Exported != *filter.Exported {
			continue
		}
		if filter.File != "" && s.File != filter.File {
			continue
		}
		if filter.Pattern != "" {
			re, err := regexp.Compile(filter.Pattern)
			if err != nil {
				return "", fmt.Errorf("invalid pattern: %w", err)
			}
			if !re.MatchString(s.Name) {
				continue
			}
		}
		// Skip fields in general symbol listing - they're noise.
		if s.Kind == KindField && filter.Kind != KindField {
			continue
		}
		results = append(results, s)
	}

	// Sort by file then line.
	sort.Slice(results, func(i, j int) bool {
		if results[i].File != results[j].File {
			return results[i].File < results[j].File
		}
		return results[i].Line < results[j].Line
	})

	if len(results) == 0 {
		return "(no symbols found)", nil
	}

	const maxResults = 100
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d symbols", len(results))
	if len(results) > maxResults {
		fmt.Fprintf(&b, " (showing first %d)", maxResults)
	}
	b.WriteString(":\n\n")

	shown := results
	if len(shown) > maxResults {
		shown = shown[:maxResults]
	}

	for _, s := range shown {
		exported := " "
		if s.Exported {
			exported = "+"
		}
		if s.Signature != "" {
			fmt.Fprintf(&b, "%s %-10s %s%s  %s\n", exported, s.Kind, s.Location(), pad(s.Location(), 35), s.Signature)
		} else {
			qn := s.QualifiedName()
			fmt.Fprintf(&b, "%s %-10s %s%s  %s\n", exported, s.Kind, s.Location(), pad(s.Location(), 35), qn)
		}
	}

	return b.String(), nil
}

// QueryCallers returns all callers of a symbol, formatted for display.
func QueryCallers(g *Graph, symbolName string, transitive bool, maxDepth int) (string, error) {
	// Resolve the symbol.
	syms := g.LookupSymbol(symbolName)
	if len(syms) == 0 {
		// Try fuzzy matching: look for any symbol containing the name as a suffix.
		for _, s := range g.Symbols {
			if strings.HasSuffix(s.QualifiedName(), "."+symbolName) ||
				strings.HasSuffix(s.QualifiedName(), symbolName) {
				syms = append(syms, s)
			}
		}
	}
	if len(syms) == 0 {
		return fmt.Sprintf("Symbol %q not found in the codebase graph.", symbolName), nil
	}

	var b strings.Builder
	for _, sym := range syms {
		qn := sym.QualifiedName()
		fmt.Fprintf(&b, "Callers of %s (%s):\n", qn, sym.Location())

		var edges []Edge
		if transitive {
			edges = g.TransitiveCallers(qn, maxDepth)
		} else {
			edges = g.Callers(qn)
		}

		if len(edges) == 0 {
			b.WriteString("  (no callers found)\n")
		} else {
			// Deduplicate and sort.
			type callerInfo struct {
				from string
				file string
				line int
				expr string
			}
			seen := make(map[string]bool)
			var callers []callerInfo
			for _, e := range edges {
				key := fmt.Sprintf("%s:%d", e.File, e.Line)
				if !seen[key] {
					seen[key] = true
					callers = append(callers, callerInfo{
						from: e.From,
						file: e.File,
						line: e.Line,
						expr: e.CallExpr,
					})
				}
			}
			sort.Slice(callers, func(i, j int) bool {
				if callers[i].file != callers[j].file {
					return callers[i].file < callers[j].file
				}
				return callers[i].line < callers[j].line
			})

			for _, c := range callers {
				fmt.Fprintf(&b, "  %s:%d  %s", c.file, c.line, c.from)
				if c.expr != "" {
					fmt.Fprintf(&b, "  (%s)", c.expr)
				}
				b.WriteByte('\n')
			}
		}
		b.WriteByte('\n')
	}

	return b.String(), nil
}

// QueryCallees returns all functions called by a symbol, formatted for display.
func QueryCallees(g *Graph, symbolName string) (string, error) {
	syms := g.LookupSymbol(symbolName)
	if len(syms) == 0 {
		return fmt.Sprintf("Symbol %q not found in the codebase graph.", symbolName), nil
	}

	var b strings.Builder
	for _, sym := range syms {
		qn := sym.QualifiedName()
		edges := g.Callees(qn)
		fmt.Fprintf(&b, "Callees of %s (%s):\n", qn, sym.Location())

		if len(edges) == 0 {
			b.WriteString("  (no callees found)\n")
		} else {
			seen := make(map[string]bool)
			for _, e := range edges {
				if !seen[e.To] {
					seen[e.To] = true
					fmt.Fprintf(&b, "  → %s  (%s:%d)\n", e.To, e.File, e.Line)
				}
			}
		}
		b.WriteByte('\n')
	}

	return b.String(), nil
}

// QueryDeps returns package dependencies, formatted for display.
func QueryDeps(g *Graph, pkg, direction string) (string, error) {
	var b strings.Builder

	switch direction {
	case "imports", "":
		deps := g.PackageImports(pkg)
		fmt.Fprintf(&b, "Packages imported by %q:\n", pkg)
		if len(deps) == 0 {
			b.WriteString("  (none found)\n")
		} else {
			sort.Strings(deps)
			for _, d := range deps {
				fmt.Fprintf(&b, "  → %s\n", d)
			}
		}

	case "imported_by", "dependents":
		deps := g.PackageImportedBy(pkg)
		fmt.Fprintf(&b, "Packages that import %q:\n", pkg)
		if len(deps) == 0 {
			b.WriteString("  (none found)\n")
		} else {
			sort.Strings(deps)
			for _, d := range deps {
				fmt.Fprintf(&b, "  ← %s\n", d)
			}
		}

	case "both":
		imports := g.PackageImports(pkg)
		importedBy := g.PackageImportedBy(pkg)
		fmt.Fprintf(&b, "Package %q dependency graph:\n\n", pkg)

		b.WriteString("Imports:\n")
		if len(imports) == 0 {
			b.WriteString("  (none)\n")
		} else {
			sort.Strings(imports)
			for _, d := range imports {
				fmt.Fprintf(&b, "  → %s\n", d)
			}
		}

		b.WriteString("\nImported by:\n")
		if len(importedBy) == 0 {
			b.WriteString("  (none)\n")
		} else {
			sort.Strings(importedBy)
			for _, d := range importedBy {
				fmt.Fprintf(&b, "  ← %s\n", d)
			}
		}

	default:
		return "", fmt.Errorf("invalid direction %q (use: imports, imported_by, both)", direction)
	}

	// Also list all known packages if querying at the project level.
	if pkg == "" || pkg == "." {
		pkgs := g.AllPackages()
		sort.Strings(pkgs)
		b.WriteString("\nAll indexed packages:\n")
		for _, p := range pkgs {
			syms := g.SymbolsByPackage(p)
			fmt.Fprintf(&b, "  %s (%d symbols)\n", p, len(syms))
		}
	}

	return b.String(), nil
}

// QueryTypes returns type hierarchy information, formatted for display.
func QueryTypes(g *Graph, typeName, query string) (string, error) {
	var b strings.Builder

	switch query {
	case "implementations", "implementors", "":
		// Find types that implement an interface.
		syms := g.LookupSymbol(typeName)
		if len(syms) == 0 {
			return fmt.Sprintf("Type %q not found.", typeName), nil
		}

		for _, sym := range syms {
			if sym.Kind != KindInterface {
				// Show type info instead.
				fmt.Fprintf(&b, "Type %s (%s) at %s:\n", sym.QualifiedName(), sym.Kind, sym.Location())
				if len(sym.Embeds) > 0 {
					b.WriteString("  Embeds: " + strings.Join(sym.Embeds, ", ") + "\n")
				}
				if len(sym.Methods) > 0 {
					b.WriteString("  Methods: " + strings.Join(sym.Methods, ", ") + "\n")
				}
				// Show methods defined on this type.
				methods := findMethodsOnType(g, sym)
				if len(methods) > 0 {
					b.WriteString("  Defined methods:\n")
					for _, m := range methods {
						fmt.Fprintf(&b, "    %s  %s\n", m.Location(), m.Signature)
					}
				}
				// Show interfaces implemented.
				ifaces := g.InterfacesImplementedBy(sym.QualifiedName())
				if len(ifaces) > 0 {
					b.WriteString("  Implements: " + strings.Join(ifaces, ", ") + "\n")
				}
				continue
			}

			qn := sym.QualifiedName()
			fmt.Fprintf(&b, "Interface %s at %s\n", qn, sym.Location())
			if len(sym.Methods) > 0 {
				b.WriteString("  Required methods: " + strings.Join(sym.Methods, ", ") + "\n")
			}

			impls := g.Implementations(qn)
			if len(impls) == 0 {
				b.WriteString("  (no implementations found)\n")
			} else {
				b.WriteString("  Implementations:\n")
				for _, impl := range impls {
					fmt.Fprintf(&b, "    %s  %s\n", impl.Location(), impl.QualifiedName())
				}
			}
		}

	case "methods":
		syms := g.LookupSymbol(typeName)
		if len(syms) == 0 {
			return fmt.Sprintf("Type %q not found.", typeName), nil
		}
		for _, sym := range syms {
			methods := findMethodsOnType(g, sym)
			fmt.Fprintf(&b, "Methods on %s (%d):\n", sym.QualifiedName(), len(methods))
			for _, m := range methods {
				fmt.Fprintf(&b, "  %s  %s\n", m.Location(), m.Signature)
			}
		}

	case "hierarchy":
		syms := g.LookupSymbol(typeName)
		if len(syms) == 0 {
			return fmt.Sprintf("Type %q not found.", typeName), nil
		}
		for _, sym := range syms {
			fmt.Fprintf(&b, "Type hierarchy for %s:\n", sym.QualifiedName())

			// Embeds (what this type contains).
			if len(sym.Embeds) > 0 {
				b.WriteString("  Embeds:\n")
				for _, e := range sym.Embeds {
					fmt.Fprintf(&b, "    ↓ %s\n", e)
				}
			}

			// Embedded by (what contains this type).
			var embeddedBy []string
			for _, edge := range g.Edges {
				if edge.Kind == EdgeEmbeds && edge.To == sym.Name {
					embeddedBy = append(embeddedBy, edge.From)
				}
			}
			if len(embeddedBy) > 0 {
				b.WriteString("  Embedded by:\n")
				for _, e := range embeddedBy {
					fmt.Fprintf(&b, "    ↑ %s\n", e)
				}
			}

			// Interfaces.
			ifaces := g.InterfacesImplementedBy(sym.QualifiedName())
			if len(ifaces) > 0 {
				b.WriteString("  Implements:\n")
				for _, i := range ifaces {
					fmt.Fprintf(&b, "    ⊢ %s\n", i)
				}
			}
		}

	default:
		return "", fmt.Errorf("invalid query %q (use: implementations, methods, hierarchy)", query)
	}

	return b.String(), nil
}

// QueryRefs finds all references to a symbol across the codebase.
func QueryRefs(g *Graph, symbolName string) (string, error) {
	syms := g.LookupSymbol(symbolName)
	if len(syms) == 0 {
		return fmt.Sprintf("Symbol %q not found.", symbolName), nil
	}

	var b strings.Builder
	for _, sym := range syms {
		qn := sym.QualifiedName()
		fmt.Fprintf(&b, "References to %s (defined at %s):\n", qn, sym.Location())

		var refs []Edge
		for _, e := range g.Edges {
			if e.To == qn || e.To == sym.Name || e.To == sym.Receiver+"."+sym.Name {
				refs = append(refs, e)
			}
		}

		if len(refs) == 0 {
			b.WriteString("  (no references found)\n")
		} else {
			sort.Slice(refs, func(i, j int) bool {
				if refs[i].File != refs[j].File {
					return refs[i].File < refs[j].File
				}
				return refs[i].Line < refs[j].Line
			})
			seen := make(map[string]bool)
			for _, r := range refs {
				key := fmt.Sprintf("%s:%d", r.File, r.Line)
				if seen[key] {
					continue
				}
				seen[key] = true
				fmt.Fprintf(&b, "  %s:%d  [%s] from %s\n", r.File, r.Line, r.Kind, r.From)
			}
		}
		b.WriteByte('\n')
	}

	return b.String(), nil
}

// QueryStats returns graph statistics.
func QueryStats(g *Graph) string {
	stats := g.Stats()
	var b strings.Builder
	fmt.Fprintf(&b, "Codebase Knowledge Graph:\n")
	fmt.Fprintf(&b, "  Packages:   %d\n", stats.Packages)
	fmt.Fprintf(&b, "  Functions:  %d\n", stats.Functions)
	fmt.Fprintf(&b, "  Methods:    %d\n", stats.Methods)
	fmt.Fprintf(&b, "  Structs:    %d\n", stats.Structs)
	fmt.Fprintf(&b, "  Interfaces: %d\n", stats.Interfaces)
	fmt.Fprintf(&b, "  Total:      %d symbols, %d edges (%d call edges)\n",
		stats.TotalSymbols, stats.TotalEdges, stats.CallEdges)
	return b.String()
}

func findMethodsOnType(g *Graph, sym *Symbol) []*Symbol {
	var methods []*Symbol
	for _, s := range g.Symbols {
		if s.Kind == KindMethod && s.Receiver == sym.Name && s.Package == sym.Package {
			methods = append(methods, s)
		}
	}
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Name < methods[j].Name
	})
	return methods
}

func pad(s string, width int) string {
	if len(s) >= width {
		return "  "
	}
	return strings.Repeat(" ", width-len(s))
}
