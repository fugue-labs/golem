package codegraph

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// Indexer builds a knowledge graph from Go source files.
type Indexer struct {
	rootDir string
	fset    *token.FileSet
	graph   *Graph

	// importAliases maps file -> alias -> import path for resolving
	// package-qualified calls like "fmt.Println".
	importAliases map[string]map[string]string
}

// NewIndexer creates an indexer rooted at the given directory.
func NewIndexer(rootDir string) *Indexer {
	return &Indexer{
		rootDir:       rootDir,
		fset:          token.NewFileSet(),
		graph:         NewGraph(),
		importAliases: make(map[string]map[string]string),
	}
}

// Index parses all Go files under the root directory and builds the graph.
func (idx *Indexer) Index() (*Graph, error) {
	err := filepath.WalkDir(idx.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" ||
				name == "testdata" || name == ".beads" || name == ".runtime" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		// Skip test files for now to reduce noise, but still index _test.go
		// symbols so callers can find test-specific code.
		return idx.indexFile(path)
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	// Second pass: compute interface implementations.
	idx.computeImplementations()

	return idx.graph, nil
}

func (idx *Indexer) indexFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil // skip unreadable files
	}

	file, err := parser.ParseFile(idx.fset, path, src, parser.ParseComments)
	if err != nil {
		return nil // skip unparseable files
	}

	relPath, _ := filepath.Rel(idx.rootDir, path)
	pkgName := file.Name.Name

	// Collect import aliases for this file.
	aliases := make(map[string]string)
	for _, imp := range file.Imports {
		impPath := strings.Trim(imp.Path.Value, `"`)
		var alias string
		if imp.Name != nil {
			alias = imp.Name.Name
		} else {
			// Default alias is the last path segment.
			parts := strings.Split(impPath, "/")
			alias = parts[len(parts)-1]
		}
		aliases[alias] = impPath

		// Record package import edge.
		idx.graph.AddEdge(Edge{
			From: pkgName,
			To:   impPath,
			Kind: EdgeImports,
			File: relPath,
			Line: idx.fset.Position(imp.Pos()).Line,
		})
	}
	idx.importAliases[relPath] = aliases

	// Walk declarations.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			idx.indexFunc(d, pkgName, relPath)
		case *ast.GenDecl:
			idx.indexGenDecl(d, pkgName, relPath)
		}
	}

	return nil
}

func (idx *Indexer) indexFunc(fn *ast.FuncDecl, pkg, file string) {
	pos := idx.fset.Position(fn.Pos())
	sym := &Symbol{
		Name:     fn.Name.Name,
		Package:  pkg,
		File:     file,
		Line:     pos.Line,
		Exported: fn.Name.IsExported(),
		Doc:      truncateDoc(fn.Doc),
	}

	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		sym.Kind = KindMethod
		sym.Receiver = receiverTypeName(fn.Recv.List[0].Type)
	} else {
		sym.Kind = KindFunction
	}

	sym.Signature = formatFuncSignature(fn)
	idx.graph.AddSymbol(sym)

	// Extract calls from function body.
	if fn.Body != nil {
		callerQN := sym.QualifiedName()
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			idx.indexCall(call, callerQN, pkg, file)
			return true
		})
	}
}

func (idx *Indexer) indexCall(call *ast.CallExpr, callerQN, pkg, file string) {
	pos := idx.fset.Position(call.Pos())
	aliases := idx.importAliases[file]

	var targetName string
	var callExpr string

	switch fn := call.Fun.(type) {
	case *ast.Ident:
		// Local function call: e.g., doSomething()
		targetName = pkg + "." + fn.Name
		callExpr = fn.Name

	case *ast.SelectorExpr:
		// Qualified call: e.g., fmt.Println() or obj.Method()
		if ident, ok := fn.X.(*ast.Ident); ok {
			callExpr = ident.Name + "." + fn.Sel.Name
			// Check if it's a package-qualified call.
			if impPath, ok := aliases[ident.Name]; ok {
				// It's a package call. Use the imported package's last segment
				// as the package name (best guess without type info).
				parts := strings.Split(impPath, "/")
				targetName = parts[len(parts)-1] + "." + fn.Sel.Name
			} else {
				// It's a method call on a variable. We can't resolve the type
				// without type checking, so record what we can.
				targetName = ident.Name + "." + fn.Sel.Name
			}
		}

	default:
		return
	}

	if targetName == "" {
		return
	}

	idx.graph.AddEdge(Edge{
		From:     callerQN,
		To:       targetName,
		Kind:     EdgeCalls,
		File:     file,
		Line:     pos.Line,
		CallExpr: callExpr,
	})
}

func (idx *Indexer) indexGenDecl(decl *ast.GenDecl, pkg, file string) {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			idx.indexTypeSpec(s, decl, pkg, file)
		case *ast.ValueSpec:
			idx.indexValueSpec(s, decl, pkg, file)
		}
	}
}

func (idx *Indexer) indexTypeSpec(ts *ast.TypeSpec, decl *ast.GenDecl, pkg, file string) {
	pos := idx.fset.Position(ts.Pos())
	sym := &Symbol{
		Name:     ts.Name.Name,
		Package:  pkg,
		File:     file,
		Line:     pos.Line,
		Exported: ts.Name.IsExported(),
		Doc:      truncateDoc(decl.Doc),
	}

	switch t := ts.Type.(type) {
	case *ast.StructType:
		sym.Kind = KindStruct
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				// Check for embedded types.
				if len(field.Names) == 0 {
					embName := typeExprName(field.Type)
					if embName != "" {
						sym.Embeds = append(sym.Embeds, embName)
						idx.graph.AddEdge(Edge{
							From: pkg + "." + ts.Name.Name,
							To:   embName,
							Kind: EdgeEmbeds,
							File: file,
							Line: idx.fset.Position(field.Pos()).Line,
						})
					}
				} else {
					// Named fields - index them too.
					for _, name := range field.Names {
						fieldSym := &Symbol{
							Name:     name.Name,
							Kind:     KindField,
							Package:  pkg,
							File:     file,
							Line:     idx.fset.Position(name.Pos()).Line,
							Receiver: ts.Name.Name,
							Exported: name.IsExported(),
						}
						idx.graph.AddSymbol(fieldSym)
					}
				}
			}
		}

	case *ast.InterfaceType:
		sym.Kind = KindInterface
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				if len(method.Names) > 0 {
					sym.Methods = append(sym.Methods, method.Names[0].Name)
				} else {
					// Embedded interface.
					embName := typeExprName(method.Type)
					if embName != "" {
						sym.Embeds = append(sym.Embeds, embName)
					}
				}
			}
		}

	default:
		sym.Kind = KindTypeAlias
	}

	idx.graph.AddSymbol(sym)
}

func (idx *Indexer) indexValueSpec(vs *ast.ValueSpec, decl *ast.GenDecl, pkg, file string) {
	kind := KindVariable
	if decl.Tok == token.CONST {
		kind = KindConstant
	}

	for _, name := range vs.Names {
		if name.Name == "_" {
			continue
		}
		sym := &Symbol{
			Name:     name.Name,
			Kind:     kind,
			Package:  pkg,
			File:     file,
			Line:     idx.fset.Position(name.Pos()).Line,
			Exported: name.IsExported(),
			Doc:      truncateDoc(decl.Doc),
		}
		idx.graph.AddSymbol(sym)
	}
}

// computeImplementations checks which struct types implement which interfaces
// based on method sets. This is a structural (duck-typing) check using method
// names only, since we don't have full type information.
func (idx *Indexer) computeImplementations() {
	// Collect interfaces and their required methods.
	var interfaces []*Symbol
	for _, s := range idx.graph.Symbols {
		if s.Kind == KindInterface && len(s.Methods) > 0 {
			interfaces = append(interfaces, s)
		}
	}

	// Collect method sets per type (receiver -> method names).
	typeMethods := make(map[string]map[string]bool)
	for _, s := range idx.graph.Symbols {
		if s.Kind == KindMethod && s.Receiver != "" {
			key := s.Package + "." + s.Receiver
			if typeMethods[key] == nil {
				typeMethods[key] = make(map[string]bool)
			}
			typeMethods[key][s.Name] = true
		}
	}

	// Check each concrete type against each interface.
	for _, iface := range interfaces {
		ifaceQN := iface.QualifiedName()
		for typeName, methods := range typeMethods {
			// Check if type has all interface methods.
			allMatch := true
			for _, m := range iface.Methods {
				if !methods[m] {
					allMatch = false
					break
				}
			}
			if allMatch {
				idx.graph.AddEdge(Edge{
					From: typeName,
					To:   ifaceQN,
					Kind: EdgeImplements,
				})
			}
		}
	}
}

// Helper functions.

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	case *ast.IndexListExpr:
		return receiverTypeName(t.X)
	}
	return ""
}

func typeExprName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeExprName(t.X)
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name + "." + t.Sel.Name
		}
	case *ast.IndexExpr:
		return typeExprName(t.X)
	case *ast.IndexListExpr:
		return typeExprName(t.X)
	}
	return ""
}

func formatFuncSignature(fn *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		b.WriteString("(")
		b.WriteString(receiverTypeName(fn.Recv.List[0].Type))
		b.WriteString(") ")
	}
	b.WriteString(fn.Name.Name)
	b.WriteString("(")
	if fn.Type.Params != nil {
		var params []string
		for _, p := range fn.Type.Params.List {
			typeName := typeExprString(p.Type)
			if len(p.Names) > 0 {
				for _, n := range p.Names {
					params = append(params, n.Name+" "+typeName)
				}
			} else {
				params = append(params, typeName)
			}
		}
		b.WriteString(strings.Join(params, ", "))
	}
	b.WriteString(")")
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		var rets []string
		for _, r := range fn.Type.Results.List {
			rets = append(rets, typeExprString(r.Type))
		}
		if len(rets) == 1 {
			b.WriteString(" " + rets[0])
		} else {
			b.WriteString(" (" + strings.Join(rets, ", ") + ")")
		}
	}
	return b.String()
}

func typeExprString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeExprString(t.X)
	case *ast.SelectorExpr:
		return typeExprString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeExprString(t.Elt)
		}
		return "[...]" + typeExprString(t.Elt)
	case *ast.MapType:
		return "map[" + typeExprString(t.Key) + "]" + typeExprString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + typeExprString(t.Value)
	case *ast.Ellipsis:
		return "..." + typeExprString(t.Elt)
	case *ast.IndexExpr:
		return typeExprString(t.X) + "[" + typeExprString(t.Index) + "]"
	case *ast.IndexListExpr:
		var indices []string
		for _, i := range t.Indices {
			indices = append(indices, typeExprString(i))
		}
		return typeExprString(t.X) + "[" + strings.Join(indices, ", ") + "]"
	}
	return "?"
}

func truncateDoc(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	text := cg.Text()
	// Keep first line only, truncated.
	if idx := strings.IndexByte(text, '\n'); idx != -1 {
		text = text[:idx]
	}
	text = strings.TrimSpace(text)
	if len(text) > 120 {
		text = text[:120] + "..."
	}
	return text
}
