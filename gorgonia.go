package gorgonia

import (
	"gorgonia.org/gorgonia/internal/value"
)

// Mul ...
func (f *Formula) Mul(a, b value.Value) (value.Value, error) {
	output, err := f.graphBuilder(a, b)
	if err != nil {
		return nil, err
	}
	err = f.g.ApplyOp(ops.mulOp, output)
	return output.Value(), nil
}
