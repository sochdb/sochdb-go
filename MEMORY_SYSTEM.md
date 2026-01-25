# Memory System Architecture

**Version:** 0.4.2  
**Status:** Production Ready  
**Author:** SochDB Team

## Overview

The Memory System provides a production-grade, LLM-native memory layer for AI applications. It implements a complete knowledge lifecycle: extraction → consolidation → retrieval.

### Design Principles

1. **Event-Sourced Architecture**: Immutable assertion log with derived materialized views
2. **Multi-Source Consolidation**: Merge facts from multiple sources with confidence weighting
3. **Hybrid Retrieval**: Lexical (BM25) + Semantic (cosine) search with Reciprocal Rank Fusion
4. **Type Safety**: Strongly-typed entities, relations, and assertions with schema validation
5. **Namespace Isolation**: First-class support for multi-tenancy

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Memory System                           │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌─────────────────┐  ┌──────────────────┐  ┌────────────┐ │
│  │   Extraction    │  │  Consolidation   │  │  Retrieval │ │
│  │    Pipeline     │  │     Engine       │  │   Engine   │ │
│  └────────┬────────┘  └────────┬─────────┘  └─────┬──────┘ │
│           │                    │                    │        │
│           ▼                    ▼                    ▼        │
│  ┌──────────────────────────────────────────────────────┐   │
│  │           Embedded Database (Badger)                  │   │
│  │  • Entities      • Relations      • Assertions        │   │
│  │  • Raw Events    • Canonical Facts • Documents        │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

## Components

### 1. Extraction Pipeline

**Purpose**: Compile unstructured LLM outputs into typed, validated knowledge structures.

**Key Features**:
- Schema-based validation for entity and relation types
- Deterministic ID generation (SHA-256 hashing)
- Provenance tracking for all extracted facts
- Confidence score preservation

**API**:
```go
pipeline := sochdb.NewExtractionPipeline(db, namespace, schema)
result, _ := pipeline.Extract(text, llmExtractor)
pipeline.Commit(result)

entities, _ := pipeline.GetEntities()
relations, _ := pipeline.GetRelations()
```

**Storage Schema**:
```
memory:{namespace}:entity:{id}    → Entity JSON
memory:{namespace}:relation:{id}  → Relation JSON
memory:{namespace}:assertion:{id} → Assertion JSON
```

### 2. Consolidation Engine

**Purpose**: Merge multi-source assertions into canonical facts using event sourcing.

**Architecture**:
- **Event Log**: Immutable `RawAssertion` append-only log
- **Contradiction Tracking**: Explicit superseding relationships
- **Materialized View**: `CanonicalFact` computed from event log

**Consolidation Algorithm**:
1. Group assertions by subject (fact content hash)
2. Sort by confidence (desc) and timestamp (desc)
3. Filter out contradicted assertions (with temporal validity)
4. Merge remaining assertions:
   - Weighted confidence average (decreasing weights)
   - Union of all sources
   - Earliest timestamp as `valid_from`

**API**:
```go
consolidator := sochdb.NewConsolidator(db, namespace, config)

// Add immutable assertions
id1, _ := consolidator.Add(assertion1)
id2, _ := consolidator.Add(assertion2)

// Handle contradictions
consolidator.AddWithContradiction(newAssertion, []string{id1})

// Recompute canonical view
updated, _ := consolidator.Consolidate()

// Query results
facts, _ := consolidator.GetCanonicalFacts()
provenance, _ := consolidator.Explain(factID)
```

**Storage Schema**:
```
consolidation:{namespace}:assertion:{id}           → RawAssertion JSON
consolidation:{namespace}:contradiction:{old}:{new} → Contradiction metadata
consolidation:{namespace}:canonical:{id}           → CanonicalFact JSON
```

### 3. Hybrid Retrieval Engine

**Purpose**: Combine lexical and semantic search with Reciprocal Rank Fusion.

**Components**:

#### BM25 Scorer (Lexical)
- **Algorithm**: Okapi BM25 with parameters k1=1.5, b=0.75
- **Features**:
  - Term frequency normalization
  - Document length normalization
  - IDF weighting
- **Time Complexity**: O(q × d) where q=query terms, d=documents

#### Semantic Scorer
- **Algorithm**: Cosine similarity on term frequency vectors
- **Note**: Production deployments should integrate vector embeddings
- **Time Complexity**: O(v²) where v=vocabulary size

#### Reciprocal Rank Fusion (RRF)
- **Formula**: `score = Σ(weight_i / (k + rank_i))`
- **Default k**: 60 (balances top and lower ranks)
- **Weights**: Configurable lexical/semantic blend (default 0.3/0.7)

**Pre-filtering**:
- Applied before scoring to reduce search space
- Supports: ID whitelist, namespace prefix, custom predicates

**API**:
```go
retriever := sochdb.NewHybridRetriever(db, namespace, config)

// Index documents
retriever.IndexDocuments(documents)

// Search with pre-filtering
allowed := &sochdb.FilterAllowedSet{
    FilterFn: func(id string, doc map[string]interface{}) bool {
        return doc["category"] == "important"
    },
}

results, _ := retriever.Retrieve(query, allowed)

// Debug scoring
explanation := retriever.Explain(query, docID)
// → {lexical_score, semantic_score, combined_score, weights}
```

**Storage Schema**:
```
retrieval:{namespace}:doc:{id} → Document JSON
```

## Configuration

### ExtractionSchema
```go
schema := &sochdb.ExtractionSchema{
    EntityTypes:   []string{"Person", "Company", "Product"},
    RelationTypes: []string{"works_at", "founded", "acquired"},
    MinConfidence: 0.7,  // Reject low-confidence extractions
}
```

### ConsolidationConfig
```go
config := &sochdb.ConsolidationConfig{
    SimilarityThreshold: 0.85,      // Fact grouping threshold
    UseTemporalUpdates:  true,      // Enable time-based superseding
    MaxConflictAge:      86400,     // 24 hours conflict validity
}
```

### RetrievalConfig
```go
config := &sochdb.RetrievalConfig{
    Limit:          10,    // Max results
    LexicalWeight:  0.3,   // BM25 weight
    SemanticWeight: 0.7,   // Vector weight
    RRFConstant:    60,    // RRF k parameter
}
```

## Data Flow

### Extraction Flow
```
Text → LLM → Raw JSON → Validation → Typed Structs → Storage
                ↓
         Schema Check
         Confidence Filter
         ID Generation
```

### Consolidation Flow
```
RawAssertion → Event Log → Grouping → Filtering → Merging → CanonicalFact
                             ↓           ↓          ↓
                        By Subject  Contradictions Confidence
```

### Retrieval Flow
```
Query → Tokenization → BM25 Scoring → RRF Fusion → Ranking
        ↓              Cosine Scoring ↗
    Pre-filter
```

## Performance Characteristics

| Operation | Time Complexity | Space Complexity |
|-----------|----------------|------------------|
| Extract Entity | O(1) | O(e) per entity |
| Add Assertion | O(1) | O(a) per assertion |
| Consolidate | O(a log a + g×a) | O(a) |
| Index Documents | O(d × t) | O(d × t) |
| Retrieve (BM25) | O(q × d) | O(1) |
| RRF Fusion | O(r log r) | O(r) |

Where: a=assertions, e=entity size, g=groups, d=documents, t=terms, q=query terms, r=results

## Best Practices

### 1. Schema Design
- Keep entity types coarse-grained (5-20 types)
- Use properties for fine-grained attributes
- Validate schema against actual LLM outputs

### 2. Consolidation
- Run consolidation periodically (not per assertion)
- Use temporal updates for time-sensitive facts
- Set appropriate `MaxConflictAge` for domain

### 3. Retrieval
- Tune lexical/semantic weights for your corpus
- Use pre-filtering aggressively to reduce search space
- Cache retrieval results when possible

### 4. Namespaces
- One namespace per tenant for isolation
- Use descriptive namespace names
- Clean up old namespaces explicitly

## Error Handling

All public APIs return `(result, error)` tuples. Common errors:

- **Validation Errors**: Schema mismatch, invalid types
- **Storage Errors**: Database I/O failures
- **Not Found**: Missing entities, documents
- **Conflict**: Contradicting assertions

Example:
```go
if id, err := consolidator.Add(assertion); err != nil {
    log.Printf("Failed to add assertion: %v", err)
    return fmt.Errorf("consolidation failed: %w", err)
}
```

## Testing

Run the comprehensive example:
```bash
cd sochdb-go
go run examples/memory_system.go
```

Expected output: 6 scenarios demonstrating extraction, consolidation, contradiction handling, hybrid retrieval, pre-filtering, and explainability.

## Future Enhancements

- [ ] Vector embedding integration (replace term-frequency cosine)
- [ ] GraphQL query layer
- [ ] Temporal queries (point-in-time retrieval)
- [ ] Distributed consolidation (multi-node)
- [ ] Compression for large assertion logs

## References

- [BM25 Algorithm](https://en.wikipedia.org/wiki/Okapi_BM25)
- [Reciprocal Rank Fusion](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf)
- [Event Sourcing](https://martinfowler.com/eaaDev/EventSourcing.html)
