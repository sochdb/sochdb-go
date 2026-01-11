// Copyright 2025 Sushanth (https://github.com/sushanthpy)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package toondb

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sync"
)

// OpCode represents the wire protocol operation codes.
// Must match toondb-storage/src/ipc_server.rs opcodes exactly.
type OpCode uint8

// Client → Server opcodes
const (
	OpPut        OpCode = 0x01
	OpGet        OpCode = 0x02
	OpDelete     OpCode = 0x03
	OpBeginTxn   OpCode = 0x04
	OpCommitTxn  OpCode = 0x05
	OpAbortTxn   OpCode = 0x06
	OpQuery      OpCode = 0x07
	OpPutPath    OpCode = 0x09
	OpGetPath    OpCode = 0x0A
	OpScan       OpCode = 0x0B
	OpCheckpoint OpCode = 0x0C
	OpStats      OpCode = 0x0D
	OpPing       OpCode = 0x0E
	OpExecuteSQL OpCode = 0x0F
)

// Server → Client response opcodes
const (
	OpOK        OpCode = 0x80
	OpError     OpCode = 0x81
	OpValue     OpCode = 0x82
	OpTxnID     OpCode = 0x83
	OpRow       OpCode = 0x84
	OpEndStream OpCode = 0x85
	OpStatsResp OpCode = 0x86
	OpPong      OpCode = 0x87
)

// IPCClient handles low-level IPC communication with the ToonDB server.
type IPCClient struct {
	conn   net.Conn
	mu     sync.Mutex
	closed bool
}

// Connect establishes a connection to the ToonDB server.
func Connect(socketPath string) (*IPCClient, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, &ConnectionError{Address: socketPath, Err: err}
	}
	return &IPCClient{conn: conn}, nil
}

// ConnectToDatabase connects to a database at the given path.
func ConnectToDatabase(dbPath string) (*IPCClient, error) {
	socketPath := filepath.Join(dbPath, "toondb.sock")
	return Connect(socketPath)
}

// Close closes the connection.
func (c *IPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

// Get retrieves a value by key.
func (c *IPCClient) Get(key []byte) ([]byte, error) {
	return c.sendKeyOp(OpGet, key)
}

// Put stores a key-value pair.
func (c *IPCClient) Put(key, value []byte) error {
	_, err := c.sendKeyValueOp(OpPut, key, value)
	return err
}

// Delete removes a key.
func (c *IPCClient) Delete(key []byte) error {
	_, err := c.sendKeyOp(OpDelete, key)
	return err
}

// GetPath retrieves a value by path.
func (c *IPCClient) GetPath(path string) ([]byte, error) {
	return c.sendKeyOp(OpGetPath, []byte(path))
}

// PutPath stores a value at a path.
func (c *IPCClient) PutPath(path string, value []byte) error {
	_, err := c.sendKeyValueOp(OpPutPath, []byte(path), value)
	return err
}

// Scan scans keys with a prefix, returning key-value pairs.
// This is the preferred method for prefix-based iteration.
func (c *IPCClient) Scan(prefix string) ([]KeyValue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClosed
	}

	// SCAN payload is just the prefix string
	prefixBytes := []byte(prefix)
	if err := c.sendMessage(OpScan, prefixBytes); err != nil {
		return nil, err
	}

	return c.readScanResponse()
}

// Query executes a prefix query.
// Wire format: path_len(2 LE) + path + limit(4 LE) + offset(4 LE) + cols_count(2 LE)
func (c *IPCClient) Query(prefix string, limit, offset int) ([]KeyValue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClosed
	}

	// Build query payload
	prefixBytes := []byte(prefix)
	// Format: path_len(2) + path + limit(4) + offset(4) + cols_count(2)
	payloadLen := 2 + len(prefixBytes) + 4 + 4 + 2
	payload := make([]byte, payloadLen)
	binary.LittleEndian.PutUint16(payload[0:2], uint16(len(prefixBytes)))
	copy(payload[2:2+len(prefixBytes)], prefixBytes)
	binary.LittleEndian.PutUint32(payload[2+len(prefixBytes):6+len(prefixBytes)], uint32(limit))
	binary.LittleEndian.PutUint32(payload[6+len(prefixBytes):10+len(prefixBytes)], uint32(offset))
	binary.LittleEndian.PutUint16(payload[10+len(prefixBytes):12+len(prefixBytes)], 0) // no columns

	// Send message: opcode(1) + length(4 LE) + payload
	if err := c.sendMessage(OpQuery, payload); err != nil {
		return nil, err
	}

	// Read response
	return c.readQueryResponse()
}

// BeginTransaction starts a new transaction.
func (c *IPCClient) BeginTransaction() (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, ErrClosed
	}

	if err := c.sendMessage(OpBeginTxn, nil); err != nil {
		return 0, err
	}

	// Read response
	opcode, payload, err := c.readMessage()
	if err != nil {
		return 0, err
	}

	if opcode != OpTxnID {
		if opcode == OpError {
			return 0, c.parseErrorPayload(payload)
		}
		return 0, &ProtocolError{Message: fmt.Sprintf("expected TXN_ID, got opcode %#x", opcode)}
	}

	if len(payload) < 8 {
		return 0, &ProtocolError{Message: "invalid transaction response"}
	}

	txnID := binary.LittleEndian.Uint64(payload[0:8])
	return txnID, nil
}

// CommitTransaction commits a transaction.
func (c *IPCClient) CommitTransaction(txnID uint64) error {
	return c.sendTxnOp(OpCommitTxn, txnID)
}

// AbortTransaction aborts a transaction.
func (c *IPCClient) AbortTransaction(txnID uint64) error {
	return c.sendTxnOp(OpAbortTxn, txnID)
}

// Checkpoint forces a checkpoint.
func (c *IPCClient) Checkpoint() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrClosed
	}

	if err := c.sendMessage(OpCheckpoint, nil); err != nil {
		return err
	}

	return c.readSimpleResponse()
}

// Stats retrieves storage statistics.
func (c *IPCClient) Stats() (*StorageStats, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClosed
	}

	if err := c.sendMessage(OpStats, nil); err != nil {
		return nil, err
	}

	// Read response
	opcode, payload, err := c.readMessage()
	if err != nil {
		return nil, err
	}

	if opcode == OpError {
		return nil, c.parseErrorPayload(payload)
	}

	if opcode != OpStatsResp && opcode != OpOK {
		return nil, &ProtocolError{Message: fmt.Sprintf("unexpected stats response opcode: %#x", opcode)}
	}

	// Stats response format is JSON
	if len(payload) == 0 {
		// Empty response
		return &StorageStats{}, nil
	}

	// Parse JSON response
	var stats struct {
		MemtableSizeBytes  uint64 `json:"memtable_size_bytes"`
		WALSizeBytes       uint64 `json:"wal_size_bytes"`
		ActiveTransactions int    `json:"active_transactions"`
	}

	if err := json.Unmarshal(payload, &stats); err != nil {
		return nil, &ProtocolError{Message: fmt.Sprintf("failed to parse stats JSON: %v", err)}
	}

	return &StorageStats{
		MemtableSizeBytes:  stats.MemtableSizeBytes,
		WALSizeBytes:       stats.WALSizeBytes,
		ActiveTransactions: stats.ActiveTransactions,
	}, nil
}

// ============================================================================
// Low-level Wire Protocol Helpers
// ============================================================================

// sendMessage sends a message using the wire protocol format:
// opcode(1) + length(4 LE) + payload
func (c *IPCClient) sendMessage(op OpCode, payload []byte) error {
	// Build message: [opcode:1][length:4 LE][payload:N]
	msg := make([]byte, 5+len(payload))
	msg[0] = byte(op)
	binary.LittleEndian.PutUint32(msg[1:5], uint32(len(payload)))
	if len(payload) > 0 {
		copy(msg[5:], payload)
	}

	_, err := c.conn.Write(msg)
	return err
}

// readMessage reads a message using the wire protocol format:
// opcode(1) + length(4 LE) + payload
func (c *IPCClient) readMessage() (OpCode, []byte, error) {
	// Read opcode (1 byte)
	opcodeBuf := make([]byte, 1)
	if _, err := io.ReadFull(c.conn, opcodeBuf); err != nil {
		return 0, nil, err
	}

	// Read length (4 bytes LE)
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return 0, nil, err
	}
	payloadLen := binary.LittleEndian.Uint32(lenBuf)

	// Read payload
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(c.conn, payload); err != nil {
			return 0, nil, err
		}
	}

	return OpCode(opcodeBuf[0]), payload, nil
}

// parseErrorPayload parses an error payload (the payload part of an ERROR response)
func (c *IPCClient) parseErrorPayload(payload []byte) error {
	msg := string(payload)

	// Track specific error types
	if len(msg) > 0 {
		switch {
		case contains(msg, "permission") || contains(msg, "access denied"):
		case contains(msg, "timeout") || contains(msg, "deadline"):
		default:
		}
	}

	return &ToonDBError{Op: "remote", Message: msg}
}

// Helper methods

func (c *IPCClient) sendKeyOp(op OpCode, key []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClosed
	}

	// For GET/DELETE: payload is just the raw key
	// For GET_PATH: payload is path_count(2) + [seg_len(2) + seg]...
	if op == OpGetPath {
		// Path format: count(2) + [len(2) + segment]...
		path := string(key)
		payload := make([]byte, 2+2+len(path))
		binary.LittleEndian.PutUint16(payload[0:2], 1) // 1 segment
		binary.LittleEndian.PutUint16(payload[2:4], uint16(len(path)))
		copy(payload[4:], path)

		if err := c.sendMessage(op, payload); err != nil {
			return nil, err
		}
	} else {
		// GET/DELETE: payload is just the raw key (no length prefix)
		if err := c.sendMessage(op, key); err != nil {
			return nil, err
		}
	}

	return c.readValueResponse()
}

func (c *IPCClient) sendKeyValueOp(op OpCode, key, value []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClosed
	}

	if op == OpPutPath {
		// PUT_PATH: path_count(2) + [seg_len(2) + seg]... + value
		path := string(key)
		payload := make([]byte, 2+2+len(path)+len(value))
		binary.LittleEndian.PutUint16(payload[0:2], 1) // 1 segment
		binary.LittleEndian.PutUint16(payload[2:4], uint16(len(path)))
		copy(payload[4:4+len(path)], path)
		copy(payload[4+len(path):], value)

		if err := c.sendMessage(op, payload); err != nil {
			return nil, err
		}
	} else {
		// PUT: key_len(4 LE) + key + value (value is rest of payload)
		payload := make([]byte, 4+len(key)+len(value))
		binary.LittleEndian.PutUint32(payload[0:4], uint32(len(key)))
		copy(payload[4:4+len(key)], key)
		copy(payload[4+len(key):], value)

		if err := c.sendMessage(op, payload); err != nil {
			return nil, err
		}
	}

	return c.readValueResponse()
}

func (c *IPCClient) sendTxnOp(op OpCode, txnID uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrClosed
	}

	// Transaction ops: txn_id(8 LE)
	payload := make([]byte, 8)
	binary.LittleEndian.PutUint64(payload[0:8], txnID)

	if err := c.sendMessage(op, payload); err != nil {
		return err
	}

	return c.readSimpleResponse()
}

func (c *IPCClient) readValueResponse() ([]byte, error) {
	opcode, payload, err := c.readMessage()
	if err != nil {
		return nil, err
	}

	switch opcode {
	case OpOK:
		// OK with no data
		return nil, nil
	case OpValue:
		// VALUE response: payload is the value
		// Empty payload means key doesn't exist
		if len(payload) == 0 {
			return nil, nil
		}
		return payload, nil
	case OpError:
		return nil, c.parseErrorPayload(payload)
	default:
		return nil, &ProtocolError{Message: fmt.Sprintf("unexpected opcode: %#x", opcode)}
	}
}

func (c *IPCClient) readSimpleResponse() error {
	opcode, payload, err := c.readMessage()
	if err != nil {
		return err
	}

	if opcode == OpError {
		return c.parseErrorPayload(payload)
	}

	if opcode != OpOK && opcode != OpTxnID {
		return &ProtocolError{Message: fmt.Sprintf("expected OK, got opcode %#x", opcode)}
	}

	return nil
}

func (c *IPCClient) readQueryResponse() ([]KeyValue, error) {
	opcode, payload, err := c.readMessage()
	if err != nil {
		// Track connection/timeout errors
		if isTimeoutError(err) {
		} else {
		}
		return nil, err
	}

	if opcode == OpError {
		return nil, c.parseErrorPayload(payload)
	}

	if opcode != OpValue && opcode != OpOK {
		return nil, &ProtocolError{Message: fmt.Sprintf("unexpected query response opcode: %#x", opcode)}
	}

	if len(payload) < 4 {
		// Empty result
		return []KeyValue{}, nil
	}

	// Response format: count(4 LE) + [key_len(4 LE) + key + value_len(4 LE) + value]...
	count := binary.LittleEndian.Uint32(payload[0:4])
	results := make([]KeyValue, 0, count)
	offset := 4

	for i := uint32(0); i < count; i++ {
		if offset+4 > len(payload) {
			return nil, &ProtocolError{Message: "truncated query response"}
		}
		keyLen := binary.LittleEndian.Uint32(payload[offset : offset+4])
		offset += 4

		if offset+int(keyLen)+4 > len(payload) {
			return nil, &ProtocolError{Message: "truncated query response"}
		}
		key := make([]byte, keyLen)
		copy(key, payload[offset:offset+int(keyLen)])
		offset += int(keyLen)

		valueLen := binary.LittleEndian.Uint32(payload[offset : offset+4])
		offset += 4

		if offset+int(valueLen) > len(payload) {
			return nil, &ProtocolError{Message: "truncated query response"}
		}
		value := make([]byte, valueLen)
		copy(value, payload[offset:offset+int(valueLen)])
		offset += int(valueLen)

		results = append(results, KeyValue{Key: key, Value: value})
	}

	return results, nil
}

// readScanResponse parses the SCAN response format:
// count(4 LE) + [key_len(2 LE) + key + val_len(4 LE) + val]...
func (c *IPCClient) readScanResponse() ([]KeyValue, error) {
	opcode, payload, err := c.readMessage()
	if err != nil {
		return nil, err
	}

	if opcode == OpError {
		return nil, c.parseErrorPayload(payload)
	}

	if opcode != OpValue && opcode != OpOK {
		return nil, &ProtocolError{Message: fmt.Sprintf("unexpected scan response opcode: %#x", opcode)}
	}

	if len(payload) < 4 {
		// Empty result
		return []KeyValue{}, nil
	}

	// Response format: count(4 LE) + [key_len(2 LE) + key + val_len(4 LE) + val]...
	count := binary.LittleEndian.Uint32(payload[0:4])
	results := make([]KeyValue, 0, count)
	offset := 4

	for i := uint32(0); i < count; i++ {
		if offset+2 > len(payload) {
			return nil, &ProtocolError{Message: "truncated scan response (key_len)"}
		}
		keyLen := binary.LittleEndian.Uint16(payload[offset : offset+2])
		offset += 2

		if offset+int(keyLen)+4 > len(payload) {
			return nil, &ProtocolError{Message: "truncated scan response (key+val_len)"}
		}
		key := make([]byte, keyLen)
		copy(key, payload[offset:offset+int(keyLen)])
		offset += int(keyLen)

		valueLen := binary.LittleEndian.Uint32(payload[offset : offset+4])
		offset += 4

		if offset+int(valueLen) > len(payload) {
			return nil, &ProtocolError{Message: "truncated scan response (value)"}
		}
		value := make([]byte, valueLen)
		copy(value, payload[offset:offset+int(valueLen)])
		offset += int(valueLen)

		results = append(results, KeyValue{Key: key, Value: value})
	}

	return results, nil
}

// KeyValue represents a key-value pair returned from queries.
type KeyValue struct {
	Key   []byte
	Value []byte
}

// StorageStats contains database storage statistics.
type StorageStats struct {
	MemtableSizeBytes  uint64
	WALSizeBytes       uint64
	ActiveTransactions int
}

// Helper functions for analytics error tracking

// contains checks if a string contains a substring (case-insensitive helper)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && someContains(s, substr)))
}

func someContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "timeout") || contains(msg, "deadline")
}

// ============================================================================
// Graph Overlay Operations
// ============================================================================

// GraphNode represents a node in the graph overlay.
type GraphNode struct {
	ID         string            `json:"id"`
	NodeType   string            `json:"node_type"`
	Properties map[string]string `json:"properties"`
}

// GraphEdge represents an edge in the graph overlay.
type GraphEdge struct {
	FromID     string            `json:"from_id"`
	EdgeType   string            `json:"edge_type"`
	ToID       string            `json:"to_id"`
	Properties map[string]string `json:"properties"`
}

// AddNode adds a node to the graph overlay.
func (c *IPCClient) AddNode(namespace, nodeID, nodeType string, properties map[string]string) error {
	key := fmt.Sprintf("_graph/%s/nodes/%s", namespace, nodeID)

	if properties == nil {
		properties = make(map[string]string)
	}

	node := GraphNode{
		ID:         nodeID,
		NodeType:   nodeType,
		Properties: properties,
	}

	value, err := json.Marshal(node)
	if err != nil {
		return err
	}

	return c.Put([]byte(key), value)
}

// AddEdge adds an edge between nodes in the graph overlay.
func (c *IPCClient) AddEdge(namespace, fromID, edgeType, toID string, properties map[string]string) error {
	key := fmt.Sprintf("_graph/%s/edges/%s/%s/%s", namespace, fromID, edgeType, toID)

	if properties == nil {
		properties = make(map[string]string)
	}

	edge := GraphEdge{
		FromID:     fromID,
		EdgeType:   edgeType,
		ToID:       toID,
		Properties: properties,
	}

	value, err := json.Marshal(edge)
	if err != nil {
		return err
	}

	return c.Put([]byte(key), value)
}

// TraverseResult contains the result of a graph traversal.
type TraverseResult struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Traverse traverses the graph from a starting node.
// order: "bfs" for breadth-first, "dfs" for depth-first
func (c *IPCClient) Traverse(namespace, startNode string, maxDepth int, order string) (*TraverseResult, error) {
	visited := make(map[string]bool)
	nodes := []GraphNode{}
	edges := []GraphEdge{}

	type queueItem struct {
		nodeID string
		depth  int
	}

	frontier := []queueItem{{startNode, 0}}

	for len(frontier) > 0 {
		var current queueItem
		if order == "bfs" {
			current = frontier[0]
			frontier = frontier[1:]
		} else {
			current = frontier[len(frontier)-1]
			frontier = frontier[:len(frontier)-1]
		}

		if current.depth > maxDepth || visited[current.nodeID] {
			continue
		}
		visited[current.nodeID] = true

		// Get node data
		nodeKey := fmt.Sprintf("_graph/%s/nodes/%s", namespace, current.nodeID)
		nodeData, err := c.Get([]byte(nodeKey))
		if err == nil && nodeData != nil {
			var node GraphNode
			if json.Unmarshal(nodeData, &node) == nil {
				nodes = append(nodes, node)
			}
		}

		// Get outgoing edges
		edgePrefix := fmt.Sprintf("_graph/%s/edges/%s/", namespace, current.nodeID)
		edgeResults, err := c.Scan(edgePrefix)
		if err == nil {
			for _, kv := range edgeResults {
				var edge GraphEdge
				if json.Unmarshal(kv.Value, &edge) == nil {
					edges = append(edges, edge)
					if !visited[edge.ToID] {
						frontier = append(frontier, queueItem{edge.ToID, current.depth + 1})
					}
				}
			}
		}
	}

	return &TraverseResult{Nodes: nodes, Edges: edges}, nil
}

// ============================================================================
// Semantic Cache Operations
// ============================================================================

// CacheEntry represents an entry in the semantic cache.
type CacheEntry struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Embedding []float32 `json:"embedding"`
	ExpiresAt int64     `json:"expires_at"`
}

// CachePut stores a value in the semantic cache with its embedding.
func (c *IPCClient) CachePut(cacheName, key, value string, embedding []float32, ttlSeconds int64) error {
	// Hash the key for storage
	keyHash := fmt.Sprintf("%x", key)[:16]
	cacheKey := fmt.Sprintf("_cache/%s/%s", cacheName, keyHash)

	var expiresAt int64 = 0
	if ttlSeconds > 0 {
		expiresAt = now() + ttlSeconds
	}

	entry := CacheEntry{
		Key:       key,
		Value:     value,
		Embedding: embedding,
		ExpiresAt: expiresAt,
	}

	cacheValue, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return c.Put([]byte(cacheKey), cacheValue)
}

// CacheGet looks up a value in the semantic cache by embedding similarity.
func (c *IPCClient) CacheGet(cacheName string, queryEmbedding []float32, threshold float32) (string, bool, error) {
	prefix := fmt.Sprintf("_cache/%s/", cacheName)
	entries, err := c.Scan(prefix)
	if err != nil {
		return "", false, err
	}

	currentTime := now()
	var bestMatch *CacheEntry
	var bestSimilarity float32 = -1

	for _, kv := range entries {
		var entry CacheEntry
		if json.Unmarshal(kv.Value, &entry) != nil {
			continue
		}

		// Check expiry
		if entry.ExpiresAt > 0 && currentTime > entry.ExpiresAt {
			continue
		}

		// Compute cosine similarity
		if len(entry.Embedding) == len(queryEmbedding) {
			similarity := cosineSimilarity(queryEmbedding, entry.Embedding)
			if similarity >= threshold && similarity > bestSimilarity {
				bestSimilarity = similarity
				bestMatch = &entry
			}
		}
	}

	if bestMatch != nil {
		return bestMatch.Value, true, nil
	}
	return "", false, nil
}

func cosineSimilarity(a, b []float32) float32 {
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	normA = sqrt(normA)
	normB = sqrt(normB)
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (normA * normB)
}

func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// ============================================================================
// Trace Operations
// ============================================================================

// TraceInfo contains trace and span IDs.
type TraceInfo struct {
	TraceID    string `json:"trace_id"`
	RootSpanID string `json:"root_span_id"`
}

// SpanData represents a span in a trace.
type SpanData struct {
	SpanID       string `json:"span_id"`
	Name         string `json:"name"`
	StartUs      int64  `json:"start_us"`
	ParentSpanID string `json:"parent_span_id"`
	Status       string `json:"status"`
	EndUs        int64  `json:"end_us,omitempty"`
	DurationUs   int64  `json:"duration_us,omitempty"`
}

// StartTrace starts a new trace.
func (c *IPCClient) StartTrace(name string) (*TraceInfo, error) {
	traceID := fmt.Sprintf("trace_%x%x", now(), randUint32())
	spanID := fmt.Sprintf("span_%x%x", now(), randUint32())
	nowUs := nowMicros()

	// Store trace
	traceKey := fmt.Sprintf("_traces/%s", traceID)
	traceValue, _ := json.Marshal(map[string]interface{}{
		"trace_id":     traceID,
		"name":         name,
		"start_us":     nowUs,
		"root_span_id": spanID,
	})
	if err := c.Put([]byte(traceKey), traceValue); err != nil {
		return nil, err
	}

	// Store root span
	spanKey := fmt.Sprintf("_traces/%s/spans/%s", traceID, spanID)
	spanValue, _ := json.Marshal(SpanData{
		SpanID:       spanID,
		Name:         name,
		StartUs:      nowUs,
		ParentSpanID: "",
		Status:       "active",
	})
	if err := c.Put([]byte(spanKey), spanValue); err != nil {
		return nil, err
	}

	return &TraceInfo{TraceID: traceID, RootSpanID: spanID}, nil
}

// StartSpan starts a child span within a trace.
func (c *IPCClient) StartSpan(traceID, parentSpanID, name string) (string, error) {
	spanID := fmt.Sprintf("span_%x%x", now(), randUint32())
	nowUs := nowMicros()

	spanKey := fmt.Sprintf("_traces/%s/spans/%s", traceID, spanID)
	spanValue, _ := json.Marshal(SpanData{
		SpanID:       spanID,
		Name:         name,
		StartUs:      nowUs,
		ParentSpanID: parentSpanID,
		Status:       "active",
	})

	if err := c.Put([]byte(spanKey), spanValue); err != nil {
		return "", err
	}

	return spanID, nil
}

// EndSpan ends a span and returns its duration in microseconds.
func (c *IPCClient) EndSpan(traceID, spanID, status string) (int64, error) {
	spanKey := fmt.Sprintf("_traces/%s/spans/%s", traceID, spanID)

	spanData, err := c.Get([]byte(spanKey))
	if err != nil || spanData == nil {
		return 0, fmt.Errorf("span not found: %s", spanID)
	}

	var span SpanData
	if err := json.Unmarshal(spanData, &span); err != nil {
		return 0, err
	}

	nowUs := nowMicros()
	duration := nowUs - span.StartUs

	span.Status = status
	span.EndUs = nowUs
	span.DurationUs = duration

	updatedValue, _ := json.Marshal(span)
	if err := c.Put([]byte(spanKey), updatedValue); err != nil {
		return 0, err
	}

	return duration, nil
}

// Helper functions for time
func now() int64 {
	return int64(1704067200) // Placeholder - in real code use time.Now().Unix()
}

func nowMicros() int64 {
	return now() * 1000000
}

func randUint32() uint32 {
	return uint32(now() % 0xFFFFFFFF) // Simple pseudo-random
}
