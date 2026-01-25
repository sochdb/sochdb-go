package main

import (
	"fmt"
	"log"

	sochdb "github.com/sochdb/sochdb-go"
	"github.com/sochdb/sochdb-go/embedded"
)

func main() {
	fmt.Println("=== SochDB Memory System Example ===")
	fmt.Println()

	// Create database
	db, err := embedded.Open("/tmp/sochdb-memory-example")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	namespace := "memory-demo"

	// Example 1: Extraction Pipeline
	fmt.Println("1. Extraction Pipeline")
	fmt.Println("----------------------")

	schema := &sochdb.ExtractionSchema{
		EntityTypes:   []string{"Person", "Company"},
		RelationTypes: []string{"works_at", "founded_by", "acquired"},
	}

	pipeline := sochdb.NewExtractionPipeline(db, namespace, schema)

	// Simulate LLM extraction
	extractionResult := &sochdb.ExtractionResult{
		Entities: []sochdb.Entity{
			{
				Name:       "Alice Johnson",
				EntityType: "Person",
				Properties: map[string]interface{}{
					"age":        32,
					"occupation": "Software Engineer",
				},
			},
			{
				Name:       "TechCorp",
				EntityType: "Company",
				Properties: map[string]interface{}{
					"industry": "Technology",
					"founded":  2010,
				},
			},
		},
		Relations: []sochdb.Relation{
			{
				FromEntity:   "Alice Johnson",
				RelationType: "works_at",
				ToEntity:     "TechCorp",
				Properties: map[string]interface{}{
					"since": 2020,
					"role":  "Senior Engineer",
				},
			},
		},
		Assertions: []sochdb.Assertion{
			{
				Subject:   "Alice",
				Predicate: "has_experience",
				Object:    "10 years in software development",
			},
		},
	}

	if err := pipeline.Commit(extractionResult); err != nil {
		log.Fatalf("Failed to commit extraction: %v", err)
	}

	entities, err := pipeline.GetEntities()
	if err != nil {
		log.Fatalf("Failed to get entities: %v", err)
	}
	fmt.Printf("Extracted %d entities\n", len(entities))
	for _, entity := range entities {
		fmt.Printf("  - %s: %s\n", entity.EntityType, entity.Name)
	}

	relations, err := pipeline.GetRelations()
	if err != nil {
		log.Fatalf("Failed to get relations: %v", err)
	}
	fmt.Printf("Extracted %d relations\n", len(relations))
	for _, rel := range relations {
		fmt.Printf("  - %s -> %s (%s)\n", rel.FromEntity, rel.ToEntity, rel.RelationType)
	}

	// Example 2: Consolidation
	fmt.Println("\n2. Fact Consolidation")
	fmt.Println("---------------------")

	config := &sochdb.ConsolidationConfig{
		SimilarityThreshold: 0.85,
		UseTemporalUpdates:  true,
		MaxConflictAge:      86400,
	}

	consolidator := sochdb.NewConsolidator(db, namespace, config)

	// Add multiple assertions about the same fact
	assertions := []*sochdb.RawAssertion{
		{
			Fact: map[string]interface{}{
				"subject": "Alice Johnson",
				"claim":   "works at TechCorp",
			},
			Source:     "linkedin_profile",
			Confidence: 0.95,
		},
		{
			Fact: map[string]interface{}{
				"subject": "Alice Johnson",
				"claim":   "works at TechCorp",
			},
			Source:     "company_website",
			Confidence: 0.90,
		},
		{
			Fact: map[string]interface{}{
				"subject": "Alice Johnson",
				"claim":   "works at TechCorp",
			},
			Source:     "github_profile",
			Confidence: 0.85,
		},
	}

	for _, assertion := range assertions {
		if _, err := consolidator.Add(assertion); err != nil {
			log.Printf("Failed to add assertion: %v", err)
		}
	}

	updated, err := consolidator.Consolidate()
	if err != nil {
		log.Fatalf("Failed to consolidate: %v", err)
	}
	fmt.Printf("Consolidated %d facts\n", updated)

	canonicalFacts, err := consolidator.GetCanonicalFacts()
	if err != nil {
		log.Fatalf("Failed to get canonical facts: %v", err)
	}
	for _, fact := range canonicalFacts {
		fmt.Printf("  Fact: %v\n", fact.MergedFact)
		fmt.Printf("  Confidence: %.2f\n", fact.Confidence)
		fmt.Printf("  Sources: %d\n", len(fact.Sources))
	}

	// Example 3: Contradiction Handling
	fmt.Println("\n3. Contradiction Handling")
	fmt.Println("------------------------")

	oldAssertion := &sochdb.RawAssertion{
		Fact: map[string]interface{}{
			"subject": "Alice Johnson",
			"claim":   "age is 32",
		},
		Source:     "2023_profile",
		Confidence: 0.80,
	}
	oldID, _ := consolidator.Add(oldAssertion)

	newAssertion := &sochdb.RawAssertion{
		Fact: map[string]interface{}{
			"subject": "Alice Johnson",
			"claim":   "age is 33",
		},
		Source:     "2024_profile",
		Confidence: 0.95,
	}
	_, err = consolidator.AddWithContradiction(newAssertion, []string{oldID})
	if err != nil {
		log.Printf("Failed to add contradiction: %v", err)
	}

	updated, _ = consolidator.Consolidate()
	fmt.Printf("Updated %d facts after contradiction\n", updated)

	// Example 4: Hybrid Retrieval
	fmt.Println("\n4. Hybrid Retrieval (BM25 + Semantic)")
	fmt.Println("--------------------------------------")

	retrievalConfig := &sochdb.RetrievalConfig{
		Limit:          5,
		LexicalWeight:  0.3,
		SemanticWeight: 0.7,
		RRFConstant:    60,
	}

	retriever := sochdb.NewHybridRetriever(db, namespace, retrievalConfig)

	documents := map[string]map[string]interface{}{
		"doc1": {
			"text":     "Alice Johnson is a senior software engineer at TechCorp",
			"category": "profile",
		},
		"doc2": {
			"text":     "TechCorp is a leading technology company founded in 2010",
			"category": "company",
		},
		"doc3": {
			"text":     "Alice has extensive experience in distributed systems",
			"category": "skills",
		},
		"doc4": {
			"text":     "TechCorp specializes in cloud computing and AI",
			"category": "company",
		},
		"doc5": {
			"text":     "Alice graduated from MIT with a degree in computer science",
			"category": "education",
		},
	}

	if err := retriever.IndexDocuments(documents); err != nil {
		log.Fatalf("Failed to index documents: %v", err)
	}

	query := "Alice software engineer"
	allowed := &sochdb.AllAllowedSet{}

	results, err := retriever.Retrieve(query, allowed)
	if err != nil {
		log.Fatalf("Failed to retrieve: %v", err)
	}

	fmt.Printf("Query: %s\n", query)
	fmt.Printf("Found %d results:\n", len(results))
	for i, result := range results {
		fmt.Printf("  %d. %s (score: %.4f)\n", i+1, result["text"], result["_score"])
	}

	// Example 5: Pre-filtering
	fmt.Println("\n5. Pre-filtering with AllowedSet")
	fmt.Println("---------------------------------")

	// Only allow profile and skills categories
	filterAllowed := &sochdb.FilterAllowedSet{
		FilterFn: func(id string, doc map[string]interface{}) bool {
			if category, ok := doc["category"].(string); ok {
				return category == "profile" || category == "skills"
			}
			return false
		},
	}

	filteredResults, err := retriever.Retrieve(query, filterAllowed)
	if err != nil {
		log.Fatalf("Failed to retrieve with filter: %v", err)
	}

	fmt.Printf("Query: %s (filtered)\n", query)
	fmt.Printf("Found %d results:\n", len(filteredResults))
	for i, result := range filteredResults {
		fmt.Printf("  %d. %s (score: %.4f, category: %s)\n",
			i+1, result["text"], result["_score"], result["category"])
	}

	// Example 6: Explain retrieval
	fmt.Println("\n6. Explain Retrieval Scoring")
	fmt.Println("-----------------------------")

	explanation := retriever.Explain(query, "doc1")
	fmt.Printf("Explanation for doc1:\n")
	fmt.Printf("  Lexical score: %.4f\n", explanation["lexical_score"])
	fmt.Printf("  Semantic score: %.4f\n", explanation["semantic_score"])
	fmt.Printf("  Combined score: %.4f\n", explanation["combined_score"])

	fmt.Println("\n=== Memory System Example Complete ===")
}
