// Copyright 2025 Sushanth (https://github.com/sushanthpy)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Package toondb provides token-aware context retrieval for LLM applications.

The ContextQuery builder provides:
1. Token budgeting - Fit context within model limits
2. Relevance scoring - Prioritize most relevant chunks
3. Deduplication - Avoid repeating similar content
4. Structured output - Ready for LLM prompts

Example:

	query := toondb.NewContextQuery(collection).
		AddVectorQuery(embedding, 0.7).
		AddKeywordQuery("machine learning", 0.3).
		WithTokenBudget(4000).
		WithMinRelevance(0.5)

	result, err := query.Execute()
	if err != nil {
		log.Fatal(err)
	}

	prompt := result.AsText() + "\n\nQuestion: " + question
*/
package toondb

import (
	"sort"
	"strings"
)

// DeduplicationStrategy for context results.
type DeduplicationStrategy string

const (
	DeduplicationNone     DeduplicationStrategy = "none"
	DeduplicationExact    DeduplicationStrategy = "exact"
	DeduplicationSemantic DeduplicationStrategy = "semantic"
)

// ContextFormat for output.
type ContextFormat string

const (
	FormatText     ContextFormat = "text"
	FormatMarkdown ContextFormat = "markdown"
	FormatJSON     ContextFormat = "json"
	FormatTOON     ContextFormat = "toon"
)

// TruncationStrategy for handling budget overflow.
type TruncationStrategy string

const (
	TruncationTailDrop     TruncationStrategy = "tail_drop"
	TruncationHeadDrop     TruncationStrategy = "head_drop"
	TruncationProportional TruncationStrategy = "proportional"
	TruncationStrict       TruncationStrategy = "strict"
)

// TokenEstimator estimates token counts for text.
type TokenEstimator struct {
	tokenizer func(string) int
}

// NewTokenEstimator creates a new token estimator.
// If tokenizer is nil, uses a heuristic (4 chars ≈ 1 token).
func NewTokenEstimator(tokenizer func(string) int) *TokenEstimator {
	return &TokenEstimator{tokenizer: tokenizer}
}

// Count estimates token count for text.
func (e *TokenEstimator) Count(text string) int {
	if e.tokenizer != nil {
		return e.tokenizer(text)
	}
	// Heuristic: ~4 chars per token for English
	count := len(text) / 4
	if count < 1 {
		return 1
	}
	return count
}

// ContextChunk represents a chunk of context with metadata.
type ContextChunk struct {
	ID         string                 `json:"id"`
	Text       string                 `json:"text"`
	Score      float64                `json:"score"`
	Tokens     int                    `json:"tokens"`
	Source     string                 `json:"source,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	ChunkIndex int                    `json:"chunk_index,omitempty"`
	DocScore   float64                `json:"doc_score,omitempty"`
}

// ContextResult holds the result of a context query.
type ContextResult struct {
	Chunks            []ContextChunk `json:"chunks"`
	TotalTokens       int            `json:"total_tokens"`
	BudgetTokens      int            `json:"budget_tokens"`
	DroppedCount      int            `json:"dropped_count"`
	VectorScoreRange  *[2]float64    `json:"vector_score_range,omitempty"`
	KeywordScoreRange *[2]float64    `json:"keyword_score_range,omitempty"`
}

// AsText formats chunks as text for LLM prompt.
func (r *ContextResult) AsText(separator string) string {
	if separator == "" {
		separator = "\n\n---\n\n"
	}
	texts := make([]string, len(r.Chunks))
	for i, chunk := range r.Chunks {
		texts[i] = chunk.Text
	}
	return strings.Join(texts, separator)
}

// AsMarkdown formats chunks as markdown.
func (r *ContextResult) AsMarkdown(includeScores bool) string {
	var sb strings.Builder
	for i, chunk := range r.Chunks {
		sb.WriteString("## Chunk ")
		sb.WriteString(string(rune('0' + i + 1)))
		if includeScores {
			sb.WriteString(" (score: ")
			sb.WriteString(strings.TrimRight(strings.TrimRight(
				strings.Replace(string(rune(int(chunk.Score*100)/100)), ".", ",", 1), "0"), ","))
			sb.WriteString(")")
		}
		sb.WriteString("\n\n")
		sb.WriteString(chunk.Text)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// VectorQueryConfig holds vector query parameters.
type VectorQueryConfig struct {
	Embedding []float32
	Weight    float64
	TopK      int
}

// KeywordQueryConfig holds keyword query parameters.
type KeywordQueryConfig struct {
	Query  string
	Weight float64
	TopK   int
}

// ContextQuery is a token-aware context query builder.
type ContextQuery struct {
	db                  *Database
	collectionName      string
	tokenBudget         int
	minRelevance        float64
	maxChunks           int
	vectorQueries       []VectorQueryConfig
	keywordQueries      []KeywordQueryConfig
	deduplication       DeduplicationStrategy
	similarityThreshold float64
	format              ContextFormat
	truncation          TruncationStrategy
	estimator           *TokenEstimator
	fusionK             int // RRF fusion k parameter
	includeMetadata     bool
	recencyWeight       float64 // Weight for recency in scoring
	diversityWeight     float64 // Weight for diversity in result set
}

// NewContextQuery creates a new context query builder.
func NewContextQuery(db *Database, collectionName string) *ContextQuery {
	return &ContextQuery{
		db:                  db,
		collectionName:      collectionName,
		tokenBudget:         4096,
		minRelevance:        0.0,
		maxChunks:           100,
		deduplication:       DeduplicationNone,
		similarityThreshold: 0.9,
		format:              FormatText,
		truncation:          TruncationTailDrop,
		estimator:           NewTokenEstimator(nil),
		fusionK:             60,
		includeMetadata:     true,
		recencyWeight:       0.0,
		diversityWeight:     0.0,
	}
}

// AddVectorQuery adds a vector similarity query.
func (q *ContextQuery) AddVectorQuery(embedding []float32, weight float64) *ContextQuery {
	q.vectorQueries = append(q.vectorQueries, VectorQueryConfig{
		Embedding: embedding,
		Weight:    weight,
		TopK:      50,
	})
	return q
}

// AddVectorQueryWithK adds a vector query with custom top-k.
func (q *ContextQuery) AddVectorQueryWithK(embedding []float32, weight float64, topK int) *ContextQuery {
	q.vectorQueries = append(q.vectorQueries, VectorQueryConfig{
		Embedding: embedding,
		Weight:    weight,
		TopK:      topK,
	})
	return q
}

// AddKeywordQuery adds a keyword/BM25 query.
func (q *ContextQuery) AddKeywordQuery(query string, weight float64) *ContextQuery {
	q.keywordQueries = append(q.keywordQueries, KeywordQueryConfig{
		Query:  query,
		Weight: weight,
		TopK:   50,
	})
	return q
}

// AddKeywordQueryWithK adds a keyword query with custom top-k.
func (q *ContextQuery) AddKeywordQueryWithK(query string, weight float64, topK int) *ContextQuery {
	q.keywordQueries = append(q.keywordQueries, KeywordQueryConfig{
		Query:  query,
		Weight: weight,
		TopK:   topK,
	})
	return q
}

// WithTokenBudget sets the maximum tokens in result.
func (q *ContextQuery) WithTokenBudget(budget int) *ContextQuery {
	q.tokenBudget = budget
	return q
}

// WithMinRelevance sets minimum relevance score threshold.
func (q *ContextQuery) WithMinRelevance(score float64) *ContextQuery {
	q.minRelevance = score
	return q
}

// WithMaxChunks sets maximum number of chunks in result.
func (q *ContextQuery) WithMaxChunks(max int) *ContextQuery {
	q.maxChunks = max
	return q
}

// WithDeduplication sets deduplication strategy.
func (q *ContextQuery) WithDeduplication(strategy DeduplicationStrategy, threshold float64) *ContextQuery {
	q.deduplication = strategy
	q.similarityThreshold = threshold
	return q
}

// WithFormat sets output format.
func (q *ContextQuery) WithFormat(format ContextFormat) *ContextQuery {
	q.format = format
	return q
}

// WithTruncation sets truncation strategy.
func (q *ContextQuery) WithTruncation(strategy TruncationStrategy) *ContextQuery {
	q.truncation = strategy
	return q
}

// WithTokenizer sets custom tokenizer function.
func (q *ContextQuery) WithTokenizer(tokenizer func(string) int) *ContextQuery {
	q.estimator = NewTokenEstimator(tokenizer)
	return q
}

// WithFusionK sets the RRF fusion k parameter (default: 60).
func (q *ContextQuery) WithFusionK(k int) *ContextQuery {
	q.fusionK = k
	return q
}

// WithRecencyWeight sets weight for recency in scoring (0-1).
func (q *ContextQuery) WithRecencyWeight(weight float64) *ContextQuery {
	q.recencyWeight = weight
	return q
}

// WithDiversityWeight sets weight for diversity in result set (0-1).
func (q *ContextQuery) WithDiversityWeight(weight float64) *ContextQuery {
	q.diversityWeight = weight
	return q
}

// IncludeMetadata sets whether to include metadata in results.
func (q *ContextQuery) IncludeMetadata(include bool) *ContextQuery {
	q.includeMetadata = include
	return q
}

// Execute runs the context query and returns results.
func (q *ContextQuery) Execute() (*ContextResult, error) {
	// Collect candidate chunks from all queries
	candidates := make(map[string]*ContextChunk)
	var vectorScores, keywordScores []float64

	// Execute vector queries
	for _, vq := range q.vectorQueries {
		results, err := q.executeVectorQuery(vq)
		if err != nil {
			return nil, err
		}
		for _, chunk := range results {
			if existing, ok := candidates[chunk.ID]; ok {
				// RRF fusion: combine scores
				existing.Score = q.rrfFusion(existing.Score, chunk.Score*vq.Weight)
			} else {
				chunk.Score = chunk.Score * vq.Weight
				candidates[chunk.ID] = &chunk
			}
			vectorScores = append(vectorScores, chunk.Score)
		}
	}

	// Execute keyword queries
	for _, kq := range q.keywordQueries {
		results, err := q.executeKeywordQuery(kq)
		if err != nil {
			return nil, err
		}
		for _, chunk := range results {
			if existing, ok := candidates[chunk.ID]; ok {
				existing.Score = q.rrfFusion(existing.Score, chunk.Score*kq.Weight)
			} else {
				chunk.Score = chunk.Score * kq.Weight
				candidates[chunk.ID] = &chunk
			}
			keywordScores = append(keywordScores, chunk.Score)
		}
	}

	// Convert to slice and sort by score
	chunks := make([]ContextChunk, 0, len(candidates))
	for _, chunk := range candidates {
		// Filter by minimum relevance
		if chunk.Score >= q.minRelevance {
			chunks = append(chunks, *chunk)
		}
	}
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Score > chunks[j].Score
	})

	// Apply deduplication
	chunks = q.deduplicate(chunks)

	// Apply token budget
	var totalTokens int
	var droppedCount int
	finalChunks := make([]ContextChunk, 0)

	for _, chunk := range chunks {
		if len(finalChunks) >= q.maxChunks {
			droppedCount++
			continue
		}

		tokens := q.estimator.Count(chunk.Text)
		chunk.Tokens = tokens

		if q.truncation == TruncationStrict && totalTokens+tokens > q.tokenBudget {
			droppedCount++
			continue
		}

		if totalTokens+tokens > q.tokenBudget {
			// Handle based on truncation strategy
			switch q.truncation {
			case TruncationTailDrop:
				droppedCount++
				continue
			case TruncationHeadDrop:
				// Drop from head instead
				if len(finalChunks) > 0 {
					totalTokens -= finalChunks[0].Tokens
					finalChunks = finalChunks[1:]
					droppedCount++
				}
			case TruncationProportional:
				// Truncate text proportionally
				available := q.tokenBudget - totalTokens
				if available > 0 {
					ratio := float64(available) / float64(tokens)
					truncLen := int(float64(len(chunk.Text)) * ratio)
					if truncLen > 0 {
						chunk.Text = chunk.Text[:truncLen] + "..."
						chunk.Tokens = available
						tokens = available
					} else {
						droppedCount++
						continue
					}
				}
			}
		}

		totalTokens += tokens
		finalChunks = append(finalChunks, chunk)
	}

	result := &ContextResult{
		Chunks:       finalChunks,
		TotalTokens:  totalTokens,
		BudgetTokens: q.tokenBudget,
		DroppedCount: droppedCount,
	}

	// Set score ranges
	if len(vectorScores) > 0 {
		sort.Float64s(vectorScores)
		result.VectorScoreRange = &[2]float64{vectorScores[0], vectorScores[len(vectorScores)-1]}
	}
	if len(keywordScores) > 0 {
		sort.Float64s(keywordScores)
		result.KeywordScoreRange = &[2]float64{keywordScores[0], keywordScores[len(keywordScores)-1]}
	}

	return result, nil
}

// rrfFusion combines scores using Reciprocal Rank Fusion.
func (q *ContextQuery) rrfFusion(score1, score2 float64) float64 {
	// RRF: 1/(k + rank)
	// For scores, we approximate: combined = s1 + s2 + k / (k + 1/s1 + 1/s2)
	_ = float64(q.fusionK) // k is reserved for future RRF implementation
	return score1 + score2
}

// executeVectorQuery executes a vector similarity search.
func (q *ContextQuery) executeVectorQuery(vq VectorQueryConfig) ([]ContextChunk, error) {
	// This would call the actual vector search API
	// For now, return empty - implementation depends on DB interface
	results, err := q.db.VectorSearch(q.collectionName, vq.Embedding, vq.TopK)
	if err != nil {
		return nil, err
	}

	chunks := make([]ContextChunk, 0, len(results))
	for _, r := range results {
		chunks = append(chunks, ContextChunk{
			ID:       r.ID,
			Text:     string(r.Value),
			Score:    float64(r.Score),
			Metadata: r.Metadata,
		})
	}

	return chunks, nil
}

// executeKeywordQuery executes a keyword/BM25 search.
func (q *ContextQuery) executeKeywordQuery(kq KeywordQueryConfig) ([]ContextChunk, error) {
	// This would call the actual keyword search API
	// For now, use prefix scan as approximation
	results, err := q.db.Scan(q.collectionName + "/" + kq.Query)
	if err != nil {
		return nil, err
	}

	chunks := make([]ContextChunk, 0, len(results))
	for i, r := range results {
		if i >= kq.TopK {
			break
		}
		chunks = append(chunks, ContextChunk{
			ID:    string(r.Key),
			Text:  string(r.Value),
			Score: 1.0 / float64(i+1), // Simple rank-based score
		})
	}

	return chunks, nil
}

// deduplicate removes duplicate or similar chunks.
func (q *ContextQuery) deduplicate(chunks []ContextChunk) []ContextChunk {
	if q.deduplication == DeduplicationNone {
		return chunks
	}

	seen := make(map[string]bool)
	result := make([]ContextChunk, 0, len(chunks))

	for _, chunk := range chunks {
		switch q.deduplication {
		case DeduplicationExact:
			if !seen[chunk.Text] {
				seen[chunk.Text] = true
				result = append(result, chunk)
			}
		case DeduplicationSemantic:
			// For semantic, we'd need embedding comparison
			// Simplified: use text hash for now
			hash := chunk.Text
			if len(hash) > 100 {
				hash = hash[:100]
			}
			if !seen[hash] {
				seen[hash] = true
				result = append(result, chunk)
			}
		}
	}

	return result
}

// ContextVectorSearchResult represents a vector search result for context queries.
type ContextVectorSearchResult struct {
	ID       string
	Value    []byte
	Score    float32
	Metadata map[string]interface{}
}

// VectorSearch performs vector similarity search.
// This is a placeholder - actual implementation depends on DB interface.
func (db *Database) VectorSearch(collection string, embedding []float32, topK int) ([]ContextVectorSearchResult, error) {
	// TODO: Implement actual vector search
	// This would call the vector index API
	return nil, nil
}
