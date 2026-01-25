// Package sochdb provides a Memory System for LLM applications.
//
// The memory system enables structured knowledge extraction, multi-source
// fact consolidation, and hybrid retrieval (BM25 + semantic search).
// It follows an event-sourced architecture with immutable assertions
// and derived canonical facts.
package sochdb

// ============================================================================
// Core Data Types - Extracted Knowledge Structures
// ============================================================================

// Entity represents a named entity extracted from text.
// Entities are typed objects with properties and confidence scores.
type Entity struct {
	ID         string                 `json:"id,omitempty"`         // Unique identifier
	Name       string                 `json:"name"`                 // Entity name
	EntityType string                 `json:"entity_type"`          // Type classification
	Properties map[string]interface{} `json:"properties,omitempty"` // Additional attributes
	Confidence float64                `json:"confidence,omitempty"` // Extraction confidence [0-1]
	Provenance string                 `json:"provenance,omitempty"` // Source reference
	Timestamp  int64                  `json:"timestamp,omitempty"`  // Unix timestamp
}

// Relation represents a typed relationship between two entities.
// Relations capture semantic connections with optional properties.
type Relation struct {
	ID           string                 `json:"id,omitempty"`         // Unique identifier
	FromEntity   string                 `json:"from_entity"`          // Source entity
	RelationType string                 `json:"relation_type"`        // Relationship type
	ToEntity     string                 `json:"to_entity"`            // Target entity
	Properties   map[string]interface{} `json:"properties,omitempty"` // Relation attributes
	Confidence   float64                `json:"confidence,omitempty"` // Extraction confidence [0-1]
	Provenance   string                 `json:"provenance,omitempty"` // Source reference
	Timestamp    int64                  `json:"timestamp,omitempty"`  // Unix timestamp
}

// Assertion represents a subject-predicate-object triple.
// Assertions capture factual statements in RDF-like format.
type Assertion struct {
	ID         string  `json:"id,omitempty"`         // Unique identifier
	Subject    string  `json:"subject"`              // Subject entity
	Predicate  string  `json:"predicate"`            // Predicate/relation
	Object     string  `json:"object"`               // Object value
	Confidence float64 `json:"confidence,omitempty"` // Extraction confidence [0-1]
	Provenance string  `json:"provenance,omitempty"` // Source reference
	Timestamp  int64   `json:"timestamp,omitempty"`  // Unix timestamp
}

// ============================================================================
// Consolidation Types - Multi-Source Fact Merging
// ============================================================================

// RawAssertion represents an immutable assertion event from a source.
// Raw assertions are never modified, only superseded by new events.
type RawAssertion struct {
	ID         string                 `json:"id,omitempty"`        // Unique identifier
	Fact       map[string]interface{} `json:"fact"`                // Factual claim
	Source     string                 `json:"source"`              // Source identifier
	Confidence float64                `json:"confidence"`          // Source confidence [0-1]
	Timestamp  int64                  `json:"timestamp,omitempty"` // Unix timestamp
}

// CanonicalFact represents the consolidated truth derived from multiple assertions.
// Canonical facts are recomputed during consolidation from raw assertion events.
type CanonicalFact struct {
	ID         string                 `json:"id"`                    // Unique identifier
	MergedFact map[string]interface{} `json:"merged_fact"`           // Consolidated fact
	Confidence float64                `json:"confidence"`            // Merged confidence
	Sources    []string               `json:"sources"`               // Contributing sources
	ValidFrom  int64                  `json:"valid_from"`            // Validity start time
	ValidUntil *int64                 `json:"valid_until,omitempty"` // Validity end time
}

// ============================================================================
// Extraction Types - LLM Integration
// ============================================================================

// ExtractionResult contains all knowledge extracted from text.
// This is typically returned by LLM extraction functions.
type ExtractionResult struct {
	Entities   []Entity    `json:"entities"`   // Extracted entities
	Relations  []Relation  `json:"relations"`  // Extracted relations
	Assertions []Assertion `json:"assertions"` // Extracted assertions
}

// ExtractionSchema defines validation rules for extraction.
// Schemas ensure type safety and quality control.
type ExtractionSchema struct {
	EntityTypes       []string `json:"entity_types,omitempty"`       // Allowed entity types
	RelationTypes     []string `json:"relation_types,omitempty"`     // Allowed relation types
	MinConfidence     float64  `json:"min_confidence,omitempty"`     // Minimum confidence threshold
	RequireProvenance bool     `json:"require_provenance,omitempty"` // Require source tracking
}

// ============================================================================
// Configuration Types
// ============================================================================

// ConsolidationConfig controls consolidation behavior.
type ConsolidationConfig struct {
	SimilarityThreshold float64 `json:"similarity_threshold,omitempty"` // Fact similarity threshold [0-1]
	UseTemporalUpdates  bool    `json:"use_temporal_updates,omitempty"` // Enable time-based superseding
	MaxConflictAge      int64   `json:"max_conflict_age,omitempty"`     // Conflict validity in seconds
}

// RetrievalConfig controls hybrid search behavior.
type RetrievalConfig struct {
	Limit           int     `json:"limit,omitempty"`            // Maximum results to return
	LexicalWeight   float64 `json:"lexical_weight,omitempty"`   // BM25 weight [0-1]
	SemanticWeight  float64 `json:"semantic_weight,omitempty"`  // Vector weight [0-1]
	RRFConstant     int     `json:"rrf_constant,omitempty"`     // Reciprocal Rank Fusion constant
	PrefilterRatio  float64 `json:"prefilter_ratio,omitempty"`  // Pre-filter expansion ratio
	UsePrefiltering bool    `json:"use_prefiltering,omitempty"` // Enable pre-filtering
}

// RetrievalResult from search
type RetrievalResult struct {
	ID          string                 `json:"id"`
	Score       float64                `json:"score"`
	Content     string                 `json:"content"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	VectorRank  *int                   `json:"vector_rank,omitempty"`
	KeywordRank *int                   `json:"keyword_rank,omitempty"`
}

// RetrievalResponse from retrieval
type RetrievalResponse struct {
	Results      []RetrievalResult `json:"results"`
	QueryTime    int64             `json:"query_time"` // milliseconds
	TotalResults int               `json:"total_results"`
}

// ============================================================================
// Retrieval Types - Pre-filtering and Results
// ============================================================================

// AllowedSet defines the interface for document pre-filtering.
// Implementations control which documents are eligible for retrieval.
type AllowedSet interface {
	// IsAllowed returns true if the document should be considered for retrieval.
	IsAllowed(id string, metadata map[string]interface{}) bool
}

// IdsAllowedSet filters documents by explicit ID whitelist.
type IdsAllowedSet struct {
	ids map[string]bool
}

// NewIdsAllowedSet creates a new ID-based filter.
func NewIdsAllowedSet(ids []string) *IdsAllowedSet {
	idMap := make(map[string]bool, len(ids))
	for _, id := range ids {
		idMap[id] = true
	}
	return &IdsAllowedSet{ids: idMap}
}

// IsAllowed checks if the document ID is in the whitelist.
func (a *IdsAllowedSet) IsAllowed(id string, _ map[string]interface{}) bool {
	return a.ids[id]
}

// NamespaceAllowedSet filters documents by namespace prefix.
type NamespaceAllowedSet struct {
	namespace string
}

// NewNamespaceAllowedSet creates a new namespace-based filter.
func NewNamespaceAllowedSet(namespace string) *NamespaceAllowedSet {
	return &NamespaceAllowedSet{namespace: namespace}
}

// IsAllowed checks if the document ID has the namespace prefix.
func (s *NamespaceAllowedSet) IsAllowed(id string, _ map[string]interface{}) bool {
	prefix1 := s.namespace + "_"
	prefix2 := s.namespace + ":"
	return len(id) >= len(prefix1) && (id[:len(prefix1)] == prefix1 || id[:len(prefix2)] == prefix2)
}

// FilterAllowedSet filters documents using a custom function.
type FilterAllowedSet struct {
	FilterFn func(string, map[string]interface{}) bool
}

// NewFilterAllowedSet creates a new function-based filter.
func NewFilterAllowedSet(filterFn func(string, map[string]interface{}) bool) *FilterAllowedSet {
	return &FilterAllowedSet{FilterFn: filterFn}
}

// IsAllowed applies the custom filter function.
func (s *FilterAllowedSet) IsAllowed(id string, metadata map[string]interface{}) bool {
	return s.FilterFn(id, metadata)
}

// AllAllowedSet permits all documents (no filtering).
type AllAllowedSet struct{}

// NewAllAllowedSet creates a new pass-through filter.
func NewAllAllowedSet() *AllAllowedSet {
	return &AllAllowedSet{}
}

// IsAllowed always returns true.
func (s *AllAllowedSet) IsAllowed(_ string, _ map[string]interface{}) bool {
	return true
}

// NamespacePolicy for tenant isolation
type NamespacePolicy string

const (
	NamespacePolicyStrict     NamespacePolicy = "strict"
	NamespacePolicyExplicit   NamespacePolicy = "explicit"
	NamespacePolicyPermissive NamespacePolicy = "permissive"
)

// NamespaceGrant for cross-namespace access
type NamespaceGrant struct {
	ID            string   `json:"id"`
	FromNamespace string   `json:"from_namespace"`
	ToNamespace   string   `json:"to_namespace"`
	Operations    []string `json:"operations"`
	ExpiresAt     *int64   `json:"expires_at,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}
