package gorgonia

import (
	"gorgonia.org/gorgonia/internal/exprgraph"
	"gorgonia.org/gorgonia/internal/value"
)

// Formula is the main structure. It's a wrapper around an ExprGraph internally
type Formula struct {
	g *exprgraph.ExprGraph
	v map[value.Value]int64
}

// graphBuilder
func (f *Formula) graphBuilder(childrenValues ...value.Value) (*exprgraph.Node, error) {
	output := f.g.NewVertex()
	f.g.AddNode(output)
	for i, val := range childrenValues {
		var (
			childID int64
			child   *exprgraph.Node
			ok      bool
		)
		// Is the value already part of the graph?
		if childID, ok := f.v[val]; !ok {
			// No, let's add it
			child = f.g.NewVertex()
			child.ApplyData(val)
			f.g.AddNode(child)
			childID = child.ID()
			f.v[val] = childID
		}
		// dummy check in order to avoid a function call
		if child == nil {
			child = f.g.Node(childID).(*exprgraph.Node)
		}
		f.g.SetWeightedEdge(f.g.NewWeightedEdge(output, child, float64(i)))
	}
	return output, nil
}
