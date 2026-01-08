package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/toondb/toondb-go"
)

func main() {
	fmt.Println("ToonDB Go SDK v0.3.4 Example")
	fmt.Println("================================")

	// Example 1: gRPC Client (Server Mode)
	fmt.Println("\n1. Server Mode (gRPC):")
	grpcExample()

	// Example 2: IPC Client (Embedded Mode)
	fmt.Println("\n2. Embedded Mode (IPC):")
	ipcExample()

	// Example 3: Format Utilities
	fmt.Println("\n3. Format Utilities:")
	formatExample()
}

func grpcExample() {
	// Connect to ToonDB gRPC server
	client, err := toondb.GrpcConnect("localhost:50051")
	if err != nil {
		log.Printf("Failed to connect: %v (make sure toondb-grpc server is running)", err)
		return
	}
	defer client.Close()

	ctx := context.Background()

	// Put key-value
	err = client.PutKv(ctx, "default", "user:1", []byte(`{"name": "Alice", "age": 30}`))
	if err != nil {
		log.Printf("PutKv failed: %v", err)
		return
	}
	fmt.Println("✅ Put: user:1 -> {name: Alice, age: 30}")

	// Get key-value
	value, err := client.GetKv(ctx, "default", "user:1")
	if err != nil {
		log.Printf("GetKv failed: %v", err)
		return
	}
	fmt.Printf("✅ Get: user:1 -> %s\n", string(value))

	// Add graph edge
	edge := toondb.GrpcGraphEdge{
		FromID:   "alice",
		EdgeType: "follows",
		ToID:     "bob",
		Properties: map[string]string{
			"since": time.Now().Format(time.RFC3339),
		},
	}
	err = client.AddEdge(ctx, "default", edge)
	if err != nil {
		log.Printf("AddEdge failed: %v", err)
		return
	}
	fmt.Println("✅ Added edge: alice -[follows]-> bob")

	// Query graph
	results, err := client.QueryGraph(ctx, "default", "alice", "follows", 10)
	if err != nil {
		log.Printf("QueryGraph failed: %v", err)
		return
	}
	fmt.Printf("✅ Query: alice follows %d nodes\n", len(results))
}

func ipcExample() {
	// Connect via Unix socket (embedded mode)
	client, err := toondb.Connect("/tmp/toondb.sock")
	if err != nil {
		log.Printf("Failed to connect: %v (make sure ToonDB server is running with IPC)", err)
		return
	}
	defer client.Close()

	// Put key-value via IPC
	err = client.Put([]byte("key1"), []byte("value1"))
	if err != nil {
		log.Printf("Put failed: %v", err)
		return
	}
	fmt.Println("✅ IPC Put: key1 -> value1")

	// Get key-value via IPC
	value, err := client.Get([]byte("key1"))
	if err != nil {
		log.Printf("Get failed: %v", err)
		return
	}
	fmt.Printf("✅ IPC Get: key1 -> %s\n", string(value))
}

func formatExample() {
	// Format utilities for LLM context optimization
	fmt.Println("\nFormat Utilities (LLM Context Optimization):")
	fmt.Printf("• WireFormat.Toon: %s (40-66%% fewer tokens than JSON)\n", toondb.WireFormatToon.String())
	fmt.Printf("• WireFormat.JSON: %s\n", toondb.WireFormatJSON.String())
	fmt.Printf("• WireFormat.Columnar: %s\n", toondb.WireFormatColumnar.String())
	fmt.Printf("• ContextFormat.Toon: %s\n", toondb.ContextFormatToon.String())
	fmt.Printf("• ContextFormat.JSON: %s\n", toondb.ContextFormatJSON.String())
	fmt.Printf("• ContextFormat.Markdown: %s\n", toondb.ContextFormatMarkdown.String())

	// Format capabilities
	caps := toondb.NewFormatCapabilities()
	roundTrip := caps.SupportsRoundTripConversion(toondb.WireFormatToon, toondb.ContextFormatToon)
	fmt.Printf("\n✅ TOON round-trip supported: %v\n", roundTrip)
}
