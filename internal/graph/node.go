package graph

import (
	"github.com/chewxy/hm"
	"gorgonia.org/gorgonia/internal/execution"
	"gorgonia.org/gorgonia/internal/graph/op"
	"gorgonia.org/gorgonia/internal/value"
	"gorgonia.org/tensor"
)

// A Node is a node in the computation graph
type Node struct {
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
	Deriv *Node

	// for hashing nodes
	id int64 // id is the ID at which the node is added to the graph
}

// Value returns the valuse bound to the node. May return nil
func (n *Node) Value() value.Value {
	if dv, ok := n.BoundTo.(*value.DualValue); ok {
		return dv.Value
	}
	return n.BoundTo
}

// SetName of the node
func (n *Node) SetName(name string) {
	n.Name = name
}

// GetName of the node
func (n *Node) GetName() string {
	return n.Name
}

// ID fulfills the graph.Node interface
func (n *Node) ID() int64 {
	return n.id
}

// ApplyData v to current node (somewhat similar to NodeFromAny)
// TODO: Test that
func (n *Node) ApplyData(v value.Value) error {
	n.T = v.Dtype()
	n.Shape = v.Shape()
	n.BoundTo = v
	return nil
}
