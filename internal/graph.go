package gorgonia

import (
	ggraph "gorgonia.org/gorgonia/internal/graph"
	"gorgonia.org/gorgonia/internal/value"
)

// Graph is the main structure
type Graph struct {
	g *ggraph.ExprGraph
	v map[value.Value]int64
}

// graphBuilder ...
func (g *Graph) graphBuilder(childrenValues ...value.Value) (*ggraph.Node, error) {
	output := g.g.NewVertex()
	g.g.AddNode(output)
	for i, val := range childrenValues {
		var (
			childID int64
			child   *ggraph.Node
			ok      bool
		)
		// Is the value already part of the graph?
		if childID, ok := g.v[val]; !ok {
			// No, let's add it
			child = g.g.NewVertex()
			child.ApplyData(val)
			g.g.AddNode(child)
			childID = child.ID()
			g.v[val] = childID
		}
		// dummy check in order to avoid a function call
		if child == nil {
			child = g.g.Node(childID).(*ggraph.Node)
		}
		g.g.SetWeightedEdge(g.g.NewWeightedEdge(output, child, float64(i)))
	}
	return output, nil
}
