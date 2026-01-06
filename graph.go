// Copyright 2025 Sushanth (https://github.com/sushanthpy)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Package toondb provides a semi-GraphDB overlay for agent memory.

This module provides a lightweight graph layer on top of ToonDB's KV storage
for modeling agent memory relationships:

  - Entity-to-entity relationships (user <-> conversation <-> message)
  - Causal chains (action1 -> action2 -> action3)
  - Reference graphs (document <- citation <- quote)

Storage Model:

	Nodes: _graph/{namespace}/nodes/{node_id} -> {type, properties}
	Edges: _graph/{namespace}/edges/{from_id}/{edge_type}/{to_id} -> {properties}
	Index: _graph/{namespace}/index/{edge_type}/{to_id} -> [from_ids] (reverse lookup)

Example:

	graph := toondb.NewGraphOverlay(db, "agent_001")

	graph.AddNode("user_1", "User", map[string]interface{}{"name": "Alice"})
	graph.AddNode("conv_1", "Conversation", map[string]interface{}{"title": "Planning"})

	graph.AddEdge("user_1", "STARTED", "conv_1", nil)

	path := graph.ShortestPath("user_1", "msg_1", 10, nil)
*/
package toondb

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TraversalOrder specifies graph traversal order.
type TraversalOrder string

const (
	TraversalBFS TraversalOrder = "bfs"
	TraversalDFS TraversalOrder = "dfs"
)

// GraphNode represents a node in the graph.
type GraphNode struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
}

// GraphEdge represents an edge in the graph.
type GraphEdge struct {
	FromID     string                 `json:"from_id"`
	EdgeType   string                 `json:"edge_type"`
	ToID       string                 `json:"to_id"`
	Properties map[string]interface{} `json:"properties"`
}

// GraphOverlay provides a lightweight graph layer on ToonDB.
type GraphOverlay struct {
	db        *Database
	namespace string
	prefix    string
}

// NewGraphOverlay creates a new graph overlay.
func NewGraphOverlay(db *Database, namespace string) *GraphOverlay {
	return &GraphOverlay{
		db:        db,
		namespace: namespace,
		prefix:    fmt.Sprintf("_graph/%s", namespace),
	}
}

// Key helpers
func (g *GraphOverlay) nodeKey(nodeID string) string {
	return fmt.Sprintf("%s/nodes/%s", g.prefix, nodeID)
}

func (g *GraphOverlay) edgeKey(fromID, edgeType, toID string) string {
	return fmt.Sprintf("%s/edges/%s/%s/%s", g.prefix, fromID, edgeType, toID)
}

func (g *GraphOverlay) edgePrefix(fromID string, edgeType string) string {
	if edgeType != "" {
		return fmt.Sprintf("%s/edges/%s/%s/", g.prefix, fromID, edgeType)
	}
	return fmt.Sprintf("%s/edges/%s/", g.prefix, fromID)
}

func (g *GraphOverlay) reverseIndexKey(edgeType, toID, fromID string) string {
	return fmt.Sprintf("%s/index/%s/%s/%s", g.prefix, edgeType, toID, fromID)
}

func (g *GraphOverlay) reverseIndexPrefix(edgeType, toID string) string {
	return fmt.Sprintf("%s/index/%s/%s/", g.prefix, edgeType, toID)
}

// =============================================================================
// Node Operations
// =============================================================================

// AddNode adds a node to the graph.
func (g *GraphOverlay) AddNode(nodeID, nodeType string, properties map[string]interface{}) (*GraphNode, error) {
	if properties == nil {
		properties = make(map[string]interface{})
	}

	node := &GraphNode{
		ID:         nodeID,
		Type:       nodeType,
		Properties: properties,
	}

	data, err := json.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal node: %w", err)
	}

	if err := g.db.Put(g.nodeKey(nodeID), data); err != nil {
		return nil, err
	}

	return node, nil
}

// GetNode retrieves a node by ID.
func (g *GraphOverlay) GetNode(nodeID string) (*GraphNode, error) {
	data, err := g.db.Get(g.nodeKey(nodeID))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	var node GraphNode
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("failed to unmarshal node: %w", err)
	}

	return &node, nil
}

// UpdateNode updates a node's properties or type.
func (g *GraphOverlay) UpdateNode(nodeID string, properties map[string]interface{}, nodeType string) (*GraphNode, error) {
	node, err := g.GetNode(nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}

	if properties != nil {
		for k, v := range properties {
			node.Properties[k] = v
		}
	}
	if nodeType != "" {
		node.Type = nodeType
	}

	data, err := json.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal node: %w", err)
	}

	if err := g.db.Put(g.nodeKey(nodeID), data); err != nil {
		return nil, err
	}

	return node, nil
}

// DeleteNode deletes a node from the graph.
func (g *GraphOverlay) DeleteNode(nodeID string, cascade bool) (bool, error) {
	node, err := g.GetNode(nodeID)
	if err != nil {
		return false, err
	}
	if node == nil {
		return false, nil
	}

	if cascade {
		// Delete outgoing edges
		edges, err := g.GetEdges(nodeID, "")
		if err != nil {
			return false, err
		}
		for _, edge := range edges {
			if _, err := g.DeleteEdge(nodeID, edge.EdgeType, edge.ToID); err != nil {
				return false, err
			}
		}

		// Delete incoming edges
		inEdges, err := g.GetIncomingEdges(nodeID, "")
		if err != nil {
			return false, err
		}
		for _, edge := range inEdges {
			if _, err := g.DeleteEdge(edge.FromID, edge.EdgeType, nodeID); err != nil {
				return false, err
			}
		}
	}

	if err := g.db.Delete(g.nodeKey(nodeID)); err != nil {
		return false, err
	}

	return true, nil
}

// NodeExists checks if a node exists.
func (g *GraphOverlay) NodeExists(nodeID string) (bool, error) {
	data, err := g.db.Get(g.nodeKey(nodeID))
	if err != nil {
		return false, err
	}
	return data != nil, nil
}

// =============================================================================
// Edge Operations
// =============================================================================

// AddEdge adds an edge between two nodes.
func (g *GraphOverlay) AddEdge(fromID, edgeType, toID string, properties map[string]interface{}) (*GraphEdge, error) {
	if properties == nil {
		properties = make(map[string]interface{})
	}

	edge := &GraphEdge{
		FromID:     fromID,
		EdgeType:   edgeType,
		ToID:       toID,
		Properties: properties,
	}

	data, err := json.Marshal(edge)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal edge: %w", err)
	}

	// Store edge
	if err := g.db.Put(g.edgeKey(fromID, edgeType, toID), data); err != nil {
		return nil, err
	}

	// Store reverse index
	if err := g.db.Put(g.reverseIndexKey(edgeType, toID, fromID), []byte(fromID)); err != nil {
		return nil, err
	}

	return edge, nil
}

// GetEdge retrieves a specific edge.
func (g *GraphOverlay) GetEdge(fromID, edgeType, toID string) (*GraphEdge, error) {
	data, err := g.db.Get(g.edgeKey(fromID, edgeType, toID))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	var edge GraphEdge
	if err := json.Unmarshal(data, &edge); err != nil {
		return nil, fmt.Errorf("failed to unmarshal edge: %w", err)
	}

	return &edge, nil
}

// GetEdges retrieves all outgoing edges from a node.
func (g *GraphOverlay) GetEdges(fromID, edgeType string) ([]*GraphEdge, error) {
	prefix := g.edgePrefix(fromID, edgeType)
	results, err := g.db.ScanPrefix(prefix)
	if err != nil {
		return nil, err
	}

	edges := make([]*GraphEdge, 0, len(results))
	for _, result := range results {
		var edge GraphEdge
		if err := json.Unmarshal(result.Value, &edge); err != nil {
			continue
		}
		edges = append(edges, &edge)
	}

	return edges, nil
}

// GetIncomingEdges retrieves all incoming edges to a node.
func (g *GraphOverlay) GetIncomingEdges(toID, edgeType string) ([]*GraphEdge, error) {
	edges := make([]*GraphEdge, 0)

	if edgeType != "" {
		// Query specific edge type
		prefix := g.reverseIndexPrefix(edgeType, toID)
		results, err := g.db.ScanPrefix(prefix)
		if err != nil {
			return nil, err
		}

		for _, result := range results {
			fromID := string(result.Value)
			edge, err := g.GetEdge(fromID, edgeType, toID)
			if err != nil {
				continue
			}
			if edge != nil {
				edges = append(edges, edge)
			}
		}
	} else {
		// Query all edge types - scan all index entries
		indexPrefix := fmt.Sprintf("%s/index/", g.prefix)
		results, err := g.db.ScanPrefix(indexPrefix)
		if err != nil {
			return nil, err
		}

		for _, result := range results {
			parts := strings.Split(result.Key, "/")
			if len(parts) >= 6 && parts[4] == toID {
				fromID := string(result.Value)
				et := parts[3]
				edge, err := g.GetEdge(fromID, et, toID)
				if err != nil {
					continue
				}
				if edge != nil {
					edges = append(edges, edge)
				}
			}
		}
	}

	return edges, nil
}

// DeleteEdge deletes an edge.
func (g *GraphOverlay) DeleteEdge(fromID, edgeType, toID string) (bool, error) {
	edge, err := g.GetEdge(fromID, edgeType, toID)
	if err != nil {
		return false, err
	}
	if edge == nil {
		return false, nil
	}

	// Delete edge
	if err := g.db.Delete(g.edgeKey(fromID, edgeType, toID)); err != nil {
		return false, err
	}

	// Delete reverse index
	if err := g.db.Delete(g.reverseIndexKey(edgeType, toID, fromID)); err != nil {
		return false, err
	}

	return true, nil
}

// =============================================================================
// Traversal Operations
// =============================================================================

// BFS performs breadth-first search from a starting node.
func (g *GraphOverlay) BFS(startID string, maxDepth int, edgeTypes, nodeTypes []string) ([]string, error) {
	return g.traverse(startID, maxDepth, edgeTypes, nodeTypes, TraversalBFS)
}

// DFS performs depth-first search from a starting node.
func (g *GraphOverlay) DFS(startID string, maxDepth int, edgeTypes, nodeTypes []string) ([]string, error) {
	return g.traverse(startID, maxDepth, edgeTypes, nodeTypes, TraversalDFS)
}

func (g *GraphOverlay) traverse(startID string, maxDepth int, edgeTypes, nodeTypes []string, order TraversalOrder) ([]string, error) {
	visited := make(map[string]bool)
	result := make([]string, 0)

	type item struct {
		nodeID string
		depth  int
	}

	frontier := []item{{startID, 0}}
	edgeTypeSet := make(map[string]bool)
	for _, et := range edgeTypes {
		edgeTypeSet[et] = true
	}
	nodeTypeSet := make(map[string]bool)
	for _, nt := range nodeTypes {
		nodeTypeSet[nt] = true
	}

	for len(frontier) > 0 {
		var current item
		if order == TraversalBFS {
			current = frontier[0]
			frontier = frontier[1:]
		} else {
			current = frontier[len(frontier)-1]
			frontier = frontier[:len(frontier)-1]
		}

		if visited[current.nodeID] {
			continue
		}
		visited[current.nodeID] = true

		// Check node type filter
		if len(nodeTypes) > 0 {
			node, err := g.GetNode(current.nodeID)
			if err != nil {
				continue
			}
			if node == nil || !nodeTypeSet[node.Type] {
				continue
			}
		}

		result = append(result, current.nodeID)

		if current.depth >= maxDepth {
			continue
		}

		// Get outgoing edges
		edges, err := g.GetEdges(current.nodeID, "")
		if err != nil {
			continue
		}

		for _, edge := range edges {
			if len(edgeTypes) > 0 && !edgeTypeSet[edge.EdgeType] {
				continue
			}
			if !visited[edge.ToID] {
				frontier = append(frontier, item{edge.ToID, current.depth + 1})
			}
		}
	}

	return result, nil
}

// ShortestPath finds the shortest path between two nodes using BFS.
func (g *GraphOverlay) ShortestPath(fromID, toID string, maxDepth int, edgeTypes []string) ([]string, error) {
	if fromID == toID {
		return []string{fromID}, nil
	}

	visited := map[string]bool{fromID: true}
	parent := make(map[string]string)

	type item struct {
		nodeID string
		depth  int
	}

	frontier := []item{{fromID, 0}}
	edgeTypeSet := make(map[string]bool)
	for _, et := range edgeTypes {
		edgeTypeSet[et] = true
	}

	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]

		if current.depth >= maxDepth {
			continue
		}

		edges, err := g.GetEdges(current.nodeID, "")
		if err != nil {
			continue
		}

		for _, edge := range edges {
			if len(edgeTypes) > 0 && !edgeTypeSet[edge.EdgeType] {
				continue
			}

			nextID := edge.ToID
			if visited[nextID] {
				continue
			}

			visited[nextID] = true
			parent[nextID] = current.nodeID

			if nextID == toID {
				// Reconstruct path
				path := []string{toID}
				curr := toID
				for {
					p, ok := parent[curr]
					if !ok {
						break
					}
					path = append([]string{p}, path...)
					curr = p
				}
				return path, nil
			}

			frontier = append(frontier, item{nextID, current.depth + 1})
		}
	}

	return nil, nil // No path found
}

// =============================================================================
// Query Operations
// =============================================================================

// EdgeDirection specifies edge direction for neighbor queries.
type EdgeDirection string

const (
	EdgeOutgoing EdgeDirection = "outgoing"
	EdgeIncoming EdgeDirection = "incoming"
	EdgeBoth     EdgeDirection = "both"
)

// Neighbor represents a neighboring node with its connecting edge.
type Neighbor struct {
	NodeID string
	Edge   *GraphEdge
}

// GetNeighbors retrieves neighboring nodes with their connecting edges.
func (g *GraphOverlay) GetNeighbors(nodeID string, edgeTypes []string, direction EdgeDirection) ([]Neighbor, error) {
	neighbors := make([]Neighbor, 0)
	edgeTypeSet := make(map[string]bool)
	for _, et := range edgeTypes {
		edgeTypeSet[et] = true
	}

	if direction == EdgeOutgoing || direction == EdgeBoth {
		edges, err := g.GetEdges(nodeID, "")
		if err != nil {
			return nil, err
		}
		for _, edge := range edges {
			if len(edgeTypes) > 0 && !edgeTypeSet[edge.EdgeType] {
				continue
			}
			neighbors = append(neighbors, Neighbor{edge.ToID, edge})
		}
	}

	if direction == EdgeIncoming || direction == EdgeBoth {
		edges, err := g.GetIncomingEdges(nodeID, "")
		if err != nil {
			return nil, err
		}
		for _, edge := range edges {
			if len(edgeTypes) > 0 && !edgeTypeSet[edge.EdgeType] {
				continue
			}
			neighbors = append(neighbors, Neighbor{edge.FromID, edge})
		}
	}

	return neighbors, nil
}

// GetNodesByType retrieves all nodes of a specific type.
func (g *GraphOverlay) GetNodesByType(nodeType string, limit int) ([]*GraphNode, error) {
	prefix := fmt.Sprintf("%s/nodes/", g.prefix)
	results, err := g.db.ScanPrefix(prefix)
	if err != nil {
		return nil, err
	}

	nodes := make([]*GraphNode, 0)
	for _, result := range results {
		var node GraphNode
		if err := json.Unmarshal(result.Value, &node); err != nil {
			continue
		}
		if node.Type == nodeType {
			nodes = append(nodes, &node)
			if limit > 0 && len(nodes) >= limit {
				break
			}
		}
	}

	return nodes, nil
}

// GetSubgraph retrieves a subgraph starting from a node.
func (g *GraphOverlay) GetSubgraph(startID string, maxDepth int, edgeTypes []string) (*Subgraph, error) {
	nodeIDs, err := g.BFS(startID, maxDepth, edgeTypes, nil)
	if err != nil {
		return nil, err
	}

	nodes := make(map[string]*GraphNode)
	edges := make([]*GraphEdge, 0)

	for _, nodeID := range nodeIDs {
		node, err := g.GetNode(nodeID)
		if err != nil {
			continue
		}
		if node != nil {
			nodes[nodeID] = node
		}

		nodeEdges, err := g.GetEdges(nodeID, "")
		if err != nil {
			continue
		}
		for _, edge := range nodeEdges {
			// Only include edges where both nodes are in the subgraph
			if _, ok := nodes[edge.ToID]; ok {
				edges = append(edges, edge)
			}
		}
	}

	return &Subgraph{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// Subgraph represents a subset of the graph.
type Subgraph struct {
	Nodes map[string]*GraphNode
	Edges []*GraphEdge
}
