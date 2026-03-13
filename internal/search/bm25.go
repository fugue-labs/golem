package search

import (
	"math"
	"strings"
	"unicode"
)

// BM25 parameters.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// Document represents a single indexed document.
type Document struct {
	ID     string // unique identifier (e.g., session file path)
	Text   string // original text content
	tokens []string
}

// Index is an in-memory BM25 full-text search index.
type Index struct {
	docs     []Document
	docFreq  map[string]int     // term -> number of docs containing term
	termFreq []map[string]int   // per-doc term frequency
	docLen   []int              // token count per doc
	avgDL    float64            // average document length
}

// NewIndex creates an empty BM25 index.
func NewIndex() *Index {
	return &Index{
		docFreq: make(map[string]int),
	}
}

// Add indexes a document. The ID should be unique.
func (idx *Index) Add(id, text string) {
	tokens := tokenize(text)
	tf := make(map[string]int, len(tokens))
	seen := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		tf[t]++
		if !seen[t] {
			idx.docFreq[t]++
			seen[t] = true
		}
	}

	idx.docs = append(idx.docs, Document{ID: id, Text: text, tokens: tokens})
	idx.termFreq = append(idx.termFreq, tf)
	idx.docLen = append(idx.docLen, len(tokens))

	// Recompute average doc length.
	total := 0
	for _, dl := range idx.docLen {
		total += dl
	}
	idx.avgDL = float64(total) / float64(len(idx.docLen))
}

// Result is a single search hit.
type Result struct {
	DocID string
	Score float64
	Doc   *Document
}

// Search returns documents matching the query, ranked by BM25 score.
// Returns at most maxResults results.
func (idx *Index) Search(query string, maxResults int) []Result {
	if len(idx.docs) == 0 {
		return nil
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	n := float64(len(idx.docs))
	scores := make([]float64, len(idx.docs))

	for _, qt := range queryTokens {
		df, ok := idx.docFreq[qt]
		if !ok {
			continue
		}
		// IDF with smoothing.
		idf := math.Log((n-float64(df)+0.5)/(float64(df)+0.5) + 1)

		for i, tf := range idx.termFreq {
			f := float64(tf[qt])
			if f == 0 {
				continue
			}
			dl := float64(idx.docLen[i])
			denom := f + bm25K1*(1-bm25B+bm25B*dl/idx.avgDL)
			scores[i] += idf * (f * (bm25K1 + 1)) / denom
		}
	}

	// Collect non-zero results and sort by score descending.
	var results []Result
	for i, s := range scores {
		if s > 0 {
			results = append(results, Result{
				DocID: idx.docs[i].ID,
				Score: s,
				Doc:   &idx.docs[i],
			})
		}
	}

	// Simple insertion sort (results are typically small).
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}
	return results
}

// Len returns the number of indexed documents.
func (idx *Index) Len() int {
	return len(idx.docs)
}

// tokenize splits text into lowercase word tokens.
func tokenize(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	// Filter very short tokens and stop words.
	filtered := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) < 2 || stopWords[w] {
			continue
		}
		filtered = append(filtered, w)
	}
	return filtered
}

var stopWords = map[string]bool{
	"the": true, "is": true, "at": true, "which": true, "on": true,
	"a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "of": true, "to": true, "for": true, "with": true,
	"it": true, "this": true, "that": true, "be": true, "as": true,
	"was": true, "are": true, "were": true, "been": true, "has": true,
	"have": true, "had": true, "do": true, "does": true, "did": true,
	"will": true, "would": true, "could": true, "should": true,
	"not": true, "no": true, "if": true, "then": true, "so": true,
	"from": true, "by": true, "we": true, "you": true, "he": true,
	"she": true, "they": true, "my": true, "your": true, "its": true,
}
