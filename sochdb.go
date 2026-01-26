// Package sochdb provides Go SDK for SochDB v0.4.4
//
// Dual-mode architecture: Embedded (FFI) + Server (gRPC/IPC)
//
// Architecture: Flexible Deployment
// ==================================
// This SDK supports BOTH modes:
//
// 1. Embedded Mode (FFI) - For single-process apps:
//   - Direct FFI bindings to Rust libraries (via CGO)
//   - No server required - just import and run
//   - Best for: Local development, simple apps, edge deployments
//
// 2. Server Mode (gRPC) - For distributed systems:
//   - Thin client connecting to sochdb-grpc server
//   - Best for: Production, multi-language, scalability
//
// Example (Server Mode - gRPC):
//
//	import "github.com/sochdb/sochdb-go"
//
//	// Connect to server
//	client, err := sochdb.GrpcConnect("localhost:50051")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Use the client
//	err = client.PutKv(context.Background(), "namespace", "key", []byte("value"))
//
// Example (Embedded Mode - IPC):
//
//	import "github.com/sochdb/sochdb-go"
//
//	// Connect via Unix socket
//	client, err := sochdb.Connect("/path/to/db/sochdb.sock")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Use the client
//	value, err := client.Get([]byte("key"))
//
// Example (Namespace & Collections - v0.4.1):
//
//	import "github.com/sochdb/sochdb-go"
//	import "github.com/sochdb/sochdb-go/embedded"
//
//	// Open embedded database
//	db, err := embedded.Open("./mydb")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	// Create namespace
//	ns, err := db.CreateNamespace("tenant_123")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create collection
//	collection, err := ns.CreateCollection(sochdb.CollectionConfig{
//	    Name:      "documents",
//	    Dimension: 384,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Insert vectors
//	err = collection.Insert([]float32{1.0, 2.0, 3.0}, map[string]interface{}{"source": "web"}, "")
//
// Example (Priority Queue - v0.4.1):
//
//	import "github.com/sochdb/sochdb-go"
//	import "github.com/sochdb/sochdb-go/embedded"
//
//	// Open database
//	db, err := embedded.Open("./queue_db")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	// Create queue
//	queue := sochdb.NewPriorityQueue(db, "tasks", nil)
//
//	// Enqueue task
//	taskID, err := queue.Enqueue(1, []byte("high priority task"), nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Dequeue and process
//	task, err := queue.Dequeue("worker-1")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if task != nil {
//	    // Process task...
//	    err = queue.Ack(task.TaskID)
//	}
package sochdb

// Version is the current SDK version.
const Version = "0.4.1"
