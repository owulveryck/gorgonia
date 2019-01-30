package onnx

import (
	"gonum.org/v1/gonum/graph"
	"gorgonia.org/gorgonia/internal/engine"
)

// Maxpool operator ...
type Maxpool struct{}

// Constructor to fulfil the interface ...
func (r *Maxpool) Constructor() func(g graph.WeightedDirected, n graph.Node) (interface{}, error) {
	return func(g graph.WeightedDirected, n graph.Node) (interface{}, error) {
		return engine.NewMaxPool2DOperation(nil, nil, nil)(g, n.(*engine.Node))
	}
}
