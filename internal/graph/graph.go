package graph

import (
	"math"

	"gonum.org/v1/gonum/graph/simple"
)

// ExprGraph is a data structure for a directed acyclic graph (of expressions). This structure is the main entry point
// for Gorgonia.
type ExprGraph struct {
	w    *simple.WeightedDirectedGraph
	Name string
}

// NewGraph creates a new graph. Duh
func NewGraph() *ExprGraph {
	return &ExprGraph{
		w: simple.NewWeightedDirectedGraph(math.MaxFloat64, -1),
	}
}
