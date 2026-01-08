// Package toondb provides Go SDK for ToonDB v0.3.4
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
//   - Thin client connecting to toondb-grpc server
//   - Best for: Production, multi-language, scalability
//
// Example (Server Mode - gRPC):
//
//	import "github.com/toondb/toondb-go"
//
//	// Connect to server
//	client, err := toondb.GrpcConnect("localhost:50051")
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
//	import "github.com/toondb/toondb-go"
//
//	// Connect via Unix socket
//	client, err := toondb.Connect("/path/to/db/toondb.sock")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Use the client
//	value, err := client.Get([]byte("key"))
package toondb

// Version is the current SDK version.
const Version = "0.3.4"
