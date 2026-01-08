# ToonDB Go SDK v0.3.4

**Ultra-thin client for ToonDB server.**  
All business logic runs on the server.

## Architecture: Thick Server / Thin Client

```
┌────────────────────────────────────────────────┐
│         Rust Server (toondb-grpc)              │
├────────────────────────────────────────────────┤
│  • All business logic (Graph, Policy, Search)  │
│  • Vector operations (HNSW)                    │
│  • SQL parsing & execution                     │
│  • Collections & Namespaces                    │
│  • Single source of truth                      │
└────────────────────────────────────────────────┘
                       │ gRPC/IPC
                       ▼
            ┌─────────────────────┐
            │     Go SDK          │
            │   (~200 LOC)        │
            ├─────────────────────┤
            │ • Transport layer   │
            │ • Type definitions  │
            │ • Zero logic        │
            └─────────────────────┘
```

### What This SDK Contains

**This SDK is ~1,144 lines of code, consisting of:**
- **Transport Layer** (~850 LOC): gRPC and IPC clients
- **Type Definitions** (~300 LOC): Errors, queries, results
- **Zero business logic**: Everything delegates to server

**This SDK does NOT contain:**
- ❌ No database logic (all server-side)
- ❌ No vector operations (all server-side)
- ❌ No SQL parsing (all server-side)
- ❌ No graph algorithms (all server-side)
- ❌ No policy evaluation (all server-side)

### Why This Design?

**Before (Fat Client - REMOVED):**
```go
// ❌ OLD: Business logic duplicated in every language
import "github.com/toondb/toondb-go"

db := toondb.Open("./data")  // 800+ lines of logic
tx := db.Begin()              // Complex transaction logic
tx.Put([]byte("key"), []byte("value"))
```

**After (Thin Client - CURRENT):**
```go
// ✅ NEW: All logic on server, SDK just sends requests
import "github.com/toondb/toondb-go"

client := toondb.NewGrpcClient("localhost:50051")
client.PutKv("key", []byte("value"))  // → Server handles it
```

**Benefits:**
- 🎯 **Single source of truth**: Fix bugs once in Rust, not 3 times
- 🔧 **3x easier maintenance**: No semantic drift between languages
- 🚀 **Faster development**: Add features once, works everywhere
- 📦 **Smaller SDK size**: 73% code reduction

---

## Installation

```bash
go get github.com/toondb/toondb-go
```

---

## Quick Start

### 1. Start ToonDB Server

```bash
# Start the gRPC server
cd toondb
cargo run -p toondb-grpc --release

# Server listens on localhost:50051
```

### 2. Connect from Go

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/toondb/toondb-go"
)

func main() {
    // Connect to server
    client := toondb.NewGrpcClient("localhost:50051")
    defer client.Close()
    
    // Create a vector collection
    err := client.CreateCollection("documents", &toondb.CollectionOptions{
        Dimension: 384,
        Namespace: "default",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Add documents with embeddings
    documents := []toondb.Document{
        {
            ID:        "doc1",
            Content:   "Machine learning tutorial",
            Embedding: make([]float32, 384), // 384-dimensional vector
            Metadata:  map[string]string{"category": "AI"},
        },
    }
    
    ids, err := client.AddDocuments("documents", documents, "default")
    if err != nil {
        log.Fatal(err)
    }
    
    // Search for similar documents
    queryVector := make([]float32, 384)
    results, err := client.SearchCollection("documents", queryVector, 5, "default", nil)
    if err != nil {
        log.Fatal(err)
    }
    
    for _, result := range results {
        fmt.Printf("Score: %f, Content: %s\n", result.Score, result.Content)
    }
}
```

---

## API Reference

### GrpcClient (gRPC Transport)

**Constructor:**
```go
client := toondb.NewGrpcClient(address string) *GrpcClient
// address: "localhost:50051"

// With TLS
client := toondb.NewGrpcClientWithTLS(address string, certFile string) *GrpcClient
```

**Vector Operations:**
```go
// Create vector index
err := client.CreateIndex(
    name string,
    dimension int,
    metric string,  // "cosine", "euclidean", "dot"
) error

// Insert vectors
err := client.InsertVectors(
    indexName string,
    ids []uint64,
    vectors [][]float32,
) error

// Search vectors
results, err := client.Search(
    indexName string,
    query []float32,
    k int,
) ([]GrpcSearchResult, error)
```

**Collection Operations:**
```go
// Create collection
err := client.CreateCollection(
    name string,
    options *CollectionOptions,
) error

type CollectionOptions struct {
    Dimension int
    Namespace string  // Default: "default"
}

// Add documents
ids, err := client.AddDocuments(
    collectionName string,
    documents []Document,
    namespace string,
) ([]string, error)

// Search collection
results, err := client.SearchCollection(
    collectionName string,
    query []float32,
    k int,
    namespace string,
    filter map[string]string,
) ([]GrpcDocument, error)
```

**Graph Operations:**
```go
// Add graph node
err := client.AddNode(
    nodeID string,
    nodeType string,
    properties map[string]string,
    namespace string,
) error

// Add graph edge
err := client.AddEdge(
    fromID string,
    edgeType string,
    toID string,
    properties map[string]string,
    namespace string,
) error

// Traverse graph
nodes, edges, err := client.Traverse(
    startNode string,
    options *TraverseOptions,
) ([]GrpcGraphNode, []GrpcGraphEdge, error)

type TraverseOptions struct {
    MaxDepth   int      // Default: 3
    EdgeTypes  []string
    Namespace  string
}
```

**Namespace Operations:**
```go
// Create namespace
err := client.CreateNamespace(
    name string,
    metadata map[string]string,
) error

// List namespaces
namespaces, err := client.ListNamespaces() ([]string, error)
```

**Key-Value Operations:**
```go
// Put key-value
err := client.PutKv(
    key string,
    value []byte,
    namespace string,
) error

// Get value
value, err := client.GetKv(
    key string,
    namespace string,
) ([]byte, error)

// Batch operations (atomic)
err := client.BatchPut(
    entries []KvEntry,
) error
```

**Temporal Graph Operations:**
```go
// Add time-bounded edge
err := client.AddTemporalEdge(ctx, &toondb.AddTemporalEdgeRequest{
    Namespace:  "agent_memory",
    FromId:     "door_1",
    EdgeType:   "is_open",
    ToId:       "room_5",
    ValidFrom:  uint64(time.Now().UnixMilli()),
    ValidUntil: uint64(time.Now().Add(time.Hour).UnixMilli()),
})

// Query at specific point in time
resp, err := client.QueryTemporalGraph(ctx, &toondb.QueryTemporalGraphRequest{
    Namespace: "agent_memory",
    NodeId:    "door_1",
    Mode:      toondb.TemporalQueryMode_TEMPORAL_QUERY_MODE_POINT_IN_TIME,
    Timestamp: uint64(time.Now().Add(-2 * time.Minute).UnixMilli()),
})
```

**Format Utilities:**
```go
import "github.com/toondb/toondb-go"

// Parse format from string
wire, err := toondb.ParseWireFormat("json")  // WireFormatJSON

// Convert between formats
caps := toondb.FormatCapabilities{}
ctx := caps.WireToContext(toondb.WireFormatJSON)
// Returns: &ContextFormatJSON

// Check round-trip support
supports := caps.SupportsRoundTrip(toondb.WireFormatToon)
// Returns: true
```

### IPCClient (Unix Socket Transport)

For local inter-process communication:

```go
import "github.com/toondb/toondb-go"

// Connect via Unix socket
client := toondb.NewIPCClient("/tmp/toondb.sock")
defer client.Close()

// Same API as GrpcClient
err := client.Put([]byte("key"), []byte("value"))
value, err := client.Get([]byte("key"))
```

---

## Data Types

### GrpcSearchResult
```go
type GrpcSearchResult struct {
    ID       uint64   // Vector ID
    Distance float32  // Similarity distance
}
```

### GrpcDocument
```go
type GrpcDocument struct {
    ID        string             // Document ID
    Content   string             // Text content
    Embedding []float32          // Vector embedding
    Metadata  map[string]string  // Metadata
}
```

### GrpcGraphNode
```go
type GrpcGraphNode struct {
    ID         string             // Node ID
    NodeType   string             // Node type
    Properties map[string]string  // Properties
}
```

### GrpcGraphEdge
```go
type GrpcGraphEdge struct {
    FromID     string             // Source node
    EdgeType   string             // Edge type
    ToID       string             // Target node
    Properties map[string]string  // Properties
}
```

### TemporalEdge
```go
type TemporalEdge struct {
    FromID     string             // Source node
    EdgeType   string             // Edge type
    ToID       string             // Target node
    ValidFrom  uint64             // Unix timestamp (ms)
    ValidUntil uint64             // Unix timestamp (ms), 0 = no expiry
    Properties map[string]string  // Properties
}
```

### WireFormat
```go
type WireFormat int

const (
    WireFormatToon WireFormat = iota  // 40-66% fewer tokens than JSON
    WireFormatJSON                     // Standard compatibility
    WireFormatColumnar                 // Analytics optimized
)
```

### ContextFormat
```go
type ContextFormat int

const (
    ContextFormatToon ContextFormat = iota  // Token-efficient for LLMs
    ContextFormatJSON                        // Structured data
    ContextFormatMarkdown                    // Human-readable
)
```

---

## Advanced Features

### Temporal Graph Queries

Temporal graphs allow you to query "What did the system know at time T?"

**Use Case: Agent Memory with Time Travel**
```go
import (
    "context"
    "github.com/toondb/toondb-go"
    "time"
)

client := toondb.NewGrpcClient("localhost:50051")

// Record that door was open from 10:00 to 11:00
now := time.Now()
oneHour := time.Hour

err := client.AddTemporalEdge(context.Background(), &toondb.AddTemporalEdgeRequest{
    Namespace:  "agent_memory",
    FromId:     "door_1",
    EdgeType:   "is_open",
    ToId:       "room_5",
    ValidFrom:  uint64(now.UnixMilli()),
    ValidUntil: uint64(now.Add(oneHour).UnixMilli()),
})

// Query: "Was door_1 open 30 minutes ago?"
thirtyMinAgo := now.Add(-30 * time.Minute)
resp, err := client.QueryTemporalGraph(context.Background(), &toondb.QueryTemporalGraphRequest{
    Namespace: "agent_memory",
    NodeId:    "door_1",
    Mode:      toondb.TemporalQueryMode_TEMPORAL_QUERY_MODE_POINT_IN_TIME,
    Timestamp: uint64(thirtyMinAgo.UnixMilli()),
})

fmt.Printf("Door was open: %v\n", len(resp.Edges) > 0)

// Query: "What changed in the last hour?"
resp, err = client.QueryTemporalGraph(context.Background(), &toondb.QueryTemporalGraphRequest{
    Namespace: "agent_memory",
    NodeId:    "door_1",
    Mode:      toondb.TemporalQueryMode_TEMPORAL_QUERY_MODE_RANGE,
    StartTime: uint64(now.Add(-oneHour).UnixMilli()),
    EndTime:   uint64(now.UnixMilli()),
})
```

**Query Modes:**
- `TEMPORAL_QUERY_MODE_POINT_IN_TIME`: Edges valid at specific timestamp
- `TEMPORAL_QUERY_MODE_RANGE`: Edges overlapping a time range
- `TEMPORAL_QUERY_MODE_CURRENT`: Edges valid right now

### Atomic Multi-Operation Writes

Ensure all-or-nothing semantics across multiple operations:

```go
import "github.com/toondb/toondb-go"

client := toondb.NewGrpcClient("localhost:50051")

// All operations succeed or all fail atomically
entries := []toondb.KvEntry{
    {Key: []byte("user:alice:email"), Value: []byte("alice@example.com")},
    {Key: []byte("user:alice:age"), Value: []byte("30")},
    {Key: []byte("user:alice:created"), Value: []byte("2026-01-07")},
}

err := client.BatchPut(entries)

// If server crashes mid-batch, none of the writes persist
```

### Format Conversion for LLM Context

Optimize token usage when sending data to LLMs:

```go
import "github.com/toondb/toondb-go"

// Query results come in WireFormat
queryFormat := toondb.WireFormatToon  // 40-66% fewer tokens than JSON

// Convert to ContextFormat for LLM prompt
caps := toondb.FormatCapabilities{}
ctxFormat := caps.WireToContext(queryFormat)
// Returns: &ContextFormatToon

// TOON format example:
// user:alice|email:alice@example.com,age:30
// vs JSON:
// {"user":"alice","email":"alice@example.com","age":30}

// Check if format supports decode(encode(x)) = x
isLossless := caps.SupportsRoundTrip(toondb.WireFormatToon)
// Returns: true (TOON and JSON are lossless)
```

**Format Benefits:**
- **TOON format**: 40-66% fewer tokens than JSON → Lower LLM API costs
- **Round-trip guarantee**: `decode(encode(x)) = x` for TOON and JSON
- **Columnar format**: Optimized for analytics queries with projections

---

## Error Handling

```go
import "github.com/toondb/toondb-go"

client := toondb.NewGrpcClient("localhost:50051")

err := client.CreateCollection("test", &toondb.CollectionOptions{
    Dimension: 128,
})

if err != nil {
    switch e := err.(type) {
    case *toondb.ConnectionError:
        log.Printf("Cannot connect to server: %v", e)
    case *toondb.ToonDBError:
        log.Printf("ToonDB error: %v", e)
    default:
        log.Printf("Unknown error: %v", err)
    }
}
```

**Error Types:**
- `ToonDBError` - Base error type
- `ConnectionError` - Cannot connect to server
- `TransactionError` - Transaction failed
- `ProtocolError` - Protocol mismatch
- `DatabaseError` - Server-side error

---

## Advanced Usage

### Connection with TLS
```go
client := toondb.NewGrpcClientWithTLS(
    "api.example.com:50051",
    "/path/to/cert.pem",
)
```

### Batch Operations
```go
// Insert multiple vectors at once
ids := make([]uint64, 1000)
for i := range ids {
    ids[i] = uint64(i)
}

vectors := make([][]float32, 1000)
for i := range vectors {
    vectors[i] = make([]float32, 384)
    // ... populate vector
}

err := client.InsertVectors("my_index", ids, vectors)
```

### Filtered Search
```go
// Search with metadata filtering
filter := map[string]string{
    "category": "AI",
    "year":     "2024",
}

results, err := client.SearchCollection(
    "documents",
    queryVector,
    10,
    "default",
    filter,
)
```

### Context with Timeout
```go
import (
    "context"
    "time"
)

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := client.CreateCollectionWithContext(ctx, "test", options)
```

---

## Performance

**Network Overhead:**
- gRPC: ~100-200 μs per request (local)
- IPC: ~50-100 μs per request (Unix socket)

**Batch Operations:**
- Vector insert: 50,000 vectors/sec (batch mode)
- Vector search: 20,000 queries/sec (47 μs/query)

**Recommendation:**
- Use **batch operations** for high throughput
- Use **IPC** for same-machine communication
- Use **gRPC** for distributed systems

---

## Examples

### Basic Vector Search
```go
package main

import (
    "fmt"
    "log"
    
    "github.com/toondb/toondb-go"
)

func main() {
    client := toondb.NewGrpcClient("localhost:50051")
    defer client.Close()
    
    // Create index
    err := client.CreateIndex("embeddings", 384, "cosine")
    if err != nil {
        log.Fatal(err)
    }
    
    // Insert vectors
    ids := []uint64{1, 2, 3}
    vectors := [][]float32{
        {0.1, 0.2 /* ... 384 dims */},
        {0.3, 0.4 /* ... 384 dims */},
        {0.5, 0.6 /* ... 384 dims */},
    }
    
    err = client.InsertVectors("embeddings", ids, vectors)
    if err != nil {
        log.Fatal(err)
    }
    
    // Search
    query := make([]float32, 384)
    query[0] = 0.15
    query[1] = 0.25
    
    results, err := client.Search("embeddings", query, 5)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("Search results:", results)
}
```

### Graph Operations
```go
// Build a knowledge graph
err := client.AddNode("alice", "person", map[string]string{"name": "Alice"}, "default")
err = client.AddNode("bob", "person", map[string]string{"name": "Bob"}, "default")
err = client.AddNode("project", "repository", map[string]string{"name": "ToonDB"}, "default")

err = client.AddEdge("alice", "KNOWS", "bob", nil, "default")
err = client.AddEdge("alice", "CONTRIBUTES_TO", "project", nil, "default")

// Traverse from Alice
nodes, edges, err := client.Traverse("alice", &toondb.TraverseOptions{
    MaxDepth:  2,
    Namespace: "default",
})

fmt.Printf("Connected nodes: %v\n", nodes)
fmt.Printf("Relationships: %v\n", edges)
```

---

## FAQ

**Q: Why remove the embedded Database type?**  
A: To eliminate duplicate business logic. Having SQL parsers, vector indexes, and graph algorithms in every language creates 3x maintenance burden and semantic drift.

**Q: What if I need offline/embedded mode?**  
A: Use the IPC client with a local server process. The server can run on the same machine with Unix socket communication (50 μs latency).

**Q: Is this slower than the old FFI-based approach?**  
A: Network overhead is ~100-200 μs. For batch operations (1000+ vectors), the throughput is identical. The server's Rust implementation is 15x faster than alternatives.

**Q: Can I use this in production?**  
A: Yes. Deploy one or more ToonDB servers and connect clients via gRPC. The server handles all business logic, scaling, and consistency.

---

## Getting Help

- **Documentation**: https://toondb.dev
- **GitHub Issues**: https://github.com/sushanthpy/toondb/issues
- **Examples**: See [example/](example/) directory

---

## Contributing

Interested in contributing? See [CONTRIBUTING.md](CONTRIBUTING.md) for:
- Development environment setup
- Building from source
- Running tests
- Code style guidelines
- Pull request process

---

## License

Apache License 2.0
