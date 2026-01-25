// HybridRetriever for Memory System
//
// Combines BM25 (lexical) and vector similarity (semantic) with Reciprocal Rank Fusion (RRF).

package sochdb

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/sochdb/sochdb-go/embedded"
)

// HybridRetriever combines lexical and semantic search
type HybridRetriever struct {
	db        *embedded.Database
	namespace string
	config    *RetrievalConfig
	prefix    []byte
	bm25      *BM25Scorer
}

// BM25Scorer implements BM25 scoring
type BM25Scorer struct {
	k1              float64
	b               float64
	documentCount   int
	avgDocLength    float64
	termDocFreq     map[string]int
	documentLengths map[string]int
	documents       map[string]string
}

// NewBM25Scorer creates a new BM25 scorer
func NewBM25Scorer(k1, b float64) *BM25Scorer {
	return &BM25Scorer{
		k1:              k1,
		b:               b,
		termDocFreq:     make(map[string]int),
		documentLengths: make(map[string]int),
		documents:       make(map[string]string),
	}
}

// IndexDocuments indexes documents for BM25
func (bm *BM25Scorer) IndexDocuments(docs map[string]string) {
	bm.documents = docs
	bm.documentCount = len(docs)
	totalLength := 0

	// Calculate document lengths and term frequencies
	for id, text := range docs {
		terms := tokenize(text)
		bm.documentLengths[id] = len(terms)
		totalLength += len(terms)

		// Track unique terms in this document
		seen := make(map[string]bool)
		for _, term := range terms {
			if !seen[term] {
				bm.termDocFreq[term]++
				seen[term] = true
			}
		}
	}

	if bm.documentCount > 0 {
		bm.avgDocLength = float64(totalLength) / float64(bm.documentCount)
	}
}

// Score calculates BM25 score for a query against a document
func (bm *BM25Scorer) Score(query string, docID string) float64 {
	queryTerms := tokenize(query)
	docText, exists := bm.documents[docID]
	if !exists {
		return 0
	}
	docTerms := tokenize(docText)

	// Count term frequencies in document
	termFreqs := make(map[string]int)
	for _, term := range docTerms {
		termFreqs[term]++
	}

	score := 0.0
	docLength := float64(bm.documentLengths[docID])

	for _, term := range queryTerms {
		tf := float64(termFreqs[term])
		df := float64(bm.termDocFreq[term])

		if df == 0 {
			continue
		}

		// IDF calculation
		idf := math.Log((float64(bm.documentCount)-df+0.5)/(df+0.5) + 1.0)

		// BM25 formula
		numerator := tf * (bm.k1 + 1)
		denominator := tf + bm.k1*(1-bm.b+bm.b*(docLength/bm.avgDocLength))
		score += idf * (numerator / denominator)
	}

	return score
}

// NewHybridRetriever creates a new hybrid retriever
func NewHybridRetriever(db *embedded.Database, namespace string, config *RetrievalConfig) *HybridRetriever {
	if config == nil {
		config = &RetrievalConfig{
			Limit:           10,
			LexicalWeight:   0.3,
			SemanticWeight:  0.7,
			RRFConstant:     60,
			PrefilterRatio:  3.0,
			UsePrefiltering: true,
		}
	}

	return &HybridRetriever{
		db:        db,
		namespace: namespace,
		config:    config,
		prefix:    []byte(fmt.Sprintf("retrieval:%s:", namespace)),
		bm25:      NewBM25Scorer(1.5, 0.75),
	}
}

// IndexDocuments indexes documents for retrieval
func (hr *HybridRetriever) IndexDocuments(documents map[string]map[string]interface{}) error {
	// Store documents
	for id, doc := range documents {
		key := append(hr.prefix, []byte(fmt.Sprintf("doc:%s", id))...)
		data, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("failed to marshal document: %w", err)
		}
		if err := hr.db.Put(key, data); err != nil {
			return fmt.Errorf("failed to store document: %w", err)
		}
	}

	// Build BM25 index
	textMap := make(map[string]string)
	for id, doc := range documents {
		if text, ok := doc["text"].(string); ok {
			textMap[id] = text
		}
	}
	hr.bm25.IndexDocuments(textMap)

	return nil
}

// Retrieve performs hybrid retrieval
func (hr *HybridRetriever) Retrieve(query string, allowed AllowedSet) ([]map[string]interface{}, error) {
	// Get all documents
	documents, err := hr.getAllDocuments()
	if err != nil {
		return nil, err
	}

	// Pre-filter by AllowedSet
	filtered := []map[string]interface{}{}
	for id, doc := range documents {
		if allowed.IsAllowed(id, doc) {
			filtered = append(filtered, doc)
		}
	}

	if len(filtered) == 0 {
		return []map[string]interface{}{}, nil
	}

	// Calculate lexical scores (BM25)
	lexicalScores := make(map[string]float64)
	for _, doc := range filtered {
		id := doc["id"].(string)
		lexicalScores[id] = hr.bm25.Score(query, id)
	}

	// Calculate semantic scores (cosine similarity)
	semanticScores := make(map[string]float64)
	for _, doc := range filtered {
		id := doc["id"].(string)
		if text, ok := doc["text"].(string); ok {
			semanticScores[id] = hr.cosineSimilarity(query, text)
		}
	}

	// Combine with RRF
	combined := hr.reciprocalRankFusion(lexicalScores, semanticScores, hr.config.RRFConstant)

	// Sort by score
	type scoredDoc struct {
		doc   map[string]interface{}
		score float64
	}
	scored := make([]scoredDoc, 0, len(filtered))
	for _, doc := range filtered {
		id := doc["id"].(string)
		score := combined[id]
		scored = append(scored, scoredDoc{doc: doc, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Limit results
	limit := hr.config.Limit
	if limit > len(scored) {
		limit = len(scored)
	}

	results := make([]map[string]interface{}, limit)
	for i := 0; i < limit; i++ {
		results[i] = scored[i].doc
		results[i]["_score"] = scored[i].score
	}

	return results, nil
}

// Explain retrieval for debugging
func (hr *HybridRetriever) Explain(query string, docID string) map[string]interface{} {
	lexicalScore := hr.bm25.Score(query, docID)

	doc, err := hr.getDocument(docID)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	semanticScore := 0.0
	if text, ok := doc["text"].(string); ok {
		semanticScore = hr.cosineSimilarity(query, text)
	}

	combined := hr.reciprocalRankFusion(
		map[string]float64{docID: lexicalScore},
		map[string]float64{docID: semanticScore},
		hr.config.RRFConstant,
	)

	return map[string]interface{}{
		"lexical_score":  lexicalScore,
		"semantic_score": semanticScore,
		"combined_score": combined[docID],
		"weights": map[string]float64{
			"lexical":  hr.config.LexicalWeight,
			"semantic": hr.config.SemanticWeight,
		},
	}
}

// Reciprocal Rank Fusion
func (hr *HybridRetriever) reciprocalRankFusion(lexical, semantic map[string]float64, k int) map[string]float64 {
	// Rank documents by lexical scores
	lexicalRanks := hr.rankScores(lexical)

	// Rank documents by semantic scores
	semanticRanks := hr.rankScores(semantic)

	// Combine with RRF
	combined := make(map[string]float64)
	allIDs := make(map[string]bool)
	for id := range lexical {
		allIDs[id] = true
	}
	for id := range semantic {
		allIDs[id] = true
	}

	for id := range allIDs {
		lexRank := lexicalRanks[id]
		semRank := semanticRanks[id]

		score := hr.config.LexicalWeight/(float64(k)+float64(lexRank)) +
			hr.config.SemanticWeight/(float64(k)+float64(semRank))
		combined[id] = score
	}

	return combined
}

// Rank scores (higher scores = lower rank numbers)
func (hr *HybridRetriever) rankScores(scores map[string]float64) map[string]int {
	type idScore struct {
		id    string
		score float64
	}
	sorted := make([]idScore, 0, len(scores))
	for id, score := range scores {
		sorted = append(sorted, idScore{id, score})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	ranks := make(map[string]int)
	for i, item := range sorted {
		ranks[item.id] = i + 1
	}

	// Assign max rank to missing IDs
	maxRank := len(scores) + 1
	for id := range scores {
		if _, exists := ranks[id]; !exists {
			ranks[id] = maxRank
		}
	}

	return ranks
}

// Cosine similarity (simple word overlap)
func (hr *HybridRetriever) cosineSimilarity(query, text string) float64 {
	queryTerms := tokenize(query)
	textTerms := tokenize(text)

	if len(queryTerms) == 0 || len(textTerms) == 0 {
		return 0
	}

	// Build frequency maps
	queryFreq := make(map[string]int)
	textFreq := make(map[string]int)
	for _, term := range queryTerms {
		queryFreq[term]++
	}
	for _, term := range textTerms {
		textFreq[term]++
	}

	// Calculate dot product
	dotProduct := 0.0
	for term, qf := range queryFreq {
		if tf, exists := textFreq[term]; exists {
			dotProduct += float64(qf * tf)
		}
	}

	// Calculate magnitudes
	queryMag := 0.0
	for _, count := range queryFreq {
		queryMag += float64(count * count)
	}
	queryMag = math.Sqrt(queryMag)

	textMag := 0.0
	for _, count := range textFreq {
		textMag += float64(count * count)
	}
	textMag = math.Sqrt(textMag)

	if queryMag == 0 || textMag == 0 {
		return 0
	}

	return dotProduct / (queryMag * textMag)
}

// Get all documents
func (hr *HybridRetriever) getAllDocuments() (map[string]map[string]interface{}, error) {
	documents := make(map[string]map[string]interface{})
	docPrefix := append(hr.prefix, []byte("doc:")...)

	txn := hr.db.Begin()
	defer txn.Abort()

	iter := txn.ScanPrefix(docPrefix)
	defer iter.Close()

	for {
		key, value, ok := iter.Next()
		if !ok {
			break
		}

		var doc map[string]interface{}
		if err := json.Unmarshal(value, &doc); err != nil {
			continue
		}

		// Extract ID from key
		id := string(key[len(docPrefix):])
		doc["id"] = id
		documents[id] = doc
	}

	_ = txn.Commit()
	return documents, nil
}

// Get a single document
func (hr *HybridRetriever) getDocument(docID string) (map[string]interface{}, error) {
	key := append(hr.prefix, []byte(fmt.Sprintf("doc:%s", docID))...)
	value, err := hr.db.Get(key)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(value, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal document: %w", err)
	}

	doc["id"] = docID
	return doc, nil
}

// Tokenize text into terms
func tokenize(text string) []string {
	// Simple tokenization: lowercase and split on whitespace
	text = strings.ToLower(text)
	terms := strings.Fields(text)

	// Remove punctuation
	cleaned := make([]string, 0, len(terms))
	for _, term := range terms {
		// Remove common punctuation
		term = strings.Trim(term, ".,!?;:()[]{}\"'")
		if len(term) > 0 {
			cleaned = append(cleaned, term)
		}
	}

	return cleaned
}
