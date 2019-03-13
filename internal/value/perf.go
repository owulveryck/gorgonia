package value

import (
	"sync"

	"github.com/chewxy/hm"
	"gorgonia.org/gorgonia/internal/value/factory"
	"gorgonia.org/tensor"
)

// handles Returning of value.Values

var dvpool = &sync.Pool{
	New: func() interface{} { return new(DualValue) },
}

// BorrowDV get a DualValue from the pool
func BorrowDV() *DualValue { return dvpool.Get().(*DualValue) }

// ReturnDV returns the DualValue to the pool
func ReturnDV(dv *DualValue) {
	returnValue(dv.D)
	returnValue(dv.Value)
	// if dvdT, ok := dv.d.(tensor.Tensor); ok {
	// 	returnTensor(dvdT)
	// }
	// if dvvT, ok := dv.Value.(tensor.Tensor); ok {
	// 	returnTensor(dvvT)
	// }

	dv.D = nil
	dv.Value = nil
	dvpool.Put(dv)
}
func returnValue(v Value) {
	if t, ok := v.(tensor.Tensor); ok {
		tensor.ReturnTensor(t)
		//	returnTensor(t)
	}
}

// ReturnType ...
func ReturnType(t hm.Type) {
	switch tt := t.(type) {
	case *factory.TensorType:
		factory.ReturnTensorType(tt)
	case factory.TensorType:
		// do nothing
	case tensor.Dtype:
		// do nothing
	case *hm.FunctionType:
		hm.ReturnFnType(tt)
	}
}
