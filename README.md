# ToonDB Go SDK v0.3.6

**Dual-mode architecture: Embedded (FFI) + Server (gRPC)**  
Choose the deployment mode that fits your needs.

## 🎯 Two Deployment Modes

### 🆕 Embedded Mode (FFI) - No Server Required

Perfect for CLI tools, edge computing, and single-process apps:

```go
import "github.com/toondb/toondb-go/embedded"

// Direct FFI - works offline, 100x faster
db, err := embedded.Open("./mydb")
defer db.Close()

err = db.Put([]byte("key"), []byte("value"))
value, err := db.Get([]byte("key"))
```

**Benefits:**
- ✅ No server setup required
- ✅ 100x faster (1μs vs 100-200μs per operation)
- ✅ Works completely offline
- ✅ Perfect for CLI, desktop apps, edge computing

### Server Mode (gRPC) - Production Scale

For distributed systems and multi-language teams:

```go
import "github.com/toondb/toondb-go"

// Connect to server
client := toondb.NewGrpcClient("localhost:50051")
defer client.Close()

client.PutKv("key", []byte("value"), "default")
```

**Benefits:**
- ✅ Horizontal scaling
- ✅ Multi-language support
- ✅ Centralized business logic

---

## Installation

```bash
go get github.com/toondb/toondb-go
```

---

## Quick Start

### Embedded Mode (Recommended for Development)

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/toondb/toondb-go/embedded"
)

func main() {
    // Open database
    db, err := embedded.Open("./mydb")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    
    // Put/Get operations
    err = db.Put([]byte("user:1"), []byte("Alice"))
    value, err := db.Get([]byte("user:1"))
    
    fmt.Printf("Value: %s\n", string(value))
}
```

### Server Mode (For Production)

```go
package main

import (
    "log"
    
    "github.com/toondb/toondb-go"
)

func main() {
    client := toondb.NewGrpcClient("localhost:50051")
    defer client.Close()
    
    err := client.PutKv("user:1", []byte("Alice"), "default")
    value, err := client.GetKv("user:1", "default")
}
```

---

## Embedded Mode API Reference

### Database Operations

```go
// Open database
db, err := embedded.Open(path string) (*Database, error)

// Basic KV operations
err = db.Put(key, value []byte) error
value, err := db.Get(key []byte) ([]byte, error)
err = db.Delete(key []byte) error

// Path operations
err = db.PutPath(path string, value []byte) error
value, err := db.GetPath(path string) ([]byte, error)

// Transactions
err = db.WithTransaction(func(txn *Transaction) error {
    txn.Put([]byte("key1"), []byte("value1"))
    txn.Put([]byte("key2"), []byte("value2"))
    return nil
})

// Manual transaction control
txn := db.Begin()
defer txn.Abort()  // Safety net
txn.Put([]byte("key"), []byte("value"))
err = txn.Commit()

// Stats and checkpointing
stats, err := db.Stats()
lsn, err := db.Checkpoint()

// Close database
db.Close()
```

### Transaction Operations

```go
// Within a transaction
txn := db.Begin()
defer txn.Abort()

// KV operations
err = txn.Put(key, value []byte) error
value, err := txn.Get(key []byte) ([]byte, error)
err = txn.Delete(key []byte) error

// Path operations
err = txn.PutPath(path string, value []byte) error
value, err := txn.GetPath(path string) ([]byte, error)

// Scanning
iter := txn.ScanPrefix(prefix []byte)
defer iter.Close()

for {
    key, value, ok := iter.Next()
    if !ok {
        break
    }
    fmt.Printf("%s = %s\n", key, value)
}

// Commit or abort
err = txn.Commit()  // Returns error on SSI conflict
txn.Abort()         // Safe to call multiple times
```

### Index Policy

```go
// Set index policy for a table
db.SetTableIndexPolicy("users", embedded.IndexBalanced)

// Get index policy
policy, err := db.GetTableIndexPolicy("users")

// Available policies:
// - IndexWriteOptimized: O(1) insert, O(N) scan
// - IndexBalanced: O(1) amortized insert, O(log K) scan  
// - IndexScanOptimized: O(log N) insert, O(log N + K) scan
// - IndexAppendOnly: O(1) insert, O(N) scan (time-series)
```

---

## Server Mode API Reference

[Previous server mode documentation remains the same...]

---

## Architecture Comparison

### Embedded Mode (FFI)

```
Go App → CGO → libtoondb_storage.so
         ↓
    Direct FFI (~1μs/op)
```

**Use Cases:**
- CLI tools and utilities
- Desktop applications
- Edge computing (offline)
- Serverless functions (AWS Lambda)
- Single-process applications
- Development and testing

### Server Mode (gRPC)

```
Go App → gRPC → toondb-grpc → libtoondb_storage.so
                     ↓
           Network (~100-200μs/op)
```

**Use Cases:**
- Production deployments
- Multi-language teams
- Distributed systems
- Microservices
- Horizontal scaling

---

## Examples

### Embedded: Transactions with SSI

```go
import "github.com/toondb/toondb-go/embedded"

db, _ := embedded.Open("./mydb")
defer db.Close()

// Automatic retry on SSI conflict
err := db.WithTransaction(func(txn *embedded.Transaction) error {
    // Read current balance
    balance, err := txn.Get([]byte("account:balance"))
    if err != nil {
        return err
    }
    
    // Update balance
    newBalance := parseInt(balance) + 100
    return txn.Put([]byte("account:balance"), []byte(fmt.Sprintf("%d", newBalance)))
})

if err != nil {
    // SSI conflict or other error
    log.Printf("Transaction failed: %v", err)
}
```

### Embedded: Scanning with Prefix

```go
db, _ := embedded.Open("./mydb")
defer db.Close()

// Add some data
db.Put([]byte("user:alice"), []byte("Alice"))
db.Put([]byte("user:bob"), []byte("Bob"))
db.Put([]byte("user:charlie"), []byte("Charlie"))

// Scan all users
txn := db.Begin()
defer txn.Abort()

iter := txn.ScanPrefix([]byte("user:"))
defer iter.Close()

for {
    key, value, ok := iter.Next()
    if !ok {
        break
    }
    fmt.Printf("%s = %s\n", string(key), string(value))
}
```

### Embedded: Path Operations

```go
db, _ := embedded.Open("./mydb")
defer db.Close()

// Hierarchical data storage
db.PutPath("users/alice/profile/name", []byte("Alice Smith"))
db.PutPath("users/alice/profile/email", []byte("alice@example.com"))
db.PutPath("users/alice/settings/theme", []byte("dark"))

// Retrieve
name, _ := db.GetPath("users/alice/profile/name")
fmt.Printf("Name: %s\n", string(name))
```

---

## Performance

### Embedded Mode

- **Single operation**: ~1μs
- **Batch writes**: 500,000 ops/sec
- **Scan throughput**: 1M keys/sec
- **Memory overhead**: ~10MB base + data

### Server Mode

- **Network latency**: 100-200μs (local)
- **Batch operations**: Similar to embedded
- **Recommended for**: Production, multi-client

---

## Testing

```bash
# Test embedded mode
cd embedded
go test -v

# Run example
cd examples/embedded
go run main.go
```

---

## Building with Embedded Mode

The embedded mode uses CGO and requires the ToonDB native library:

```bash
# Set library path (development)
export CGO_LDFLAGS="-L/path/to/toondb/target/release"
export LD_LIBRARY_PATH="/path/to/toondb/target/release"

# Build
go build -o myapp

# Or use go:embed for static linking (advanced)
go build -tags static -o myapp
```

For production, the library should be installed system-wide or bundled with your application.

---

## Feature Comparison

| Feature | Embedded Mode | Server Mode |
|---------|---------------|-------------|
| **Setup** | `embedded.Open()` | Start server + connect |
| **Performance** | 1μs/op | 100-200μs/op |
| **Offline** | ✅ Yes | ❌ No |
| **Multi-process** | ❌ No | ✅ Yes |
| **Deployment** | Single binary | Server + clients |
| **Best For** | CLI, edge, dev | Production, scale |

---

## Migration Guide

### From Server to Embedded

```go
// Before (Server Mode)
client := toondb.NewGrpcClient("localhost:50051")
client.PutKv("key", []byte("value"), "default")
value, _ := client.GetKv("key", "default")

// After (Embedded Mode)
db, _ := embedded.Open("./mydb")
defer db.Close()
db.Put([]byte("key"), []byte("value"))
value, _ := db.Get([]byte("key"))
```

### From Embedded to Server

Just start the toondb-grpc server and change the connection method. Your data is compatible!

---

## FAQ

**Q: Which mode should I use?**  
A: Use embedded for CLI tools, dev, and single-process apps. Use server for production and multi-client scenarios.

**Q: Can I switch between modes?**  
A: Yes! The data format is identical. Just point the server at your embedded database directory.

**Q: Is embedded mode slower than server mode?**  
A: No! Embedded is 100x faster (1μs vs 100-200μs) because there's no network overhead.

**Q: Does embedded mode support transactions?**  
A: Yes! Full ACID transactions with SSI (Serializable Snapshot Isolation).

---

## Getting Help

- **Documentation**: https://toondb.dev
- **GitHub Issues**: https://github.com/toondb/toondb/issues
- **Examples**: See [examples/embedded](examples/embedded/)

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md)

---

## License

Apache License 2.0
