package value

import (
	"gorgonia.org/tensor"
)

func tensorInfo(t tensor.Tensor) (tensor.Dtype, int) {
	return t.Dtype(), t.Dims()
}
