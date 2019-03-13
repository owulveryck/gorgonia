package exprgraph

import (
	"math"
	"testing"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/testgraph"
)

func TestGraph_AddNode(t *testing.T) {
	g := NewGraph()
	// Simple test
	testgraph.AddNodes(t, g, 1000)
}

func TestGraph_WeightedEdges(t *testing.T) {
	for _, w := range []float64{-1, 0, math.MaxFloat64} {
		g := NewGraph()
		testgraph.AddWeightedEdges(t, 500, g, w, func(id int64) graph.Node {
			return &node{
				id: id,
			}
		}, true, true)
	}

}
