// Package toondb provides a thin gRPC client wrapper for the ToonDB server.
// All business logic runs on the server (Thick Server / Thin Client architecture).
//
// The client is approximately ~300 lines of code, delegating all operations to the server.
package toondb

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GrpcClient provides thin gRPC client for ToonDB.
// All operations are delegated to the ToonDB gRPC server.
type GrpcClient struct {
	conn    *grpc.ClientConn
	address string
	timeout time.Duration
}

// GrpcClientOptions configures the gRPC client.
type GrpcClientOptions struct {
	Address string
	Timeout time.Duration
	Secure  bool
}

// GrpcSearchResult represents a vector search result.
type GrpcSearchResult struct {
	ID       uint64
	Distance float32
}

// GrpcDocument represents a document with embedding.
type GrpcDocument struct {
	ID        string
	Content   string
	Embedding []float32
	Metadata  map[string]string
}

// GrpcGraphNode represents a node in the graph.
type GrpcGraphNode struct {
	ID         string
	NodeType   string
	Properties map[string]string
}

// GrpcGraphEdge represents an edge in the graph.
type GrpcGraphEdge struct {
	FromID     string
	EdgeType   string
	ToID       string
	Properties map[string]string
}

// NewGrpcClient creates a new gRPC client connected to ToonDB server.
func NewGrpcClient(opts GrpcClientOptions) (*GrpcClient, error) {
	if opts.Address == "" {
		opts.Address = "localhost:50051"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}

	// Create connection options
	dialOpts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100 * 1024 * 1024)),
	}

	if opts.Secure {
		return nil, fmt.Errorf("secure connections not yet implemented")
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.Dial(opts.Address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", opts.Address, err)
	}

	return &GrpcClient{
		conn:    conn,
		address: opts.Address,
		timeout: opts.Timeout,
	}, nil
}

// GrpcConnect is a convenience function to connect to a ToonDB gRPC server.
func GrpcConnect(address string) (*GrpcClient, error) {
	return NewGrpcClient(GrpcClientOptions{Address: address})
}

// Close closes the gRPC connection.
func (c *GrpcClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ctx returns a context with the configured timeout.
func (c *GrpcClient) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), c.timeout)
}

// ===========================================================================
// Vector Index Operations
// ===========================================================================

// CreateIndex creates a new vector index.
// Note: Requires generated proto code. This is a placeholder implementation.
func (c *GrpcClient) CreateIndex(name string, dimension int, metric string) error {
	// TODO: Call VectorIndexService.CreateIndex via generated client
	return fmt.Errorf("gRPC proto files not yet generated - run protoc to generate Go client code")
}

// InsertVectors inserts vectors into an index.
func (c *GrpcClient) InsertVectors(indexName string, ids []uint64, vectors [][]float32) (int, error) {
	// TODO: Call VectorIndexService.InsertBatch via generated client
	return 0, fmt.Errorf("gRPC proto files not yet generated")
}

// GrpcSearch performs k-nearest neighbor search.
func (c *GrpcClient) GrpcSearch(indexName string, query []float32, k int) ([]GrpcSearchResult, error) {
	// TODO: Call VectorIndexService.Search via generated client
	return nil, fmt.Errorf("gRPC proto files not yet generated")
}

// ===========================================================================
// Collection Operations
// ===========================================================================

// CreateCollection creates a new collection.
func (c *GrpcClient) CreateCollection(name string, dimension int, namespace string) error {
	// TODO: Call CollectionService.CreateCollection
	return fmt.Errorf("gRPC proto files not yet generated")
}

// AddDocuments adds documents to a collection.
func (c *GrpcClient) AddDocuments(collectionName string, documents []GrpcDocument, namespace string) ([]string, error) {
	// TODO: Call CollectionService.AddDocuments
	return nil, fmt.Errorf("gRPC proto files not yet generated")
}

// SearchCollection searches a collection for similar documents.
func (c *GrpcClient) SearchCollection(collectionName string, query []float32, k int, namespace string) ([]GrpcDocument, error) {
	// TODO: Call CollectionService.SearchCollection
	return nil, fmt.Errorf("gRPC proto files not yet generated")
}

// ===========================================================================
// Graph Operations
// ===========================================================================

// AddGraphNode adds a node to the graph.
func (c *GrpcClient) AddGraphNode(nodeID, nodeType string, properties map[string]string, namespace string) error {
	// TODO: Call GraphService.AddNode
	return fmt.Errorf("gRPC proto files not yet generated")
}

// AddGraphEdge adds an edge between nodes.
func (c *GrpcClient) AddGraphEdge(fromID, edgeType, toID string, properties map[string]string, namespace string) error {
	// TODO: Call GraphService.AddEdge
	return fmt.Errorf("gRPC proto files not yet generated")
}

// TraverseGraph performs graph traversal from a starting node.
func (c *GrpcClient) TraverseGraph(startNode string, maxDepth int, order string, namespace string) ([]GrpcGraphNode, []GrpcGraphEdge, error) {
	// TODO: Call GraphService.Traverse
	return nil, nil, fmt.Errorf("gRPC proto files not yet generated")
}

// ===========================================================================
// Semantic Cache Operations
// ===========================================================================

// CacheGet retrieves from semantic cache by similarity.
func (c *GrpcClient) CacheGet(cacheName string, queryEmbedding []float32, threshold float32) (string, bool, error) {
	// TODO: Call SemanticCacheService.Get
	return "", false, fmt.Errorf("gRPC proto files not yet generated")
}

// CachePut stores a value in the semantic cache.
func (c *GrpcClient) CachePut(cacheName, key, value string, keyEmbedding []float32, ttlSeconds int) error {
	// TODO: Call SemanticCacheService.Put
	return fmt.Errorf("gRPC proto files not yet generated")
}

// ===========================================================================
// Trace Operations
// ===========================================================================

// StartTrace starts a new trace.
func (c *GrpcClient) StartTrace(name string) (traceID, rootSpanID string, err error) {
	// TODO: Call TraceService.StartTrace
	return "", "", fmt.Errorf("gRPC proto files not yet generated")
}

// StartSpan starts a span within a trace.
func (c *GrpcClient) StartSpan(traceID, parentSpanID, name string) (spanID string, err error) {
	// TODO: Call TraceService.StartSpan
	return "", fmt.Errorf("gRPC proto files not yet generated")
}

// EndSpan ends a span.
func (c *GrpcClient) EndSpan(traceID, spanID, status string) (durationUs int64, err error) {
	// TODO: Call TraceService.EndSpan
	return 0, fmt.Errorf("gRPC proto files not yet generated")
}

// ===========================================================================
// KV Operations
// ===========================================================================

// GrpcGet retrieves a value by key.
func (c *GrpcClient) GrpcGet(key []byte, namespace string) ([]byte, bool, error) {
	// TODO: Call KvService.Get
	return nil, false, fmt.Errorf("gRPC proto files not yet generated")
}

// GrpcPut stores a value.
func (c *GrpcClient) GrpcPut(key, value []byte, namespace string, ttlSeconds int) error {
	// TODO: Call KvService.Put
	return fmt.Errorf("gRPC proto files not yet generated")
}

// GrpcDelete removes a key.
func (c *GrpcClient) GrpcDelete(key []byte, namespace string) error {
	// TODO: Call KvService.Delete
	return fmt.Errorf("gRPC proto files not yet generated")
}

// ===========================================================================
// Convenience Methods (Simpler API)
// ===========================================================================

// PutKv stores a key-value pair in the specified namespace.
func (c *GrpcClient) PutKv(ctx context.Context, namespace, key string, value []byte) error {
	return c.GrpcPut([]byte(key), value, namespace, 0)
}

// GetKv retrieves a value by key from the specified namespace.
func (c *GrpcClient) GetKv(ctx context.Context, namespace, key string) ([]byte, error) {
	value, found, err := c.GrpcGet([]byte(key), namespace)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return value, nil
}

// AddEdge adds an edge to the graph (convenience wrapper).
func (c *GrpcClient) AddEdge(ctx context.Context, namespace string, edge GrpcGraphEdge) error {
	return c.AddGraphEdge(edge.FromID, edge.EdgeType, edge.ToID, edge.Properties, namespace)
}

// QueryGraph queries the graph for edges (convenience wrapper).
func (c *GrpcClient) QueryGraph(ctx context.Context, namespace, fromID, edgeType string, limit int) ([]GrpcGraphEdge, error) {
	// For now, return placeholder
	return nil, fmt.Errorf("gRPC proto files not yet generated")
}
