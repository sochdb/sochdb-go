// Extraction Pipeline for Memory System
//
// Compiles LLM outputs into typed, validated facts (Entity, Relation, Assertion).

package sochdb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sochdb/sochdb-go/embedded"
)

// ExtractorFunction type - user provides this to call their LLM
type ExtractorFunction func(text string) (map[string]interface{}, error)

// ExtractionPipeline compiles LLM outputs into typed facts
type ExtractionPipeline struct {
	db        *embedded.Database
	namespace string
	schema    *ExtractionSchema
	prefix    []byte
}

// NewExtractionPipeline creates a new extraction pipeline
func NewExtractionPipeline(db *embedded.Database, namespace string, schema *ExtractionSchema) *ExtractionPipeline {
	return &ExtractionPipeline{
		db:        db,
		namespace: namespace,
		schema:    schema,
		prefix:    []byte(fmt.Sprintf("memory:%s:", namespace)),
	}
}

// Extract entities and relations from text
func (p *ExtractionPipeline) Extract(text string, extractor ExtractorFunction) (*ExtractionResult, error) {
	rawResult, err := extractor(text)
	if err != nil {
		return nil, fmt.Errorf("extractor failed: %w", err)
	}

	timestamp := time.Now().Unix()
	result := &ExtractionResult{
		Entities:   []Entity{},
		Relations:  []Relation{},
		Assertions: []Assertion{},
	}

	// Extract entities
	if entitiesRaw, ok := rawResult["entities"].([]interface{}); ok {
		for _, entityRaw := range entitiesRaw {
			if entityMap, ok := entityRaw.(map[string]interface{}); ok {
				name, _ := entityMap["name"].(string)
				entityType, _ := entityMap["entity_type"].(string)
				confidence, _ := entityMap["confidence"].(float64)
				if confidence == 0 {
					confidence = 1.0
				}

				entity := Entity{
					ID:         p.generateEntityID(name, entityType),
					Name:       name,
					EntityType: entityType,
					Confidence: confidence,
					Provenance: text[:min(100, len(text))],
					Timestamp:  timestamp,
				}

				if props, ok := entityMap["properties"].(map[string]interface{}); ok {
					entity.Properties = props
				}

				// Validate
				if p.validateEntity(entity) {
					result.Entities = append(result.Entities, entity)
				}
			}
		}
	}

	// Extract relations
	if relationsRaw, ok := rawResult["relations"].([]interface{}); ok {
		for _, relationRaw := range relationsRaw {
			if relationMap, ok := relationRaw.(map[string]interface{}); ok {
				fromEntity, _ := relationMap["from_entity"].(string)
				relationType, _ := relationMap["relation_type"].(string)
				toEntity, _ := relationMap["to_entity"].(string)
				confidence, _ := relationMap["confidence"].(float64)
				if confidence == 0 {
					confidence = 1.0
				}

				relation := Relation{
					ID:           p.generateRelationID(fromEntity, relationType, toEntity),
					FromEntity:   fromEntity,
					RelationType: relationType,
					ToEntity:     toEntity,
					Confidence:   confidence,
					Provenance:   text[:min(100, len(text))],
					Timestamp:    timestamp,
				}

				if props, ok := relationMap["properties"].(map[string]interface{}); ok {
					relation.Properties = props
				}

				// Validate
				if p.validateRelation(relation) {
					result.Relations = append(result.Relations, relation)
				}
			}
		}
	}

	// Extract assertions
	if assertionsRaw, ok := rawResult["assertions"].([]interface{}); ok {
		for _, assertionRaw := range assertionsRaw {
			if assertionMap, ok := assertionRaw.(map[string]interface{}); ok {
				subject, _ := assertionMap["subject"].(string)
				predicate, _ := assertionMap["predicate"].(string)
				object, _ := assertionMap["object"].(string)
				confidence, _ := assertionMap["confidence"].(float64)
				if confidence == 0 {
					confidence = 1.0
				}

				assertion := Assertion{
					ID:         p.generateAssertionID(subject, predicate, object),
					Subject:    subject,
					Predicate:  predicate,
					Object:     object,
					Confidence: confidence,
					Provenance: text[:min(100, len(text))],
					Timestamp:  timestamp,
				}

				// Validate
				if p.validateAssertion(assertion) {
					result.Assertions = append(result.Assertions, assertion)
				}
			}
		}
	}

	return result, nil
}

// ExtractAndCommit extracts and immediately commits
func (p *ExtractionPipeline) ExtractAndCommit(text string, extractor ExtractorFunction) (*ExtractionResult, error) {
	result, err := p.Extract(text, extractor)
	if err != nil {
		return nil, err
	}

	if err := p.Commit(result); err != nil {
		return nil, err
	}

	return result, nil
}

// Commit extraction result to database
func (p *ExtractionPipeline) Commit(result *ExtractionResult) error {
	// Store entities
	for _, entity := range result.Entities {
		key := append(p.prefix, []byte(fmt.Sprintf("entity:%s", entity.ID))...)
		data, err := json.Marshal(entity)
		if err != nil {
			return fmt.Errorf("failed to marshal entity: %w", err)
		}
		if err := p.db.Put(key, data); err != nil {
			return fmt.Errorf("failed to store entity: %w", err)
		}
	}

	// Store relations
	for _, relation := range result.Relations {
		key := append(p.prefix, []byte(fmt.Sprintf("relation:%s", relation.ID))...)
		data, err := json.Marshal(relation)
		if err != nil {
			return fmt.Errorf("failed to marshal relation: %w", err)
		}
		if err := p.db.Put(key, data); err != nil {
			return fmt.Errorf("failed to store relation: %w", err)
		}
	}

	// Store assertions
	for _, assertion := range result.Assertions {
		key := append(p.prefix, []byte(fmt.Sprintf("assertion:%s", assertion.ID))...)
		data, err := json.Marshal(assertion)
		if err != nil {
			return fmt.Errorf("failed to marshal assertion: %w", err)
		}
		if err := p.db.Put(key, data); err != nil {
			return fmt.Errorf("failed to store assertion: %w", err)
		}
	}

	return nil
}

// GetEntities retrieves all entities
func (p *ExtractionPipeline) GetEntities() ([]Entity, error) {
	entities := []Entity{}
	entityPrefix := append(p.prefix, []byte("entity:")...)

	txn := p.db.Begin()
	defer txn.Abort()

	iter := txn.ScanPrefix(entityPrefix)
	defer iter.Close()

	for {
		_, value, ok := iter.Next()
		if !ok {
			break
		}

		var entity Entity
		if err := json.Unmarshal(value, &entity); err != nil {
			continue
		}
		entities = append(entities, entity)
	}

	_ = txn.Commit()
	return entities, nil
}

// GetRelations retrieves all relations
func (p *ExtractionPipeline) GetRelations() ([]Relation, error) {
	relations := []Relation{}
	relationPrefix := append(p.prefix, []byte("relation:")...)

	txn := p.db.Begin()
	defer txn.Abort()

	iter := txn.ScanPrefix(relationPrefix)
	defer iter.Close()

	for {
		_, value, ok := iter.Next()
		if !ok {
			break
		}

		var relation Relation
		if err := json.Unmarshal(value, &relation); err != nil {
			continue
		}
		relations = append(relations, relation)
	}

	_ = txn.Commit()
	return relations, nil
}

// GetAssertions retrieves all assertions
func (p *ExtractionPipeline) GetAssertions() ([]Assertion, error) {
	assertions := []Assertion{}
	assertionPrefix := append(p.prefix, []byte("assertion:")...)

	txn := p.db.Begin()
	defer txn.Abort()

	iter := txn.ScanPrefix(assertionPrefix)
	defer iter.Close()

	for {
		_, value, ok := iter.Next()
		if !ok {
			break
		}

		var assertion Assertion
		if err := json.Unmarshal(value, &assertion); err != nil {
			continue
		}
		assertions = append(assertions, assertion)
	}

	_ = txn.Commit()
	return assertions, nil
}

// Validate entity
func (p *ExtractionPipeline) validateEntity(entity Entity) bool {
	if p.schema == nil {
		return true
	}

	// Check entity type
	if len(p.schema.EntityTypes) > 0 {
		found := false
		for _, t := range p.schema.EntityTypes {
			if t == entity.EntityType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check confidence
	if p.schema.MinConfidence > 0 && entity.Confidence < p.schema.MinConfidence {
		return false
	}

	return true
}

// Validate relation
func (p *ExtractionPipeline) validateRelation(relation Relation) bool {
	if p.schema == nil {
		return true
	}

	// Check relation type
	if len(p.schema.RelationTypes) > 0 {
		found := false
		for _, t := range p.schema.RelationTypes {
			if t == relation.RelationType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check confidence
	if p.schema.MinConfidence > 0 && relation.Confidence < p.schema.MinConfidence {
		return false
	}

	return true
}

// Validate assertion
func (p *ExtractionPipeline) validateAssertion(assertion Assertion) bool {
	if p.schema == nil {
		return true
	}

	// Check confidence
	if p.schema.MinConfidence > 0 && assertion.Confidence < p.schema.MinConfidence {
		return false
	}

	return true
}

// Generate deterministic entity ID
func (p *ExtractionPipeline) generateEntityID(name, entityType string) string {
	hash := sha256.Sum256([]byte(name + ":" + entityType))
	return hex.EncodeToString(hash[:])[:16]
}

// Generate deterministic relation ID
func (p *ExtractionPipeline) generateRelationID(from, relationType, to string) string {
	hash := sha256.Sum256([]byte(from + ":" + relationType + ":" + to))
	return hex.EncodeToString(hash[:])[:16]
}

// Generate deterministic assertion ID
func (p *ExtractionPipeline) generateAssertionID(subject, predicate, object string) string {
	hash := sha256.Sum256([]byte(subject + ":" + predicate + ":" + object))
	return hex.EncodeToString(hash[:])[:16]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
