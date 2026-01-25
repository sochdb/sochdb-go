// Semantic Cache for LLM responses
//
// Cache LLM responses with similarity-based retrieval for cost savings.
// Uses database prefix scanning to store and retrieve cached responses.

package sochdb

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/sochdb/sochdb-go/embedded"
)

// SemanticCacheEntry represents a cached response with embedding
type SemanticCacheEntry struct {
	Key       string                 `json:"key"`
	Value     string                 `json:"value"`
	Embedding []float32              `json:"embedding"`
	Timestamp int64                  `json:"timestamp"`
	TTL       int64                  `json:"ttl,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SemanticCacheHit represents a cache hit with similarity score
type SemanticCacheHit struct {
	SemanticCacheEntry
	Score float32 `json:"score"`
}

// SemanticCacheStats represents cache statistics
type SemanticCacheStats struct {
	Count       int     `json:"count"`
	Hits        int     `json:"hits"`
	Misses      int     `json:"misses"`
	HitRate     float64 `json:"hit_rate"`
	MemoryUsage int64   `json:"memory_usage"`
}

// SemanticCache provides semantic caching for LLM responses
type SemanticCache struct {
	db        *embedded.Database
	cacheName string
	prefix    []byte
	hits      int
	misses    int
}

// NewSemanticCache creates a new semantic cache
func NewSemanticCache(db *embedded.Database, cacheName string) *SemanticCache {
	return &SemanticCache{
		db:        db,
		cacheName: cacheName,
		prefix:    []byte(fmt.Sprintf("cache:%s:", cacheName)),
		hits:      0,
		misses:    0,
	}
}

// semanticCosineSimilarity calculates cosine similarity between two vectors
func semanticCosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vectors must have same length")
	}

	var dotProduct, normA, normB float32
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0, nil
	}

	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB)))), nil
}

// Put stores a cached response
func (c *SemanticCache) Put(key, value string, embedding []float32, ttlSeconds int64, metadata map[string]interface{}) error {
	entry := SemanticCacheEntry{
		Key:       key,
		Value:     value,
		Embedding: embedding,
		Timestamp: time.Now().Unix(),
		TTL:       ttlSeconds,
		Metadata:  metadata,
	}

	entryBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	entryKey := append(c.prefix, []byte(key)...)
	return c.db.Put(entryKey, entryBytes)
}

// Get retrieves cached response by similarity
func (c *SemanticCache) Get(queryEmbedding []float32, threshold float32) (*SemanticCacheHit, error) {
	now := time.Now().Unix()
	var bestMatch *SemanticCacheHit
	bestScore := threshold

	// Begin transaction
	txn := c.db.Begin()
	defer txn.Abort()

	// Scan all cache entries with this prefix
	iter := txn.ScanPrefix(c.prefix)
	defer iter.Close()

	for {
		_, value, ok := iter.Next()
		if !ok {
			break
		}

		var entry SemanticCacheEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			continue
		}

		// Check TTL expiration
		if entry.TTL > 0 && entry.Timestamp > 0 {
			expiresAt := entry.Timestamp + entry.TTL
			if now > expiresAt {
				continue
			}
		}

		// Calculate similarity
		score, err := semanticCosineSimilarity(queryEmbedding, entry.Embedding)
		if err != nil {
			continue
		}

		// Update best match
		if score > bestScore {
			bestScore = score
			bestMatch = &SemanticCacheHit{
				SemanticCacheEntry: entry,
				Score:              score,
			}
		}
	}

	_ = txn.Commit()

	if bestMatch != nil {
		c.hits++
	} else {
		c.misses++
	}

	return bestMatch, nil
}

// Delete removes a specific cache entry
func (c *SemanticCache) Delete(key string) error {
	entryKey := append(c.prefix, []byte(key)...)
	return c.db.Delete(entryKey)
}

// Clear removes all entries in this cache
func (c *SemanticCache) Clear() (int, error) {
	deleted := 0
	toDelete := [][]byte{}

	// Begin transaction for scanning
	txn := c.db.Begin()
	defer txn.Abort()

	// Collect keys to delete
	iter := txn.ScanPrefix(c.prefix)
	defer iter.Close()

	for {
		key, _, ok := iter.Next()
		if !ok {
			break
		}

		keyCopy := make([]byte, len(key))
		copy(keyCopy, key)
		toDelete = append(toDelete, keyCopy)
	}

	_ = txn.Commit()

	// Delete collected keys
	for _, key := range toDelete {
		if err := c.db.Delete(key); err != nil {
			return deleted, err
		}
		deleted++
	}

	// Reset stats
	c.hits = 0
	c.misses = 0

	return deleted, nil
}

// Stats returns cache statistics
func (c *SemanticCache) Stats() (*SemanticCacheStats, error) {
	now := time.Now().Unix()
	count := 0
	var memoryUsage int64

	// Begin transaction
	txn := c.db.Begin()
	defer txn.Abort()

	iter := txn.ScanPrefix(c.prefix)
	defer iter.Close()

	for {
		key, value, ok := iter.Next()
		if !ok {
			break
		}

		var entry SemanticCacheEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			continue
		}

		// Skip expired entries
		if entry.TTL > 0 && entry.Timestamp > 0 {
			expiresAt := entry.Timestamp + entry.TTL
			if now > expiresAt {
				continue
			}
		}

		count++
		memoryUsage += int64(len(key) + len(value))
	}

	_ = txn.Commit()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return &SemanticCacheStats{
		Count:       count,
		Hits:        c.hits,
		Misses:      c.misses,
		HitRate:     hitRate,
		MemoryUsage: memoryUsage,
	}, nil
}

// PurgeExpired removes expired entries
func (c *SemanticCache) PurgeExpired() (int, error) {
	now := time.Now().Unix()
	purged := 0
	toDelete := [][]byte{}

	// Begin transaction for scanning
	txn := c.db.Begin()
	defer txn.Abort()

	// Collect expired keys
	iter := txn.ScanPrefix(c.prefix)
	defer iter.Close()

	for {
		key, value, ok := iter.Next()
		if !ok {
			break
		}

		var entry SemanticCacheEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			continue
		}

		if entry.TTL > 0 && entry.Timestamp > 0 {
			expiresAt := entry.Timestamp + entry.TTL
			if now > expiresAt {
				keyCopy := make([]byte, len(key))
				copy(keyCopy, key)
				toDelete = append(toDelete, keyCopy)
			}
		}
	}

	_ = txn.Commit()

	// Delete expired keys
	for _, key := range toDelete {
		if err := c.db.Delete(key); err != nil {
			return purged, err
		}
		purged++
	}

	return purged, nil
}
