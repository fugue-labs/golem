package codegraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testProject creates a temporary Go project for testing.
func testProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestIndexBasicFunction(t *testing.T) {
	dir := testProject(t, map[string]string{
		"main.go": `package main

import "fmt"

// Hello prints a greeting.
func Hello(name string) {
	fmt.Println("Hello,", name)
}

func main() {
	Hello("world")
}
`,
	})

	idx := NewIndexer(dir)
	g, err := idx.Index()
	if err != nil {
		t.Fatal(err)
	}

	// Should find Hello and main.
	syms := g.LookupSymbol("Hello")
	if len(syms) == 0 {
		t.Fatal("expected to find Hello symbol")
	}
	if syms[0].Kind != KindFunction {
		t.Errorf("expected function, got %s", syms[0].Kind)
	}
	if syms[0].Package != "main" {
		t.Errorf("expected package main, got %s", syms[0].Package)
	}
	if !syms[0].Exported {
		t.Error("expected Hello to be exported")
	}
	if syms[0].Doc != "Hello prints a greeting." {
		t.Errorf("unexpected doc: %q", syms[0].Doc)
	}

	// main should call Hello.
	mainSyms := g.LookupSymbol("main.main")
	if len(mainSyms) == 0 {
		t.Fatal("expected to find main")
	}
	callees := g.Callees(mainSyms[0].QualifiedName())
	found := false
	for _, e := range callees {
		if strings.Contains(e.To, "Hello") {
			found = true
		}
	}
	if !found {
		t.Error("expected main to call Hello")
	}

	// Hello should be called by main.
	callers := g.Callers(syms[0].QualifiedName())
	if len(callers) == 0 {
		t.Error("expected Hello to have callers")
	}
}

func TestIndexStructAndMethods(t *testing.T) {
	dir := testProject(t, map[string]string{
		"types.go": `package mypackage

// Server handles HTTP requests.
type Server struct {
	Host string
	Port int
}

func NewServer(host string, port int) *Server {
	return &Server{Host: host, Port: port}
}

func (s *Server) Start() error {
	return nil
}

func (s *Server) Stop() {
}
`,
	})

	idx := NewIndexer(dir)
	g, err := idx.Index()
	if err != nil {
		t.Fatal(err)
	}

	// Should find Server struct.
	serverSyms := g.LookupSymbol("Server")
	if len(serverSyms) == 0 {
		t.Fatal("expected to find Server")
	}
	if serverSyms[0].Kind != KindStruct {
		t.Errorf("expected struct, got %s", serverSyms[0].Kind)
	}

	// Should find methods.
	startSyms := g.LookupSymbol("Server.Start")
	if len(startSyms) == 0 {
		t.Fatal("expected to find Server.Start")
	}
	if startSyms[0].Kind != KindMethod {
		t.Errorf("expected method, got %s", startSyms[0].Kind)
	}
	if startSyms[0].Receiver != "Server" {
		t.Errorf("expected receiver Server, got %s", startSyms[0].Receiver)
	}
}

func TestIndexInterface(t *testing.T) {
	dir := testProject(t, map[string]string{
		"iface.go": `package mypackage

type Reader interface {
	Read(p []byte) (n int, err error)
}

type MyReader struct{}

func (r *MyReader) Read(p []byte) (int, error) {
	return 0, nil
}
`,
	})

	idx := NewIndexer(dir)
	g, err := idx.Index()
	if err != nil {
		t.Fatal(err)
	}

	// Should find Reader interface.
	readerSyms := g.LookupSymbol("Reader")
	if len(readerSyms) == 0 {
		t.Fatal("expected to find Reader")
	}
	if readerSyms[0].Kind != KindInterface {
		t.Errorf("expected interface, got %s", readerSyms[0].Kind)
	}
	if len(readerSyms[0].Methods) != 1 || readerSyms[0].Methods[0] != "Read" {
		t.Errorf("unexpected methods: %v", readerSyms[0].Methods)
	}

	// MyReader should implement Reader.
	impls := g.Implementations(readerSyms[0].QualifiedName())
	if len(impls) == 0 {
		t.Error("expected MyReader to implement Reader")
	}
}

func TestIndexImports(t *testing.T) {
	dir := testProject(t, map[string]string{
		"main.go": `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Args)
}
`,
	})

	idx := NewIndexer(dir)
	g, err := idx.Index()
	if err != nil {
		t.Fatal(err)
	}

	imports := g.PackageImports("main")
	if len(imports) != 2 {
		t.Errorf("expected 2 imports, got %d: %v", len(imports), imports)
	}

	hasFmt := false
	hasOS := false
	for _, imp := range imports {
		if imp == "fmt" {
			hasFmt = true
		}
		if imp == "os" {
			hasOS = true
		}
	}
	if !hasFmt {
		t.Error("expected fmt import")
	}
	if !hasOS {
		t.Error("expected os import")
	}
}

func TestTransitiveCallers(t *testing.T) {
	dir := testProject(t, map[string]string{
		"chain.go": `package mypackage

func A() {
	B()
}

func B() {
	C()
}

func C() {
}
`,
	})

	idx := NewIndexer(dir)
	g, err := idx.Index()
	if err != nil {
		t.Fatal(err)
	}

	// C is called by B, which is called by A.
	cSyms := g.LookupSymbol("C")
	if len(cSyms) == 0 {
		t.Fatal("expected to find C")
	}

	transitive := g.TransitiveCallers(cSyms[0].QualifiedName(), 10)
	if len(transitive) < 2 {
		t.Errorf("expected at least 2 transitive callers, got %d", len(transitive))
	}

	// Should include both B and A as transitive callers.
	callerNames := make(map[string]bool)
	for _, e := range transitive {
		callerNames[e.From] = true
	}
	if !callerNames["mypackage.B"] {
		t.Error("expected B in transitive callers")
	}
	if !callerNames["mypackage.A"] {
		t.Error("expected A in transitive callers")
	}
}

func TestMultiplePackages(t *testing.T) {
	dir := testProject(t, map[string]string{
		"pkg/a.go": `package pkg

func Helper() string {
	return "help"
}
`,
		"main.go": `package main

func main() {
}
`,
	})

	idx := NewIndexer(dir)
	g, err := idx.Index()
	if err != nil {
		t.Fatal(err)
	}

	pkgs := g.AllPackages()
	if len(pkgs) < 2 {
		t.Errorf("expected at least 2 packages, got %d: %v", len(pkgs), pkgs)
	}
}

func TestEmbeddedStruct(t *testing.T) {
	dir := testProject(t, map[string]string{
		"embed.go": `package mypackage

type Base struct {
	ID int
}

type Extended struct {
	Base
	Name string
}
`,
	})

	idx := NewIndexer(dir)
	g, err := idx.Index()
	if err != nil {
		t.Fatal(err)
	}

	extSyms := g.LookupSymbol("Extended")
	if len(extSyms) == 0 {
		t.Fatal("expected to find Extended")
	}
	if len(extSyms[0].Embeds) != 1 || extSyms[0].Embeds[0] != "Base" {
		t.Errorf("expected Extended to embed Base, got: %v", extSyms[0].Embeds)
	}

	// Should have an embeds edge.
	hasEmbed := false
	for _, e := range g.Edges {
		if e.Kind == EdgeEmbeds && strings.Contains(e.From, "Extended") && e.To == "Base" {
			hasEmbed = true
		}
	}
	if !hasEmbed {
		t.Error("expected embeds edge from Extended to Base")
	}
}

func TestQuerySymbolsFormatting(t *testing.T) {
	dir := testProject(t, map[string]string{
		"main.go": `package main

func Hello() {}
func World() {}
func helper() {}
`,
	})

	idx := NewIndexer(dir)
	g, err := idx.Index()
	if err != nil {
		t.Fatal(err)
	}

	// Query all symbols.
	result, err := QuerySymbols(g, SymbolFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Hello") {
		t.Error("expected Hello in output")
	}
	if !strings.Contains(result, "World") {
		t.Error("expected World in output")
	}

	// Query exported only.
	exported := true
	result, err = QuerySymbols(g, SymbolFilter{Exported: &exported})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Hello") {
		t.Error("expected Hello in exported output")
	}
	if strings.Contains(result, "helper") {
		t.Error("did not expect helper in exported output")
	}
}

func TestGraphStats(t *testing.T) {
	dir := testProject(t, map[string]string{
		"main.go": `package main

type Foo struct{}
type Bar interface{ Do() }
func (f *Foo) Do() {}
func main() {}
`,
	})

	idx := NewIndexer(dir)
	g, err := idx.Index()
	if err != nil {
		t.Fatal(err)
	}

	stats := g.Stats()
	if stats.Packages != 1 {
		t.Errorf("expected 1 package, got %d", stats.Packages)
	}
	if stats.Functions < 1 {
		t.Error("expected at least 1 function")
	}
	if stats.Methods < 1 {
		t.Error("expected at least 1 method")
	}
	if stats.Structs < 1 {
		t.Error("expected at least 1 struct")
	}
	if stats.Interfaces < 1 {
		t.Error("expected at least 1 interface")
	}
}
