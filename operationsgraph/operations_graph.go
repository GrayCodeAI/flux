// Package graph provides graph-based execution graph implementation for eyrie.
package operationsgraph

import (
	"fmt"
	"sync"
	"time"

	graphcontracts "github.com/GrayCodeAI/hawk-core-contracts/graph"
)

// OperationNode represents a node in the operations graph.
type OperationNode struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // "agent", "tool", "function", "start", "end"
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Handler     string                 `json:"handler,omitempty"`
	Attrs       map[string]interface{} `json:"attrs,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// OperationEdge represents an edge in the operations graph.
type OperationEdge struct {
	From      string                 `json:"from"`
	To        string                 `json:"to"`
	Condition string                 `json:"condition,omitempty"`
	Weight    float64                `json:"weight"`
	Attrs     map[string]interface{} `json:"attrs,omitempty"`
}

// OperationsGraph represents a graph of operations for eyrie.
type OperationsGraph struct {
	mu    sync.RWMutex
	ID    string                `json:"id"`
	Name  string                `json:"name"`
	Nodes map[string]*OperationNode `json:"nodes"`
	Edges []OperationEdge       `json:"edges"`
	Attrs map[string]interface{} `json:"attrs,omitempty"`
}

// NewOperationsGraph creates a new operations graph.
func NewOperationsGraph(id, name string) *OperationsGraph {
	return &OperationsGraph{
		ID:    id,
		Name:  name,
		Nodes: make(map[string]*OperationNode),
		Attrs: make(map[string]interface{}),
	}
}

// AddNode adds a node to the graph.
func (g *OperationsGraph) AddNode(node *OperationNode) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = time.Now()
	}
	node.UpdatedAt = time.Now()
	g.Nodes[node.ID] = node
}

// AddEdge adds an edge to the graph.
func (g *OperationsGraph) AddEdge(edge OperationEdge) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Edges = append(g.Edges, edge)
}

// GetNode retrieves a node by ID.
func (g *OperationsGraph) GetNode(id string) (*OperationNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	node, ok := g.Nodes[id]
	return node, ok
}

// GetNodes returns all nodes.
func (g *OperationsGraph) GetNodes() []*OperationNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	result := make([]*OperationNode, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		result = append(result, node)
	}
	return result
}

// GetEdges returns all edges.
func (g *OperationsGraph) GetEdges() []OperationEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.Edges
}

// FindByType finds all nodes of a specific type.
func (g *OperationsGraph) FindByType(nodeType string) []*OperationNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	result := []*OperationNode{}
	for _, node := range g.Nodes {
		if node.Type == nodeType {
			result = append(result, node)
		}
	}
	return result
}

// ToGraphSpec converts the operations graph to a portable graph spec.
func (g *OperationsGraph) ToGraphSpec() *graphcontracts.GraphSpec {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]graphcontracts.NodeSpec, 0, len(g.Nodes))
	for id, node := range g.Nodes {
		config := make(map[string]string)
		config["type"] = node.Type
		config["name"] = node.Name
		if node.Handler != "" {
			config["handler"] = node.Handler
		}
		for k, v := range node.Attrs {
			config[k] = fmt.Sprintf("%v", v)
		}

		nodes = append(nodes, graphcontracts.NodeSpec{
			ID:     id,
			Type:   graphcontracts.NodeTypeOperations,
			Name:   node.Name,
			Config: config,
		})
	}

	edges := make([]graphcontracts.EdgeSpec, 0, len(g.Edges))
	for _, edge := range g.Edges {
		edges = append(edges, graphcontracts.EdgeSpec{
			From:   edge.From,
			To:     edge.To,
			Weight: edge.Weight,
		})
	}

	return &graphcontracts.GraphSpec{
		ID:     g.ID,
		Name:   g.Name,
		Nodes:  nodes,
		Edges:  edges,
	}
}
