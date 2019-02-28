package graph

import (
	"github.com/chewxy/hm"
	"gorgonia.org/gorgonia/internal/execution"
	"gorgonia.org/gorgonia/internal/value"
	"gorgonia.org/tensor"
)

// A node is a node in the computation graph
type node struct {
	// metadata of the node
	T     hm.Type // pruned types only plz
	Shape tensor.Shape

	// this node is the result of applying the op to the children
	Op op.Op

	Name  string
	Group string

	// value bondage
	// inputs are bound to values directly
	BoundTo value.Value
	DataOn  execution.Device // where is the data on

	// to track derivations
	//DerivOf Nodes
	Deriv *node

	// for hashing nodes
	id int64 // id is the ID at which the node is added to the graph
}

// SetName of the node
func (n *node) SetName(name string) {
	n.Name = name
}

// GetName of the node
func (n *node) GetName() string {
	return n.Name
}

// ID fulfills the graph.Node interface
func (n *node) ID() int64 {
	return n.id
}
