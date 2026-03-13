package search

import (
	"testing"
)

func TestTokenize(t *testing.T) {
	tokens := tokenize("Hello World! This is a test_123.")
	// "this", "is", "a" are stop words; "hello", "world", "test_123" should remain.
	want := map[string]bool{"hello": true, "world": true, "test_123": true}
	got := make(map[string]bool)
	for _, tok := range tokens {
		got[tok] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("expected token %q, not found in %v", w, tokens)
		}
	}
}

func TestIndexSearchBasic(t *testing.T) {
	idx := NewIndex()
	idx.Add("doc1", "How to fix a flaky test in the CI pipeline")
	idx.Add("doc2", "Database migration guide for PostgreSQL")
	idx.Add("doc3", "Authentication bug with OAuth tokens expiring")

	results := idx.Search("flaky test", 10)
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'flaky test'")
	}
	if results[0].DocID != "doc1" {
		t.Errorf("expected doc1 as top result, got %s", results[0].DocID)
	}
}

func TestIndexSearchNoResults(t *testing.T) {
	idx := NewIndex()
	idx.Add("doc1", "Hello world program in Go")

	results := idx.Search("quantum physics", 10)
	if len(results) != 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestIndexSearchMultipleTerms(t *testing.T) {
	idx := NewIndex()
	idx.Add("doc1", "OAuth authentication error handling")
	idx.Add("doc2", "OAuth token refresh implementation")
	idx.Add("doc3", "Basic error handling patterns in Go")

	results := idx.Search("OAuth error", 10)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// doc1 has both "OAuth" and "error", should rank highest.
	if results[0].DocID != "doc1" {
		t.Errorf("expected doc1 as top result, got %s", results[0].DocID)
	}
}

func TestIndexSearchMaxResults(t *testing.T) {
	idx := NewIndex()
	for i := 0; i < 20; i++ {
		idx.Add("doc"+string(rune('A'+i)), "common search term found here")
	}
	results := idx.Search("common search", 5)
	if len(results) > 5 {
		t.Errorf("expected at most 5 results, got %d", len(results))
	}
}

func TestIndexEmpty(t *testing.T) {
	idx := NewIndex()
	results := idx.Search("anything", 10)
	if len(results) != 0 {
		t.Errorf("expected no results from empty index, got %d", len(results))
	}
}

func TestIndexLen(t *testing.T) {
	idx := NewIndex()
	if idx.Len() != 0 {
		t.Errorf("expected 0, got %d", idx.Len())
	}
	idx.Add("d1", "hello")
	idx.Add("d2", "world")
	if idx.Len() != 2 {
		t.Errorf("expected 2, got %d", idx.Len())
	}
}

func TestExtractSnippet(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog and finds a bug in the authentication module"
	snippet := extractSnippet(text, "authentication", 60)
	if snippet == "" {
		t.Error("expected non-empty snippet")
	}
	// Snippet should contain the query term area.
	if len(snippet) > 80 { // Allow some padding for ellipsis.
		t.Errorf("snippet too long: %d chars", len(snippet))
	}
}
