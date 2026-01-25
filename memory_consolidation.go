// Consolidator for Memory System
//
// Event-sourced consolidation with append-only events and derived canonical facts.

package sochdb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/sochdb/sochdb-go/embedded"
)

// Consolidator manages fact consolidation
type Consolidator struct {
	db        *embedded.Database
	namespace string
	config    *ConsolidationConfig
	prefix    []byte
}

// NewConsolidator creates a new consolidator
func NewConsolidator(db *embedded.Database, namespace string, config *ConsolidationConfig) *Consolidator {
	if config == nil {
		config = &ConsolidationConfig{
			SimilarityThreshold: 0.85,
			UseTemporalUpdates:  true,
			MaxConflictAge:      86400, // 24 hours
		}
	}
	return &Consolidator{
		db:        db,
		namespace: namespace,
		config:    config,
		prefix:    []byte(fmt.Sprintf("consolidation:%s:", namespace)),
	}
}

// Add a raw assertion (immutable event)
func (c *Consolidator) Add(assertion *RawAssertion) (string, error) {
	id := assertion.ID
	if id == "" {
		id = c.generateAssertionID(assertion)
	}

	timestamp := assertion.Timestamp
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	storedAssertion := *assertion
	storedAssertion.ID = id
	storedAssertion.Timestamp = timestamp

	key := append(c.prefix, []byte(fmt.Sprintf("assertion:%s", id))...)
	data, err := json.Marshal(storedAssertion)
	if err != nil {
		return "", fmt.Errorf("failed to marshal assertion: %w", err)
	}

	if err := c.db.Put(key, data); err != nil {
		return "", fmt.Errorf("failed to store assertion: %w", err)
	}

	return id, nil
}

// AddWithContradiction adds assertion with contradiction handling
func (c *Consolidator) AddWithContradiction(newAssertion *RawAssertion, contradicts []string) (string, error) {
	id, err := c.Add(newAssertion)
	if err != nil {
		return "", err
	}

	// Mark contradicted assertions
	for _, contradictedID := range contradicts {
		contradictionKey := append(c.prefix, []byte(fmt.Sprintf("contradiction:%s:%s", contradictedID, id))...)
		data, err := json.Marshal(map[string]interface{}{
			"from":      contradictedID,
			"to":        id,
			"timestamp": time.Now().Unix(),
		})
		if err != nil {
			return id, fmt.Errorf("failed to marshal contradiction: %w", err)
		}
		if err := c.db.Put(contradictionKey, data); err != nil {
			return id, fmt.Errorf("failed to store contradiction: %w", err)
		}
	}

	return id, nil
}

// Consolidate runs consolidation to update canonical view
func (c *Consolidator) Consolidate() (int, error) {
	assertions, err := c.getAllAssertions()
	if err != nil {
		return 0, err
	}

	contradictions, err := c.getContradictions()
	if err != nil {
		return 0, err
	}

	// Group assertions by subject
	groups := make(map[string][]*RawAssertion)
	for i := range assertions {
		subject, err := json.Marshal(assertions[i].Fact)
		if err != nil {
			continue
		}
		key := string(subject)
		groups[key] = append(groups[key], &assertions[i])
	}

	updated := 0

	// Create canonical facts
	for _, group := range groups {
		// Sort by confidence and timestamp
		sort.Slice(group, func(i, j int) bool {
			if group[i].Confidence != group[j].Confidence {
				return group[i].Confidence > group[j].Confidence
			}
			return group[i].Timestamp > group[j].Timestamp
		})

		// Filter contradicted assertions
		validAssertions := []*RawAssertion{}
		for _, a := range group {
			isContradicted := false
			for _, cont := range contradictions {
				if cont["from"] == a.ID {
					isContradicted = true
					if c.config.UseTemporalUpdates {
						if ts, ok := cont["timestamp"].(float64); ok {
							age := time.Now().Unix() - int64(ts)
							if age > c.config.MaxConflictAge {
								isContradicted = false
							}
						}
					}
					break
				}
			}
			if !isContradicted {
				validAssertions = append(validAssertions, a)
			}
		}

		if len(validAssertions) > 0 {
			sources := make([]string, len(validAssertions))
			timestamps := make([]int64, len(validAssertions))
			for i, a := range validAssertions {
				sources[i] = a.Source
				timestamps[i] = a.Timestamp
			}

			sort.Slice(timestamps, func(i, j int) bool {
				return timestamps[i] < timestamps[j]
			})

			canonical := CanonicalFact{
				ID:         c.generateCanonicalID(validAssertions[0]),
				MergedFact: validAssertions[0].Fact,
				Confidence: c.mergeConfidence(validAssertions),
				Sources:    sources,
				ValidFrom:  timestamps[0],
			}

			key := append(c.prefix, []byte(fmt.Sprintf("canonical:%s", canonical.ID))...)
			data, err := json.Marshal(canonical)
			if err != nil {
				continue
			}

			if err := c.db.Put(key, data); err != nil {
				continue
			}
			updated++
		}
	}

	return updated, nil
}

// GetCanonicalFacts retrieves canonical facts
func (c *Consolidator) GetCanonicalFacts() ([]CanonicalFact, error) {
	facts := []CanonicalFact{}
	canonicalPrefix := append(c.prefix, []byte("canonical:")...)

	txn := c.db.Begin()
	defer txn.Abort()

	iter := txn.ScanPrefix(canonicalPrefix)
	defer iter.Close()

	for {
		_, value, ok := iter.Next()
		if !ok {
			break
		}

		var fact CanonicalFact
		if err := json.Unmarshal(value, &fact); err != nil {
			continue
		}
		facts = append(facts, fact)
	}

	_ = txn.Commit()
	return facts, nil
}

// Explain provenance of a fact
func (c *Consolidator) Explain(factID string) (map[string]interface{}, error) {
	key := append(c.prefix, []byte(fmt.Sprintf("canonical:%s", factID))...)
	value, err := c.db.Get(key)
	if err != nil {
		return map[string]interface{}{
			"evidence_count": 0,
			"sources":        []string{},
			"confidence":     0.0,
		}, nil
	}

	var fact CanonicalFact
	if err := json.Unmarshal(value, &fact); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"evidence_count": len(fact.Sources),
		"sources":        fact.Sources,
		"confidence":     fact.Confidence,
	}, nil
}

// Get all raw assertions
func (c *Consolidator) getAllAssertions() ([]RawAssertion, error) {
	assertions := []RawAssertion{}
	assertionPrefix := append(c.prefix, []byte("assertion:")...)

	txn := c.db.Begin()
	defer txn.Abort()

	iter := txn.ScanPrefix(assertionPrefix)
	defer iter.Close()

	for {
		_, value, ok := iter.Next()
		if !ok {
			break
		}

		var assertion RawAssertion
		if err := json.Unmarshal(value, &assertion); err != nil {
			continue
		}
		assertions = append(assertions, assertion)
	}

	_ = txn.Commit()
	return assertions, nil
}

// Get all contradictions
func (c *Consolidator) getContradictions() ([]map[string]interface{}, error) {
	contradictions := []map[string]interface{}{}
	contradictionPrefix := append(c.prefix, []byte("contradiction:")...)

	txn := c.db.Begin()
	defer txn.Abort()

	iter := txn.ScanPrefix(contradictionPrefix)
	defer iter.Close()

	for {
		_, value, ok := iter.Next()
		if !ok {
			break
		}

		var contradiction map[string]interface{}
		if err := json.Unmarshal(value, &contradiction); err != nil {
			continue
		}
		contradictions = append(contradictions, contradiction)
	}

	_ = txn.Commit()
	return contradictions, nil
}

// Merge confidence from multiple assertions
func (c *Consolidator) mergeConfidence(assertions []*RawAssertion) float64 {
	if len(assertions) == 0 {
		return 0
	}
	if len(assertions) == 1 {
		return assertions[0].Confidence
	}

	// Weighted average with decreasing weights
	totalWeight := 0.0
	weightedSum := 0.0

	for i, assertion := range assertions {
		weight := 1.0 / float64(i+1)
		weightedSum += assertion.Confidence * weight
		totalWeight += weight
	}

	return weightedSum / totalWeight
}

// Generate deterministic assertion ID
func (c *Consolidator) generateAssertionID(assertion *RawAssertion) string {
	data, _ := json.Marshal(assertion.Fact)
	combined := string(data) + assertion.Source
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])[:16]
}

// Generate deterministic canonical fact ID
func (c *Consolidator) generateCanonicalID(assertion *RawAssertion) string {
	data, _ := json.Marshal(assertion.Fact)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])[:16]
}
