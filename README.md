# ToonDB Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/toondb/toondb-go.svg)](https://pkg.go.dev/github.com/toondb/toondb-go)
[![CI](https://github.com/toondb/toondb-go/actions/workflows/ci.yml/badge.svg)](https://github.com/toondb/toondb-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/toondb/toondb-go)](https://goreportcard.com/report/github.com/toondb/toondb-go)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

The official Go client SDK for **ToonDB** — a high-performance embedded document database with HNSW vector search and built-in multi-tenancy support.

## Features

- ✅ **Key-Value Store** — Simple `Get`/`Put`/`Delete` operations
- ✅ **Path-Native API** — Hierarchical keys like `users/alice/email`
- ✅ **Prefix Scanning** — Fast `Scan()` for multi-tenant data isolation
- ✅ **Embedded Mode** — Automatic server lifecycle management
- ✅ **Transactions** — ACID-compliant with automatic commit/abort
- ✅ **Query Builder** — Fluent API for complex queries (returns TOON format)
- ✅ **Vector Search** — HNSW approximate nearest neighbor search
- ✅ **Type-Safe** — Full Go type safety
- ✅ **Zero CGO** — Pure Go IPC client (no C dependencies)

## Installation

```bash
go get github.com/toondb/toondb-go@latest
```

**Requirements:**
- Go 1.21+
- ToonDB server binary (automatically managed in embedded mode)

**Batteries Included:**
- ✅ Pre-built binaries bundled for Linux x86_64, macOS ARM64, and Windows x64
- ✅ No manual binary installation required for released versions
- ✅ Development builds fall back to `TOONDB_SERVER_PATH` or system PATH

## What's New in v0.3.3

### 🕸️ Graph Overlay for Agent Memory
Build lightweight graph structures on top of ToonDB's KV storage for agent memory:

```go
import toondb "github.com/toondb/toondb-go"

db, _ := toondb.Open("./agent_db")
graph := toondb.NewGraphOverlay(db, "agent_memory")

// Add nodes (entities, concepts, events)
graph.AddNode("user_alice", "person", map[string]interface{}{
    "name": "Alice",
    "role": "developer",
})
graph.AddNode("conv_123", "conversation", map[string]interface{}{
    "topic": "ToonDB features",
})
graph.AddNode("action_456", "action", map[string]interface{}{
    "type":   "code_commit",
    "status": "success",
})

// Add edges (relationships, causality, references)
graph.AddEdge("user_alice", "started", "conv_123", map[string]interface{}{
    "timestamp": "2026-01-05",
})
graph.AddEdge("conv_123", "triggered", "action_456", map[string]interface{}{
    "reason": "user request",
})

// Retrieve nodes and edges
node, _ := graph.GetNode("user_alice")
edges, _ := graph.GetEdges("user_alice", "started")

// Graph traversal
visited, _ := graph.BFS("user_alice", 3, nil, nil)  // BFS from Alice
path, _ := graph.ShortestPath("user_alice", "action_456", 10, nil)  // Find connection

// Get neighbors
neighbors, _ := graph.GetNeighbors("conv_123", "both", "")

// Extract subgraph
subgraph, _ := graph.GetSubgraph([]string{"user_alice", "conv_123", "action_456"})
```

**Use Cases:**
- Agent conversation history with causal chains
- Entity relationship tracking across sessions
- Action dependency graphs for planning
- Knowledge graph construction

### 🛡️ Policy & Safety Hooks
Enforce safety policies on agent operations with pre/post triggers:

```go
db, _ := toondb.Open("./agent_data")
policy := toondb.NewPolicyEngine(db)

// Block writes to system keys from agents
policy.BeforeWrite("system/*", func(ctx *toondb.PolicyContext) toondb.PolicyAction {
    if ctx.AgentID != "" {
        return toondb.PolicyDeny
    }
    return toondb.PolicyAllow
})

// Redact sensitive data on read
policy.AfterRead("users/*/email", func(ctx *toondb.PolicyContext) toondb.PolicyAction {
    if ctx.Get("redact_pii") == "true" {
        ctx.ModifiedValue = []byte("[REDACTED]")
        return toondb.PolicyModify
    }
    return toondb.PolicyAllow
})

// Rate limit writes per agent
policy.AddRateLimit("write", 100, "agent_id")

// Enable audit logging
policy.EnableAudit(10000)

// Use policy-wrapped operations
err := policy.Put([]byte("users/alice"), []byte("data"), map[string]string{
    "agent_id": "agent_001",
})
```

### 🔀 Multi-Agent Tool Routing
Route tool calls to specialized agents with automatic failover:

```go
db, _ := toondb.Open("./agent_data")
dispatcher := toondb.NewToolDispatcher(db)

// Register local agent with handler
dispatcher.RegisterLocalAgent("code_agent",
    []toondb.ToolCategory{toondb.CategoryCode, toondb.CategoryGit},
    func(tool string, args map[string]any) (any, error) {
        return map[string]any{"result": fmt.Sprintf("Processed %s", tool)}, nil
    }, 100)

// Register remote agent
dispatcher.RegisterRemoteAgent("search_agent",
    []toondb.ToolCategory{toondb.CategorySearch},
    "http://localhost:8001/invoke", 100)

// Register tools
dispatcher.Router().RegisterTool(toondb.Tool{
    Name:        "search_code",
    Description: "Search codebase",
    Category:    toondb.CategoryCode,
})

// Invoke with automatic routing
result := dispatcher.Invoke("search_code", map[string]any{"query": "auth"},
    toondb.WithSessionID("sess_001"))
fmt.Printf("Routed to: %s, Success: %v\n", result.AgentID, result.Success)
```

### 🎯 Namespace Isolation
Logical database namespaces for true multi-tenancy without key prefixing:

```go
// Create isolated namespaces
userDB, _ := db.Namespace("users")
ordersDB, _ := db.Namespace("orders")

// Keys don't collide across namespaces
userDB.Put([]byte("123"), []byte(`{"name":"Alice"}`))
ordersDB.Put([]byte("123"), []byte(`{"total":500}`))  // Different "123"!

// Each namespace has isolated collections
userDB.CreateCollection("profiles", &toondb.CollectionConfig{
    VectorDim: 384,
    IndexType: toondb.HNSW,
})
```

### 🔍 Hybrid Search
Combine dense vectors (HNSW) with sparse BM25 text search:

```go
// Create collection with hybrid search
config := &toondb.CollectionConfig{
    VectorDim:   384,
    IndexType:   toondb.HNSW,
    EnableBM25:  true,  // Enable text search
}
collection, _ := db.CreateCollection("documents", config)

// Insert documents with text and vectors
doc := &toondb.Document{
    ID:     "doc1",
    Text:   "Machine learning models for NLP tasks",
    Vector: []float32{0.1, 0.2, ...},  // 384-dim embedding
}
collection.Insert(doc)

// Hybrid search (vector + text)
results, _ := collection.HybridSearch(&toondb.HybridQuery{
    Vector:    queryEmbedding,
    Text:      "NLP transformer",
    K:         10,
    Alpha:     0.7,  // 70% vector, 30% BM25
    RRFusion:  true, // Reciprocal Rank Fusion
})
```

### 📄 Multi-Vector Documents
Store multiple embeddings per document (e.g., title + content):

```go
// Insert document with multiple vectors
multiDoc := &toondb.MultiVectorDocument{
    ID:   "article1",
    Text: "Deep Learning: A Survey",
    Vectors: map[string][]float32{
        "title":    titleEmbedding,    // 384-dim
        "abstract": abstractEmbedding, // 384-dim
        "content":  contentEmbedding,  // 384-dim
    },
}
collection.InsertMultiVector(multiDoc)

// Search with aggregation strategy
results, _ := collection.MultiVectorSearch(&toondb.MultiVectorQuery{
    QueryVectors: map[string][]float32{
        "title":   queryTitleEmbedding,
        "content": queryContentEmbedding,
    },
    K:           10,
    Aggregation: toondb.MaxPooling,  // or MeanPooling, WeightedSum
})
```

### 🧩 Context-Aware Queries
Optimize retrieval for LLM context windows:

```go
// Query with token budget
results, _ := collection.ContextQuery(&toondb.ContextConfig{
    Vector:         queryEmbedding,
    MaxTokens:      4000,
    TargetProvider: "gpt-4",  // Auto token counting
    DedupStrategy:  toondb.Semantic,  // Avoid redundant results
})

// Results fit within 4000 tokens, deduplicated for relevance
```

### 🕸️ Graph Overlay
Lightweight graph layer for agent memory relationships:

```go
db, _ := toondb.Open("./agent_memory")
graph := toondb.NewGraphOverlay(db, "agent_001")

// Create nodes
graph.AddNode("user_1", "User", map[string]interface{}{"name": "Alice"})
graph.AddNode("conv_1", "Conversation", map[string]interface{}{"title": "Planning"})
graph.AddNode("msg_1", "Message", map[string]interface{}{"content": "Let's start"})

// Create edges
graph.AddEdge("user_1", "STARTED", "conv_1", nil)
graph.AddEdge("conv_1", "CONTAINS", "msg_1", nil)
graph.AddEdge("user_1", "SENT", "msg_1", nil)

// Traverse graph
reachable, _ := graph.BFS("user_1", 2, nil, nil)
// ["user_1", "conv_1", "msg_1"]

// Find shortest path
path, _ := graph.ShortestPath("user_1", "msg_1", 10, nil)
// ["user_1", "conv_1", "msg_1"]

// Get neighbors
neighbors, _ := graph.GetNeighbors("user_1", nil, toondb.EdgeOutgoing)
for _, n := range neighbors {
    fmt.Printf("%s via %s\n", n.NodeID, n.Edge.EdgeType)
}
```

### 🔍 Token-Aware Context Query Builder
Build context for LLM prompts with token budgeting:

```go
query := toondb.NewContextQuery(db, "documents").
    AddVectorQuery(embedding, 0.7).
    AddKeywordQuery("machine learning", 0.3).
    WithTokenBudget(4000).
    WithMinRelevance(0.5).
    WithDeduplication(toondb.DeduplicationSemantic, 0.9)

result, _ := query.Execute()

// Format for LLM prompt
context := result.AsText("\n\n---\n\n")
prompt := context + "\n\nQuestion: " + userQuestion

// Metrics
fmt.Printf("Tokens used: %d/%d\n", result.TotalTokens, result.BudgetTokens)
fmt.Printf("Chunks: %d, Dropped: %d\n", len(result.Chunks), result.DroppedCount)
```

## CLI Tools

Go-native wrappers for the ToonDB tools are available in the `cmd/` directory.

### Installation

```bash
# Install the wrappers to your $GOPATH/bin
go install github.com/toondb/toondb-go/cmd/toondb-server@latest
go install github.com/toondb/toondb-go/cmd/toondb-bulk@latest
go install github.com/toondb/toondb-go/cmd/toondb-grpc-server@latest
```

> **Note:** These wrappers require the native binary to be in your PATH or `TOONDB_SERVER_PATH` to be set.

### Usage

```bash
# Start server
toondb-server --db ./my_db

# Bulk operations
toondb-bulk build-index --input vec.npy --output index.hnsw
```

## Quick Start

### Embedded Mode (Recommended)

The SDK automatically starts and stops the server:

```go
package main

import (
    "fmt"
    "log"
    toondb "github.com/toondb/toondb-go"
)

func main() {
    // Open database with embedded server (default)
    db, err := toondb.Open("./my_database")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close() // Automatically stops server

    // Use database
    err = db.Put([]byte("key"), []byte("value"))
    if err != nil {
        log.Fatal(err)
    }

    value, err := db.Get([]byte("key"))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Value: %s\n", value)
}
```

### External Server Mode

If you want to manage the server yourself:

```go
config := &toondb.Config{
    Path:     "./my_database",
    Embedded: false, // Disable embedded mode
}

db, err := toondb.OpenWithConfig(config)
if err != nil {
    log.Fatal(err)
}
defer db.Close()
```

**Start the server manually:**
```bash
# Download or build toondb-server from the main repo, then:
./toondb-server --db ./my_database

# Output: [IpcServer] Listening on "./my_database/toondb.sock"
```

## Core API

### Basic Key-Value

```go
// Put
err = db.Put([]byte("key"), []byte("value"))

// Get
value, err := db.Get([]byte("key"))
if value == nil {
    fmt.Println("Key not found")
}

// Delete
err = db.Delete([]byte("key"))

// String helpers
db.PutString("greeting", "Hello World")
msg, _ := db.GetString("greeting")
```

**Output:**
```
Put: key → value
Get: value
Delete: key
String: Hello World
```

### Path Operations

```go
// Store hierarchical data
db.PutPath("users/alice/email", []byte("alice@example.com"))
db.PutPath("users/alice/age", []byte("30"))
db.PutPath("users/bob/email", []byte("bob@example.com"))

// Retrieve by path
email, _ := db.GetPath("users/alice/email")
fmt.Printf("Alice's email: %s\n", email)
```

**Output:**
```
Alice's email: alice@example.com
```

### Prefix Scanning ⭐ New in 0.2.6

The most efficient way to iterate keys with a common prefix:

```go
// Insert multi-tenant data
db.Put([]byte("tenants/acme/users/1"), []byte(`{"name":"Alice"}`))
db.Put([]byte("tenants/acme/users/2"), []byte(`{"name":"Bob"}`))
db.Put([]byte("tenants/acme/orders/1"), []byte(`{"total":100}`))
db.Put([]byte("tenants/globex/users/1"), []byte(`{"name":"Charlie"}`))

// Scan only ACME Corp's data
results, err := db.Scan("tenants/acme/")
fmt.Printf("ACME Corp has %d items:\n", len(results))
for _, kv := range results {
    fmt.Printf("  %s: %s\n", kv.Key, kv.Value)
}
```

**Output:**
```
ACME Corp has 3 items:
  tenants/acme/orders/1: {"total":100}
  tenants/acme/users/1: {"name":"Alice"}
  tenants/acme/users/2: {"name":"Bob"}
```

**Why use Scan():**
- **Fast**: Binary protocol, O(|prefix|) performance
- **Isolated**: Perfect for multi-tenant apps
- **Efficient**: No deserialization overhead

## Transactions

```go
// Automatic commit/abort
err := db.WithTransaction(func(txn *toondb.Transaction) error {
    txn.Put([]byte("account:1:balance"), []byte("1000"))
    txn.Put([]byte("account:2:balance"), []byte("500"))
    return nil // Commits on success
})
```

**Output:**
```
Transaction started
✅ Committed 2 writes
```

**Manual control:**
```go
txn, _ := db.BeginTransaction()
defer txn.Abort() // Cleanup if commit fails

txn.Put([]byte("key1"), []byte("value1"))
txn.Put([]byte("key2"), []byte("value2"))

err := txn.Commit()
```

## Query Builder

Returns results in **TOON format** (token-optimized for LLMs):

```go
// Insert structured data
db.Put([]byte("products/laptop"), []byte(`{"name":"Laptop","price":999}`))
db.Put([]byte("products/mouse"), []byte(`{"name":"Mouse","price":25}`))

// Query with column selection
results, err := db.Query("products/").
    Select("name", "price").
    Limit(10).
    Execute()

for _, kv := range results {
    fmt.Printf("%s: %s\n", kv.Key, kv.Value)
}
```

**Output (TOON Format):**
```
products/laptop: result[1]{name,price}:Laptop,999
products/mouse: result[1]{name,price}:Mouse,25
```

**Other query methods:**
```go
first, _ := db.Query("products/").First()    // Get first result
count, _ := db.Query("products/").Count()    // Count results
exists, _ := db.Query("products/").Exists()  // Check existence
```

## SQL-Like Operations

While Go SDK focuses on key-value operations, you can use Query for SQL-like operations:

```go
// INSERT-like: Store structured data
db.Put([]byte("products/001"), []byte(`{"id":1,"name":"Laptop","price":999}`))
db.Put([]byte("products/002"), []byte(`{"id":2,"name":"Mouse","price":25}`))

// SELECT-like: Query with column selection
results, _ := db.Query("products/").
    Select("name", "price"). // SELECT name, price
    Limit(10).               // LIMIT 10
    Execute()
```

**Output:**
```
SELECT name, price FROM products LIMIT 10:
products/001: result[1]{name,price}:Laptop,999
products/002: result[1]{name,price}:Mouse,25
```

> **Note:** For full SQL (CREATE TABLE, INSERT, SELECT, JOIN), use `db.Execute(sql)` when the server is running. The Query builder above provides SQL-like operations for KV data.

## Vector Search

```go
// Create HNSW index
config := &toondb.VectorIndexConfig{
    Dimension:      384,
    Metric:         toondb.Cosine,
    M:              16,
    EfConstruction: 100,
}
index := toondb.NewVectorIndex("./vectors", config)

// Build from embeddings
vectors := [][]float32{
    {0.1, 0.2, 0.3, /* ... 384 dims */},
    {0.4, 0.5, 0.6, /* ... 384 dims */},
}
labels := []string{"doc1", "doc2"}
index.BulkBuild(vectors, labels)

// Search
query := []float32{0.15, 0.25, 0.35, /* ... */}
results, _ := index.Query(query, 10, 50) // k=10, ef_search=50

for i, r := range results {
    fmt.Printf("%d. %s (distance: %.4f)\n", i+1, r.Label, r.Distance)
}
```

**Output:**
```
1. doc1 (distance: 0.0234)
2. doc2 (distance: 0.1567)
```

## Complete Example: Multi-Tenant App

```go
package main

import (
    "fmt"
    "log"
    toondb "github.com/toondb/toondb/toondb-go"
)

func main() {
    db, _ := toondb.Open("./multi_tenant_db")
    defer db.Close()

    // Insert data for two tenants
    db.Put([]byte("tenants/acme/users/alice"), []byte(`{"role":"admin"}`))
    db.Put([]byte("tenants/acme/users/bob"), []byte(`{"role":"user"}`))
    db.Put([]byte("tenants/globex/users/charlie"), []byte(`{"role":"admin"}`))

    // Scan ACME Corp data only (tenant isolation)
    acmeData, _ := db.Scan("tenants/acme/")
    fmt.Printf("ACME Corp: %d users\n", len(acmeData))
    for _, kv := range acmeData {
        fmt.Printf("  %s: %s\n", kv.Key, kv.Value)
    }

    // Scan Globex Corp data
    globexData, _ := db.Scan("tenants/globex/")
    fmt.Printf("\nGlobex Corp: %d users\n", len(globexData))
    for _, kv := range globexData {
        fmt.Printf("  %s: %s\n", kv.Key, kv.Value)
    }
}
```

**Output:**
```
ACME Corp: 2 users
  tenants/acme/users/alice: {"role":"admin"}
  tenants/acme/users/bob: {"role":"user"}

Globex Corp: 1 users
  tenants/globex/users/charlie: {"role":"admin"}
```

## Error Handling

```go
import "errors"

value, err := db.Get(key)
if err != nil {
    if errors.Is(err, toondb.ErrClosed) {
        log.Println("Database closed")
    }
    if errors.Is(err, toondb.ErrConnectionFailed) {
        log.Println("Server not running")
    }
    log.Fatal(err)
}

if value == nil {
    log.Println("Key not found (not an error)")
}
```

## Best Practices

✅ **Always close:** `defer db.Close()`
✅ **Use transactions:** For atomic multi-key operations
✅ **Check nil:** `value == nil` means key doesn't exist
✅ **Use Scan():** For prefix iteration (not Query)
✅ **Multi-tenant:** Prefix keys with tenant ID

## Configuration

```go
config := &toondb.Config{
    Path:              "./my_database",
    CreateIfMissing:   true,
    WALEnabled:        true,
    SyncMode:          "normal", // "full", "normal", "off"
    MemtableSizeBytes: 64 * 1024 * 1024,
}
db, err := toondb.OpenWithConfig(config)
```

## Testing

```bash
go test -v ./...
go test -bench=. -benchmem ./...
```

## Privacy & Analytics

ToonDB collects **anonymous usage analytics** to help improve the SDK. We collect:
- ✅ SDK version
- ✅ Error types (connection, query, permission, timeout)

We **never** collect:
- ❌ User data
- ❌ Database paths
- ❌ Keys or values
- ❌ Queries

**Opt-out anytime:**
```bash
export TOONDB_DISABLE_ANALYTICS=true
```

## License

Apache License 2.0

## Links

- [Documentation](https://docs.toondb.dev/)
- [Python SDK](../toondb-python-sdk)
- [JavaScript SDK](../toondb-js)
- [GitHub](https://github.com/toondb/toondb)
