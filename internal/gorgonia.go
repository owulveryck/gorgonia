package gorgonia

import (
	"gorgonia.org/gorgonia/internal/value"
)

// Mul ...
func (g *Graph) Mul(a, b value.Value) (value.Value, error) {
	output, err := g.graphBuilder(a, b)
	if err != nil {
		return nil, err
	}
	err = g.g.ApplyOp(ops.mulOp, output)
	return output.Value(), nil
}
