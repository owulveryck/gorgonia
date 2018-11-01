package gorgonia

import (
	"fmt"
	"hash"
	"time"

	"github.com/chewxy/hm"
	rng "github.com/leesper/go_rng"
	"github.com/pkg/errors"
	"gonum.org/v1/gonum/blas"
	"gorgonia.org/tensor"
	"gorgonia.org/vecf32"
	"gorgonia.org/vecf64"
)

var (
	_ SDOp = im2colOp{}
	_ Op   = col2imOp{}
	_ Op   = &maxPoolOp{}
	_ Op   = &maxPoolDiffOp{}
	_ Op   = &BatchNormOp{}
	_ Op   = &batchnormDiffOp{}
)

/*
	This file contains all the Ops related to building a neural network.

	Bear in mind that not all things that are related to a neural network are here, as not everything
	are encoded as Ops the way theano does it.

	See also: nn.go for functions that relate to neural networks
*/

type randomness byte

const (
	uniform randomness = iota
	gaussian
	binomial
)

type randomOp struct {
	which randomness
	shape tensor.Shape
	dt    tensor.Dtype

	a, b float64 // when uniform, a,b = low, high; when gaussian, a,b = mean, stdev
}

func makeRandomOp(which randomness, dt tensor.Dtype, a, b float64, shape ...int) randomOp {
	return randomOp{
		which: which,
		shape: tensor.Shape(shape),
		dt:    dt,
		a:     a,
		b:     b,
	}
}

func (op randomOp) Arity() int { return 0 }

// randomOp :: a
// randomOp :: Tensor a
func (op randomOp) Type() hm.Type {
	if op.shape.IsScalar() {
		return op.dt
	}
	tt := newTensorType(op.shape.Dims(), op.dt)
	return tt
}

func (op randomOp) InferShape(...DimSizer) (tensor.Shape, error) { return op.shape, nil }

func (op randomOp) Do(...Value) (retVal Value, err error) {
	if op.shape.IsScalar() {
		var v interface{}
		switch op.dt {
		case Float64:
			switch op.which {
			case uniform:
				rand := rng.NewUniformGenerator(time.Now().UnixNano())
				v = rand.Float64Range(op.a, op.b)
			case gaussian:
				rand := rng.NewGaussianGenerator(time.Now().UnixNano())
				v = rand.Gaussian(op.a, op.b)
			case binomial:
				rand := rng.NewBinomialGenerator(time.Now().UnixNano())
				v = float64(rand.Binomial(int64(op.a), op.b))
			}
		case Float32:
			switch op.which {
			case uniform:
				rand := rng.NewUniformGenerator(time.Now().UnixNano())
				v = rand.Float32Range(float32(op.a), float32(op.b))
			case gaussian:
				rand := rng.NewGaussianGenerator(time.Now().UnixNano())
				v = float32(rand.Gaussian(op.a, op.b))
			case binomial:
				rand := rng.NewBinomialGenerator(time.Now().UnixNano())
				v = float32(rand.Binomial(int64(op.a), op.b))
			}
		default:
			return nil, errors.Errorf(nyiFail, "randomOp.do()", op.dt)
		}

		retVal, _ = anyToScalar(v)
		return
	}

	switch op.dt {
	case Float64:
		switch op.which {
		case uniform:
			backing := Uniform64(op.a, op.b, op.shape...)
			retVal = tensor.New(tensor.WithBacking(backing), tensor.WithShape(op.shape...))
		case gaussian:
			backing := Gaussian64(op.a, op.b, op.shape...)
			retVal = tensor.New(tensor.WithBacking(backing), tensor.WithShape(op.shape...))
		case binomial:
			backing := Binomial64(op.a, op.b, op.shape...)
			retVal = tensor.New(tensor.WithBacking(backing), tensor.WithShape(op.shape...))
		}
		return
	case Float32:
		switch op.which {
		case uniform:
			backing := Uniform32(op.a, op.b, op.shape...)
			retVal = tensor.New(tensor.WithBacking(backing), tensor.WithShape(op.shape...))
		case gaussian:
			backing := Gaussian32(op.a, op.b, op.shape...)
			retVal = tensor.New(tensor.WithBacking(backing), tensor.WithShape(op.shape...))
		case binomial:
			backing := Binomial32(op.a, op.b, op.shape...)
			retVal = tensor.New(tensor.WithBacking(backing), tensor.WithShape(op.shape...))
		}
		return
	default:
		return nil, errors.Errorf(nyiFail, "randomOp.do() for non-scalar", op.dt)
	}
}

func (op randomOp) ReturnsPtr() bool     { return false }
func (op randomOp) CallsExtern() bool    { return false }
func (op randomOp) OverwritesInput() int { return -1 }
func (op randomOp) WriteHash(h hash.Hash) {
	fmt.Fprintf(h, "%d%v%f%f", op.which, op.shape, op.a, op.b)
}

func (op randomOp) Hashcode() uint32 { return simpleHash(op) }

func (op randomOp) String() string {
	return fmt.Sprintf("%v(%v, %v) - %v", op.which, op.a, op.b, op.shape)
}

type im2colOp struct {
	h, w                 int // kernel height and width
	padH, padW           int
	strideH, strideW     int
	dilationH, dilationW int
}

func makeIm2ColOp(kernelHeight, kernelWidth, padHeight, padWidth, strideHeight, strideWidth, dilationHeight, dilationWidth int) im2colOp {
	return im2colOp{
		h:         kernelHeight,
		w:         kernelWidth,
		padH:      padHeight,
		padW:      padWidth,
		strideH:   strideHeight,
		strideW:   strideWidth,
		dilationH: dilationHeight,
		dilationW: dilationWidth,
	}
}

func (op im2colOp) Arity() int { return 1 }

// im2col :: (Floats a) ⇒ Tensor a →  Tensor a
func (op im2colOp) Type() hm.Type {
	t := makeTensorType(4, hm.TypeVariable('a'))
	return hm.NewFnType(t, t)
}

func (op im2colOp) InferShape(shapes ...DimSizer) (retVal tensor.Shape, err error) {
	if err = checkArity(op, len(shapes)); err != nil {
		return
	}

	if s, ok := shapes[0].(tensor.Shape); ok {
		return op.calcShape(s), nil
	}
	return nil, errors.Errorf("expected tensor.Shape. got %T instead", shapes[0])
}

func (op im2colOp) Do(inputs ...Value) (retVal Value, err error) {
	if err = checkArity(op, len(inputs)); err != nil {
		return
	}

	im := inputs[0]

	// todo type check values
	// todo shape check values

	retShape := op.calcShape(im.Shape())
	prealloc := tensor.New(tensor.Of(im.Dtype()), tensor.WithShape(retShape...))

	return op.do(prealloc, im)
}

func (op im2colOp) ReturnsPtr() bool     { return false }
func (op im2colOp) CallsExtern() bool    { return false }
func (op im2colOp) OverwritesInput() int { return -1 }

func (op im2colOp) WriteHash(h hash.Hash) {
	fmt.Fprintf(h, "im2col:%d-%d-%d-%d-%d-%d", op.h, op.w, op.padH, op.padW, op.strideH, op.strideW)
}

func (op im2colOp) Hashcode() uint32 { return simpleHash(op) }

func (op im2colOp) String() string {
	return fmt.Sprintf("im2col<(%d,%d), (%d, %d), (%d,%d) (%d, %d)>", op.h, op.w, op.padH, op.padW, op.strideH, op.strideW, op.dilationH, op.dilationW)
}

func (op im2colOp) DiffWRT(i int) []bool { return []bool{true} }

func (op im2colOp) SymDiff(inputs Nodes, output, grad *Node) (retVal Nodes, err error) {
	if err = checkArity(op, len(inputs)); err != nil {
		return
	}
	im := inputs[0]
	s := im.Shape()
	if s.Dims() != 4 {
		return nil, errors.Errorf("Expected input to have a shape with 4 dims")
	}
	var unpaddedB, unpaddedC, unpaddedH, unpaddedW int
	unpaddedB, unpaddedC, unpaddedH, unpaddedW = s[0], s[1], s[2], s[3]
	diffOp := col2imOp{
		unpaddedB: unpaddedB,
		unpaddedC: unpaddedC,
		unpaddedH: unpaddedH,
		unpaddedW: unpaddedW,

		im2colOp: op,
	}

	var ret *Node
	if ret, err = ApplyOp(diffOp, grad); err != nil {
		return
	}
	retVal = Nodes{ret}
	return
}

func (op im2colOp) DoDiff(ctx ExecutionContext, inputs Nodes, output *Node) (err error) {
	if err = checkArity(op, len(inputs)); err != nil {
		return
	}

	im := inputs[0]
	s := im.Shape()
	imv, colv := getDV(im, output)

	var unpaddedB, unpaddedC, unpaddedH, unpaddedW int
	unpaddedB, unpaddedC, unpaddedH, unpaddedW = s[0], s[1], s[2], s[3]
	diffOp := col2imOp{
		unpaddedB: unpaddedB,
		unpaddedC: unpaddedC,
		unpaddedH: unpaddedH,
		unpaddedW: unpaddedW,

		im2colOp: op,
	}

	if _, err = diffOp.UsePreallocDo(imv.d, colv.d); err != nil {
		return errors.Wrapf(err, doFail, diffOp)
	}
	return
}

func (op im2colOp) calcShape(s tensor.Shape) (retVal tensor.Shape) {
	b := s[0]
	c := s[1]
	h := s[2]
	w := s[3]

	retHeight, retWidth := op.retHW(h, w)
	retVal = tensor.Shape(tensor.BorrowInts(4))

	// todo: double check this with tests
	retVal[0] = b
	retVal[1] = retHeight
	retVal[2] = retWidth
	retVal[3] = c * op.w * op.h

	return
}

func (op im2colOp) retHW(h, w int) (retHeight, retWidth int) {
	retHeight = (h+2*op.padH-(op.dilationH*(op.h-1)+1))/op.strideH + 1
	retWidth = (w+2*op.padW-(op.dilationW*(op.w-1)+1))/op.strideW + 1
	return
}

func (op im2colOp) do(prealloc, input Value) (retVal Value, err error) {
	inputT := input.(*tensor.Dense)
	outputT := prealloc.(*tensor.Dense)

	// extract bchw - this bit can be expanded in the future, but for now we only support bchw
	s := inputT.Shape()
	b := s[0]
	c := s[1]
	h := s[2]
	w := s[3]

	inputStrides := inputT.Strides()
	retHeight, retWidth := op.retHW(h, w)
	batchStrideIm := inputStrides[0]
	batchStrideCol := outputT.Strides()[0]
	chanStride := h * w
	inRowStride := inputStrides[2]

	var imEnd, colEnd int
	switch input.Dtype() {
	case tensor.Float64:
		imData := input.Data().([]float64)
		colData := prealloc.Data().([]float64)
		for i := 0; i < b; i++ {
			imStart := i * batchStrideIm
			colStart := i * batchStrideCol

			if imEnd = imStart + batchStrideIm; imEnd >= len(imData) {
				imEnd = len(imData)
			}
			if colEnd = colStart + batchStrideCol; colEnd >= len(colData) {
				colEnd = len(colData)
			}

			op.f64s(c, h, w, chanStride, inRowStride, retHeight, retWidth, imData[imStart:imEnd], colData[colStart:colEnd])
		}
	case tensor.Float32:
		imData := input.Data().([]float32)
		colData := prealloc.Data().([]float32)
		for i := 0; i < b; i++ {
			imStart := i * batchStrideIm
			colStart := i * batchStrideCol

			if imEnd = imStart + batchStrideIm; imEnd >= len(imData) {
				imEnd = len(imData)
			}
			if colEnd = colStart + batchStrideCol; colEnd >= len(colData) {
				colEnd = len(colData)
			}

			op.f32s(c, h, w, chanStride, inRowStride, retHeight, retWidth, imData[imStart:imEnd], colData[colStart:colEnd])
		}
	default:
		return nil, errors.Errorf(nyiFail, "im2col", input.Dtype())
	}
	return prealloc, nil
}

func (op im2colOp) f64s(chans, height, width, chanStride, inRowStride, retHeight, retWidth int, im, col []float64) {
	var colIdx int
	for ch := 0; ch < chans; ch, im = ch+1, im[chanStride:] {
		for r := 0; r < retHeight; r++ {
			for c := 0; c < retWidth; c++ {
				for kr := 0; kr < op.h; kr++ {
					inRow := -op.padH + kr*op.dilationH + r*op.strideH
					for kc := 0; kc < op.w; kc++ {
						inCol := -op.padW + kc*op.dilationW + c*op.strideW
						var val float64

						switch {
						case inRow < 0:
						case inCol < 0:
						case inRow*inRowStride+inCol >= len(im):
						case inCol >= inRowStride:
						default:
							val = im[inRow*inRowStride+inCol]
						}

						col[colIdx] = val
						colIdx++
					}
				}
			}
		}
	}
}

func (op im2colOp) f32s(chans, height, width, chanStride, inRowStride, retHeight, retWidth int, im, col []float32) {
	var colIdx int
	for ch := 0; ch < chans; ch, im = ch+1, im[chanStride:] {
		for r := 0; r < retHeight; r++ {
			for c := 0; c < retWidth; c++ {
				for kr := 0; kr < op.h; kr++ {
					inRow := -op.padH + kr*op.dilationH + r*op.strideH
					for kc := 0; kc < op.w; kc++ {
						inCol := -op.padW + kc*op.dilationW + c*op.strideW
						var val float32

						switch {
						case inRow < 0:
						case inCol < 0:
						case inRow*inRowStride+inCol >= len(im):
						case inCol >= inRowStride:
						default:
							val = im[inRow*inRowStride+inCol]
						}

						col[colIdx] = val
						colIdx++
					}
				}
			}
		}
	}
}

type col2imOp struct {
	// input shapes of im2col
	unpaddedB int
	unpaddedC int
	unpaddedH int
	unpaddedW int

	im2colOp
}

func (op col2imOp) Arity() int { return 1 }

// im2col :: (Floats a) ⇒ a →  a
func (op col2imOp) Type() hm.Type {
	return hm.NewFnType(hm.TypeVariable('a'), hm.TypeVariable('a'))
}

func (op col2imOp) InferShape(shapes ...DimSizer) (retVal tensor.Shape, err error) {
	return tensor.Shape{op.unpaddedB, op.unpaddedC, op.unpaddedH, op.unpaddedW}, nil
}

func (op col2imOp) Do(inputs ...Value) (retVal Value, err error) {
	if err = checkArity(op, len(inputs)); err != nil {
		return
	}

	im := inputs[0]

	// todo type check values
	// todo shape check values

	retShape := tensor.Shape{op.unpaddedB, op.unpaddedC, op.unpaddedH, op.unpaddedW}
	prealloc := tensor.New(tensor.Of(im.Dtype()), tensor.WithShape(retShape...))

	return op.do(prealloc, im)
}

func (op col2imOp) ReturnsPtr() bool     { return false }
func (op col2imOp) CallsExtern() bool    { return false }
func (op col2imOp) OverwritesInput() int { return -1 }

func (op col2imOp) WriteHash(h hash.Hash) {
	fmt.Fprintf(h, "col2im:%d-%d-%d-%d-%d-%d", op.h, op.w, op.padH, op.padW, op.strideH, op.strideW)
}

func (op col2imOp) Hashcode() uint32 { return simpleHash(op) }

func (op col2imOp) String() string {
	return fmt.Sprintf("col2im<(%d,%d), (%d, %d), (%d,%d)>", op.h, op.w, op.padH, op.padW, op.strideH, op.strideW)
}

func (op col2imOp) UsePreallocDo(prealloc Value, inputs ...Value) (Value, error) {
	if err := checkArity(op, len(inputs)); err != nil {
		return nil, err
	}
	return op.do(prealloc, inputs[0])
}

func (op col2imOp) do(prealloc, input Value) (retVal Value, err error) {
	b := op.unpaddedB
	c := op.unpaddedC
	retHeight := op.unpaddedH
	retWidth := op.unpaddedW
	batchStrideIm := c * retHeight * retWidth

	inputT := input.(*tensor.Dense)
	outputT := prealloc.(*tensor.Dense)

	s := inputT.Shape()
	h := s[1]
	w := s[2]
	chanStride := retHeight * retWidth
	batchStrideCol := h * w * s[3]
	imRowStride := outputT.Strides()[2]

	var imEnd, colEnd int
	switch input.Dtype() {
	case tensor.Float64:
		colData := input.Data().([]float64)
		imData := prealloc.Data().([]float64)
		for i := 0; i < b; i++ {
			imStart := i * batchStrideIm
			colStart := i * batchStrideCol

			if imEnd = imStart + batchStrideIm; imEnd >= len(imData) {
				imEnd = len(imData)
			}
			if colEnd = colStart + batchStrideCol; colEnd >= len(colData) {
				colEnd = len(colData)
			}

			op.f64s(c, retHeight, retWidth, chanStride, imRowStride, h, w, colData[colStart:colEnd], imData[imStart:imEnd])
		}
	case tensor.Float32:
		colData := input.Data().([]float32)
		imData := prealloc.Data().([]float32)
		for i := 0; i < b; i++ {
			imStart := i * batchStrideIm
			colStart := i * batchStrideCol

			if imEnd = imStart + batchStrideIm; imEnd >= len(imData) {
				imEnd = len(imData)
			}
			if colEnd = colStart + batchStrideCol; colEnd >= len(colData) {
				colEnd = len(colData)
			}

			op.f32s(c, retHeight, retWidth, chanStride, imRowStride, h, w, colData[colStart:colEnd], imData[imStart:imEnd])
		}
	default:
		return nil, errors.Errorf(nyiFail, "col2im", input.Dtype())
	}

	return prealloc, nil
}

func (op col2imOp) f64s(chans, height, width, chanStride, imRowStride, retHeight, retWidth int, col, im []float64) {
	// memset im to 0
	for i := range im {
		im[i] = 0
	}

	var colIdx int
	for ch := chans; ch > 0; ch, im = ch-1, im[chanStride:] {
		for kernelRow := 0; kernelRow < op.h; kernelRow++ {
			for kernelCol := 0; kernelCol < op.w; kernelCol++ {
				inRow := -op.padH + kernelRow*op.dilationH
				for outRow := retHeight; outRow > 0; outRow-- {
					if !(inRow >= 0 && inRow < height) {
						colIdx += retWidth
					} else {
						inCol := -op.padW + kernelCol*op.dilationW
						for outCol := retWidth; outCol > 0; outCol-- {
							if inCol >= 0 && inCol < width {
								im[inRow*width+inCol] += col[colIdx]
							}
							colIdx++
							inCol += op.strideW
						}
					}
					inRow += op.strideH
				}
			}
		}
	}
}
func (op col2imOp) f32s(chans, height, width, chanStride, imRowStride, retHeight, retWidth int, col, im []float32) {
	// memset im to 0
	for i := range im {
		im[i] = 0
	}
	var colIdx int
	for ch := 0; ch < chans; ch, im = ch+1, im[chanStride:] {
		for r := 0; r < retHeight; r++ {
			for c := 0; c < retWidth; c++ {
				for kr := 0; kr < op.h; kr++ {
					inRow := -op.padH + kr*op.dilationH + r*op.strideH
					for kc := 0; kc < op.w; kc++ {
						inCol := -op.padW + kc*op.dilationW + c*op.strideW

						switch {
						case inRow < 0:
						case inCol < 0:
						case inRow*imRowStride+inCol >= len(im):
						case inCol >= imRowStride:
						default:
							im[inRow*imRowStride+inCol] += col[colIdx]
						}

						colIdx++
					}

				}

			}
		}
	}
	// for ch := chans; ch > 0; ch, im = ch-1, im[chanStride:] {
	// 	for kernelRow := 0; kernelRow < op.h; kernelRow++ {
	// 		for kernelCol := 0; kernelCol < op.w; kernelCol++ {
	// 			inRow := -op.padH + kernelRow*op.dilationH
	// 			for outRow := retHeight; outRow > 0; outRow-- {
	// 				if !(inRow >= 0 && inRow < height) {
	// 					colIdx += retWidth
	// 				} else {
	// 					inCol := -op.padW + kernelCol*op.dilationW
	// 					for outCol := retWidth; outCol > 0; outCol-- {
	// 						if inCol >= 0 && inCol < width {
	// 							im[inRow*width+inCol] += col[colIdx]
	// 						}
	// 						colIdx++
	// 						inCol += op.strideW
	// 					}
	// 				}
	// 				inRow += op.strideH
	// 			}
	// 		}
	// 	}
	// }
}

// It's important to note that this op actually produces TWO values - one argmax, which will be used
// as a mask, and the actual pooled value.
//
// The argmax is stored as an internal state and is not exposed to anything outside the op.
// There are alternative ways of designing this op, but they all don't particularly seem nice.
// Caffe's technique seemed the nicest.
type maxPoolOp struct {
	// Shape of Input
	unpaddedB int
	unpaddedC int
	unpaddedH int
	unpaddedW int

	h, w             int // patch height and width
	padH, padW       int
	strideH, strideW int

	// execution state
	// the mask is only filled at execution time
	mask tensor.Tensor
}

func newMaxPoolOp(inputShape, kernel tensor.Shape, pad, stride []int) *maxPoolOp {
	return &maxPoolOp{
		// Shape of Input
		unpaddedB: inputShape[0],
		unpaddedC: inputShape[1],
		unpaddedH: inputShape[2],
		unpaddedW: inputShape[3],

		h:       kernel[0],
		w:       kernel[1],
		padH:    pad[0],
		padW:    pad[1],
		strideH: stride[0],
		strideW: stride[1],

		mask: tensor.New(tensor.Of(Int), tensor.WithShape(inputShape.Clone()...)),
	}
}

func (op *maxPoolOp) Arity() int { return 1 }

// maxPoolOp has this type:
// 		op :: (...) → (...)
func (op *maxPoolOp) Type() hm.Type {
	a := hm.TypeVariable('a')
	t := newTensorType(4, a)
	return hm.NewFnType(t, t)
}
func (op *maxPoolOp) InferShape(inputs ...DimSizer) (tensor.Shape, error) {
	if s, ok := inputs[0].(tensor.Shape); ok {
		return op.calcShape(s), nil
	}
	return nil, errors.Errorf("Expected a shape")
}

func (op *maxPoolOp) Do(inputs ...Value) (retVal Value, err error) {
	var in, out tensor.Tensor
	if in, err = op.checkInput(inputs...); err != nil {
		return nil, err
	}
	inShp := in.Shape()
	out = tensor.New(tensor.Of(in.Dtype()), tensor.WithShape(op.calcShape(inShp)...), tensor.WithEngine(in.Engine()))
	op.do(out, in)
	return out, nil
}

func (op *maxPoolOp) ReturnsPtr() bool     { return false }
func (op *maxPoolOp) CallsExtern() bool    { return false }
func (op *maxPoolOp) OverwritesInput() int { return -1 }
func (op *maxPoolOp) WriteHash(h hash.Hash) {
	fmt.Fprintf(h, "MaxPool{%d, %d, %d, %d}(kernel: (%d, %d), pad: (%d, %d), stride: (%d, %d))",
		op.unpaddedB, op.unpaddedC, op.unpaddedH, op.unpaddedW,
		op.h, op.w, op.padH, op.padW, op.strideH, op.strideW)
}

func (op *maxPoolOp) Hashcode() uint32 { return simpleHash(op) }

func (op *maxPoolOp) String() string {
	return fmt.Sprintf("MaxPool{%d, %d, %d, %d}(kernel: (%d, %d), pad: (%d, %d), stride: (%d, %d))",
		op.unpaddedB, op.unpaddedC, op.unpaddedH, op.unpaddedW,
		op.h, op.w, op.padH, op.padW, op.strideH, op.strideW)
}

func (op *maxPoolOp) UsePreallocDo(prealloc Value, inputs ...Value) (Value, error) {
	var in tensor.Tensor
	var err error
	if in, err = op.checkInput(inputs...); err != nil {
		return nil, err
	}

	if p, ok := prealloc.(tensor.Tensor); ok {
		op.do(p, in)
		return p, nil
	}
	return nil, errors.Errorf("Expected prealloc to be a tensor")
}

func (op *maxPoolOp) DiffWRT(inputs int) []bool { return []bool{true} }

func (op *maxPoolOp) SymDiff(inputs Nodes, output, grad *Node) (retVal Nodes, err error) {
	if err = checkArity(op, len(inputs)); err != nil {
		return
	}
	input := inputs[0]

	var op2 maxPoolOp
	op2 = *op
	diff := &maxPoolDiffOp{op2}

	var ret *Node
	if ret, err = ApplyOp(diff, input, output, grad); err != nil {
		return nil, err
	}
	return Nodes{ret}, nil
}

func (op *maxPoolOp) DoDiff(ctx ExecutionContext, inputs Nodes, output *Node) (err error) {
	if err = checkArity(op, len(inputs)); err != nil {
		return
	}
	input := inputs[0]
	inputDV, outDV := getDV(input, output)

	var op2 maxPoolOp
	op2 = *op
	diff := &maxPoolDiffOp{op2}

	if _, err = diff.UsePreallocDo(inputDV.d, inputDV.Value, outDV.Value, outDV.d); err != nil {
		return errors.Wrapf(err, doFail, diff)
	}
	return
}

func (op *maxPoolOp) checkInput(inputs ...Value) (tensor.Tensor, error) {
	if err := checkArity(op, len(inputs)); err != nil {
		return nil, err
	}

	var in tensor.Tensor
	var ok bool
	if in, ok = inputs[0].(tensor.Tensor); !ok {
		return nil, errors.Errorf("Expected input to be a tensor")
	}

	if in.Shape().Dims() != 4 {
		return nil, errors.Errorf("Expected input to have 4 dimensions")
	}
	return in, nil
}

// calcShape calculates the output shape given an input shape
func (op *maxPoolOp) calcShape(s tensor.Shape) tensor.Shape {
	b, c, h, w := s[0], s[1], s[2], s[3]

	pooledH := ceilDivInt((h - op.padH - op.h + 1), op.strideH)
	pooledW := ceilDivInt((w - op.padW - op.w + 1), op.strideW)
	return tensor.Shape{b, c, pooledH, pooledW}
}

// do prepares the data, and then dispatches it to the correct (computation) kernel.
// out is the preallocated tensor
func (op *maxPoolOp) do(out, in tensor.Tensor) {
	outShape := out.Shape()
	outStride := out.Strides()[1]
	inShape := in.Shape()
	inStride := in.Strides()[1]
	maskStride := op.mask.Strides()[1]

	b, c, h, w := outShape[0], outShape[1], outShape[2], outShape[3]
	inH, inW := inShape[2], inShape[3]

	if op.mask == nil {
		op.mask = tensor.New(tensor.Of(tensor.Int), tensor.WithShape(op.calcShape(inShape)...))
	}

	maskData := op.mask.Data().([]int)

	switch in.Dtype() {
	case tensor.Float64:
		op.f64s(b, c, h, w, inH, inW,
			outStride, inStride, maskStride,
			out.Data().([]float64), in.Data().([]float64),
			maskData)
	case tensor.Float32:
		op.f32s(b, c, h, w, inH, inW,
			outStride, inStride, maskStride,
			out.Data().([]float32), in.Data().([]float32),
			maskData)
	}
}

func (op *maxPoolOp) f32s(batches, channels, outH, outW, inH, inW,
	outStride, inStride, maskStride int,
	outData, inData []float32,
	maskData []int) {

	// set values
	for i := range outData {
		outData[i] = -maxFloat32
		maskData[i] = -1
	}

	for b := 0; b < batches; b++ {
		for c := 0; c < channels; c++ {
			for ph := 0; ph < outH; ph++ {
				for pw := 0; pw < outW; pw++ {
					hStart := ph*op.strideH - op.padH
					wStart := pw*op.strideW - op.padW
					hEnd := minInt(hStart+op.h, inH)
					wEnd := minInt(wStart+op.w, inW)
					hStart = maxInt(hStart, 0)
					wStart = maxInt(wStart, 0)

					poolIndex := ph*outW + pw
					for hi := hStart; hi < hEnd; hi++ {
						for wi := wStart; wi < wEnd; wi++ {
							i := hi*inW + wi
							if inData[i] > outData[poolIndex] {
								outData[poolIndex] = inData[i]
								maskData[poolIndex] = i
							}
						}
					}
				}
			}
			// skip by strides
			inData = inData[inStride:]
			outData = outData[outStride:]
			maskData = maskData[maskStride:]
		}
	}
}

func (op *maxPoolOp) f64s(batches, channels, outH, outW, inH, inW,
	outStride, inStride, maskStride int,
	outData, inData []float64,
	maskData []int) {

	// set values
	for i := range outData {
		outData[i] = -maxFloat64
		maskData[i] = -1
	}

	for b := 0; b < batches; b++ {
		for c := 0; c < channels; c++ {
			for ph := 0; ph < outH; ph++ {
				for pw := 0; pw < outW; pw++ {
					hStart := ph*op.strideH - op.padH
					wStart := pw*op.strideW - op.padW
					hEnd := minInt(hStart+op.h, inH)
					wEnd := minInt(wStart+op.w, inW)
					hStart = maxInt(hStart, 0)
					wStart = maxInt(wStart, 0)

					poolIndex := ph*outW + pw

					for hi := hStart; hi < hEnd; hi++ {
						for wi := wStart; wi < wEnd; wi++ {
							i := hi*inW + wi
							if inData[i] > outData[poolIndex] {
								outData[poolIndex] = inData[i]
								maskData[poolIndex] = i
							}
						}
					}
				}
			}
			// skip by strides
			inData = inData[inStride:]
			outData = outData[outStride:]
			maskData = maskData[maskStride:]
		}
	}
}

type maxPoolDiffOp struct {
	maxPoolOp
}

func (op *maxPoolDiffOp) Arity() int { return 3 }
func (op *maxPoolDiffOp) Type() hm.Type {
	a := hm.TypeVariable('a')
	t := newTensorType(4, a)
	return hm.NewFnType(t, t, t, t)
}

func (op *maxPoolDiffOp) InferShape(inputs ...DimSizer) (tensor.Shape, error) {
	s := inputs[0].(tensor.Shape).Clone()
	return s, nil
}

func (op *maxPoolDiffOp) Do(inputs ...Value) (Value, error) {
	var in, out, pooled, pooledGrad tensor.Tensor
	var err error
	if in, pooled, pooledGrad, err = op.checkInput(inputs...); err != nil {
		return nil, err
	}

	// out is the gradient of in
	out = tensor.New(tensor.Of(in.Dtype()), tensor.WithShape(in.Shape().Clone()...), tensor.WithEngine(in.Engine()))
	op.do(out, in, pooled, pooledGrad)
	return out, nil
}
func (op *maxPoolDiffOp) ReturnsPtr() bool     { return true }
func (op *maxPoolDiffOp) CallsExtern() bool    { return false }
func (op *maxPoolDiffOp) OverwritesInput() int { return -1 }
func (op *maxPoolDiffOp) WriteHash(h hash.Hash) {
	fmt.Fprintf(h, "MaxPoolDiff{%d, %d, %d, %d}(kernel: (%d, %d), pad: (%d, %d), stride: (%d, %d))",
		op.unpaddedB, op.unpaddedC, op.unpaddedH, op.unpaddedW,
		op.h, op.w, op.padH, op.padW, op.strideH, op.strideW)
}

func (op *maxPoolDiffOp) Hashcode() uint32 { return simpleHash(op) }

func (op *maxPoolDiffOp) String() string {
	return fmt.Sprintf("MaxPoolDiff{%d, %d, %d, %d}(kernel: (%d, %d), pad: (%d, %d), stride: (%d, %d))",
		op.unpaddedB, op.unpaddedC, op.unpaddedH, op.unpaddedW,
		op.h, op.w, op.padH, op.padW, op.strideH, op.strideW)
}

func (op *maxPoolDiffOp) UsePreallocDo(prealloc Value, inputs ...Value) (Value, error) {
	var in, pooled, pooledGrad tensor.Tensor
	var err error
	if in, pooled, pooledGrad, err = op.checkInput(inputs...); err != nil {
		return nil, err
	}
	if p, ok := prealloc.(tensor.Tensor); ok {
		op.do(p, in, pooled, pooledGrad)
		return prealloc, nil
	}
	return nil, errors.Errorf("Cannot do with PreallocDo - expected PreAlloc to be tensor")
}

func (op *maxPoolDiffOp) checkInput(inputs ...Value) (in, pooled, pooledGrad tensor.Tensor, err error) {
	if err = checkArity(op, len(inputs)); err != nil {
		return
	}

	var ok bool
	if in, ok = inputs[0].(tensor.Tensor); !ok {
		err = errors.Errorf("Expected input to be a tensor")
		return
	}
	if in.Shape().Dims() != 4 {
		err = errors.Errorf("Expected input to have 4 dimensions")
		return
	}

	if pooled, ok = inputs[1].(tensor.Tensor); !ok {
		err = errors.Errorf("Expected pooled to be a tensor")
		return
	}
	if pooledGrad, ok = inputs[2].(tensor.Tensor); !ok {
		err = errors.Errorf("Expected pooledGrad to be a tensor")
		return
	}
	return
}

func (op *maxPoolDiffOp) do(inGrad, in, pooled, pooledGrad tensor.Tensor) {
	pooledShape := pooled.Shape()
	pooledStride := pooled.Strides()[1]
	inStride := in.Strides()[1]
	maskStride := op.mask.Strides()[1]
	maskData := op.mask.Data().([]int)

	b, c, h, w := pooledShape[0], pooledShape[1], pooledShape[2], pooledShape[3]
	switch in.Dtype() {
	case tensor.Float32:
		inGradData := inGrad.Data().([]float32)
		pooledGradData := pooledGrad.Data().([]float32)
		op.f32s(b, c, h, w,
			inStride, pooledStride, maskStride,
			inGradData, pooledGradData, maskData)
	case tensor.Float64:
		inGradData := inGrad.Data().([]float64)
		pooledGradData := pooledGrad.Data().([]float64)
		op.f64s(b, c, h, w,
			inStride, pooledStride, maskStride,
			inGradData, pooledGradData, maskData)
	}
}

// in is the "bottom", while out is the "top" (bottom being the unpooled, and top being the pooled)
func (op *maxPoolDiffOp) f32s(batches, channels, pooledH, pooledW int,
	inStride, outStride, maskStride int,
	inDiffData, outDiffData []float32,
	maskData []int) {

	// zero out. let's hope go's optimizer is smart enought
	for i := range inDiffData {
		inDiffData[i] = 0
	}

	// this loop can be goroutine'd
	for b := 0; b < batches; b++ {
		for c := 0; c < channels; c++ {
			for ph := 0; ph < pooledH; ph++ {
				for pw := 0; pw < pooledW; pw++ {
					index := ph*pooledW + pw
					inIndex := maskData[index]
					inDiffData[inIndex] += outDiffData[index]
				}
			}
			outDiffData = outDiffData[outStride:]
			inDiffData = inDiffData[inStride:]
			maskData = maskData[maskStride:]
		}
	}
}

// in is the "bottom", while out is the "top" (bottom being the unpooled, and top being the pooled)
func (op *maxPoolDiffOp) f64s(batches, channels, pooledH, pooledW int,
	inStride, outStride, maskStride int,
	inDiffData, outDiffData []float64,
	maskData []int) {

	// zero out. let's hope go's optimizer is smart enought
	for i := range inDiffData {
		inDiffData[i] = 0
	}

	// this loop can be goroutine'd
	for b := 0; b < batches; b++ {
		for c := 0; c < channels; c++ {
			for ph := 0; ph < pooledH; ph++ {
				for pw := 0; pw < pooledW; pw++ {
					index := ph*pooledW + pw
					inIndex := maskData[index]
					inDiffData[inIndex] += outDiffData[index]
				}
			}
			outDiffData = outDiffData[outStride:]
			inDiffData = inDiffData[inStride:]
			maskData = maskData[maskStride:]
		}
	}
}

// clampOp is a constant clamping operation
type clampOp struct {
	min, max Scalar
}

func (op *clampOp) Arity() int { return 1 }

func (op *clampOp) Type() hm.Type {
	return hm.NewFnType(hm.TypeVariable('a'), hm.TypeVariable('a'))
}

func (op *clampOp) InferShape(shps ...DimSizer) (tensor.Shape, error) {
	return shps[0].(tensor.Shape), nil
}

func (op *clampOp) Do(vals ...Value) (Value, error) {
	return nil, nil
}

func (op *clampOp) ReturnsPtr() bool { return true }

func (op *clampOp) CallsExtern() bool { return false }

func (op *clampOp) OverwritesInput() int { return 0 }

func (op *clampOp) WriteHash(h hash.Hash) { fmt.Fprintf(h, "ConstClamp{%f, %f}()", op.min, op.max) }

func (op *clampOp) Hashcode() uint32 { return simpleHash(op) }
func (op *clampOp) String() string   { return fmt.Sprintf("ConstClamp{%f, %f}()", op.min, op.max) }

// BatchNormOp is a batch normalization process as described by Ioffe and Szegedy (2015) -
// http://arxiv.org/abs/1502.03167
//
// Normalization is done as:
// 	γ(x - μ) / σ + β
// γ is the scaling factor and β is the offset factor. These are created by BatchNorm()
type BatchNormOp struct {
	Momentum float64 // Momentum for the moving average
	Epsilon  float64 // Epsilon is a small variance to be added to avoid dividing by 0

	// Learnables
	Mean, Variance, MA *tensor.Dense

	// scratch space
	meanScratch, varianceScratch, tmpScratch, xNorm      *tensor.Dense
	batchSumMultiplier, numByChans, spatialSumMultiplier *tensor.Dense

	// training? if training then update movingMean and movingVar
	training bool
}

// Init the scratch spaces according to the size and type of x
func (op *BatchNormOp) Init(x *Node, training bool) error {
	dt, err := dtypeOf(x.Type())
	if err != nil {
		return err
	}
	batches := x.Shape()[0]
	channels := x.Shape()[1]
	spatialDim := x.Shape().TotalSize() / (channels * batches)

	meanScratch := tensor.New(tensor.Of(dt), tensor.WithShape(channels))
	varianceScratch := tensor.New(tensor.Of(dt), tensor.WithShape(channels))
	tmp := tensor.New(tensor.Of(dt), tensor.WithShape(x.Shape().Clone()...))
	xNorm := tensor.New(tensor.Of(dt), tensor.WithShape(x.Shape().Clone()...))
	batchSumMultiplier := tensor.New(tensor.Of(dt), tensor.WithShape(batches))

	var uno interface{}
	switch dt {
	case Float64:
		uno = float64(1)
	case Float32:
		uno = float32(1)
	}
	spatialSumMultiplier := tensor.New(tensor.Of(dt), tensor.WithShape(spatialDim))
	if err = spatialSumMultiplier.Memset(uno); err != nil {
		return err
	}

	numByChans := tensor.New(tensor.Of(dt), tensor.WithShape(channels*batches))
	if err = batchSumMultiplier.Memset(uno); err != nil {
		return err
	}

	op.meanScratch = meanScratch
	op.varianceScratch = varianceScratch
	op.tmpScratch = tmp
	op.xNorm = xNorm
	op.batchSumMultiplier = batchSumMultiplier
	op.numByChans = numByChans
	op.spatialSumMultiplier = spatialSumMultiplier

	op.training = training

	return nil
}

// Arity returns 1
func (op *BatchNormOp) Arity() int { return 1 }

// Type of the node (returns a 4D tensor)
func (op *BatchNormOp) Type() hm.Type {
	t := TensorType{Dims: 4, Of: hm.TypeVariable('a')}
	return hm.NewFnType(t, t)
}

// InferShape returns the output shape
func (op *BatchNormOp) InferShape(ns ...DimSizer) (tensor.Shape, error) {
	if err := checkArity(op, len(ns)); err != nil {
		return nil, errors.Wrapf(err, "batchNorm")
	}

	return ns[0].(tensor.Shape).Clone(), nil
}

// Do ...
func (op *BatchNormOp) Do(values ...Value) (retVal Value, err error) {
	if err := checkArity(op, len(values)); err != nil {
		return nil, errors.Wrapf(err, "batchNorm Do")
	}
	var v, out Value
	v = values[0]
	if out, err = CloneValue(v); err != nil {
		return nil, err
	}
	return op.UsePreallocDo(out, v)
}

// ReturnsPtr will always return true
func (op *BatchNormOp) ReturnsPtr() bool { return true }

// CallsExtern is returnin false: this Op do not use cuda or cgo
func (op *BatchNormOp) CallsExtern() bool { return false }

// OverwritesInput returns -1: overzriting of the input disallowed
func (op *BatchNormOp) OverwritesInput() int { return -1 }

// WriteHash ...
func (op *BatchNormOp) WriteHash(h hash.Hash) {
	fmt.Fprintf(h, "batchnorm-%1.1f-%1.1f", op.Momentum, op.Epsilon)
}

// Hashcode ...
func (op *BatchNormOp) Hashcode() uint32 { return simpleHash(op) }

func (op *BatchNormOp) String() string {
	return fmt.Sprintf("batchnorm-%1.1f-%1.1f", op.Momentum, op.Epsilon)
}

// DoDiff implements the ADOp interface (operator supports automatic differentiation)
func (op *BatchNormOp) DoDiff(ctx ExecutionContext, inputs Nodes, output *Node) error {
	diff := &batchnormDiffOp{op}
	xdv, ydv := getDV(inputs[0], output)
	_, err := diff.UsePreallocDo(xdv.d, xdv.Value, ydv.d)
	return err
}

// DiffWRT implements the SDOp interface (operator supports symbolic differentiation)
func (op *BatchNormOp) DiffWRT(inputs int) []bool { return []bool{true} }

// SymDiff implements the SDOp interface (operator supports symbolic differentiation)
func (op *BatchNormOp) SymDiff(inputs Nodes, output *Node, grad *Node) (retVal Nodes, err error) {
	if err = checkArity(op, len(inputs)); err != nil {
		return
	}
	input := inputs[0]
	diff := &batchnormDiffOp{op}

	var ret *Node
	if ret, err = ApplyOp(diff, input, grad); err != nil {
		return nil, err
	}
	return Nodes{ret}, nil
}

// UsePreallocDo implements the UsePreallocDoer interface
func (op *BatchNormOp) UsePreallocDo(prealloc Value, inputs ...Value) (retVal Value, err error) {
	v := inputs[0]
	switch v.Dtype() {
	case Float64:
		err = op.f64s(v.(*tensor.Dense), prealloc.(*tensor.Dense))
	case Float32:
		err = op.f32s(v.(*tensor.Dense), prealloc.(*tensor.Dense))
	default:
		return nil, nyi("BatchNorm Do", v.Dtype())
	}
	return prealloc, err
}

// SetTraining to true. call a Reset() before
func (op *BatchNormOp) SetTraining() { op.Reset(); op.training = true }

// SetTesting change the training mode to false.
func (op *BatchNormOp) SetTesting() { op.training = false }

// Reset ...
func (op *BatchNormOp) Reset() error {
	dt := op.MA.Dtype()
	var uno interface{}
	switch dt {
	case Float64:
		uno = float64(1)
	case Float32:
		uno = float32(1)
	}

	if err := op.spatialSumMultiplier.Memset(uno); err != nil {
		return err
	}

	if err := op.batchSumMultiplier.Memset(uno); err != nil {
		return err
	}

	op.Mean.Zero()
	op.Variance.Zero()
	op.MA.Zero()
	op.meanScratch.Zero()
	op.varianceScratch.Zero()
	op.tmpScratch.Zero()
	op.numByChans.Zero()
	return nil
}

func (op *BatchNormOp) f64s(input, output *tensor.Dense) (err error) {
	n := input.Shape()[0]
	channels := input.Shape()[1]
	nc := channels * n
	spatialDim := input.Shape().TotalSize() / (nc)

	inputF64s := input.Float64s()
	outputF64s := output.Float64s()
	copy(outputF64s, inputF64s)

	meanScratch := op.meanScratch.Float64s()
	mean := op.Mean.Float64s()
	varianceScratch := op.varianceScratch.Float64s()
	variance := op.Variance.Float64s()
	tmp := op.tmpScratch.Float64s()
	ssm := op.spatialSumMultiplier.Float64s()
	nbc := op.numByChans.Float64s()
	bsm := op.batchSumMultiplier.Float64s()

	momentum := op.Momentum
	eps := op.Epsilon

	if !op.training {
		// use stored mean/variance estimates
		scaleFactor := float64(1)
		if fst := op.MA.Float64s()[0]; fst != 1 {
			scaleFactor = fst
		}
		copy(meanScratch, mean)
		whichblas.Dscal(len(meanScratch), scaleFactor, meanScratch, 1)
		copy(varianceScratch, variance)
		whichblas.Dscal(len(varianceScratch), scaleFactor, varianceScratch, 1)
	} else {
		// compute mean
		alpha := 1.0 / float64(n*spatialDim)
		whichblas.Dgemv(blas.NoTrans, nc, spatialDim, alpha, inputF64s, spatialDim, ssm, 1, 0, nbc, 1)
		whichblas.Dgemv(blas.Trans, n, channels, 1, nbc, channels, bsm, 1, 0, meanScratch, 1)
	}

	// subtract mean
	whichblas.Dgemm(blas.NoTrans, blas.NoTrans, n, channels, 1, 1, bsm, 1, meanScratch, channels, 0, nbc, channels)
	whichblas.Dgemm(blas.NoTrans, blas.NoTrans, nc, spatialDim, 1, -1, nbc, 1, ssm, spatialDim, 1, outputF64s, spatialDim)

	if op.training {
		// compute variance using var(X) = E(X-EX)^2)
		copy(tmp, outputF64s)
		vecf64.Mul(tmp, tmp) // (X-EX) ^ 2

		whichblas.Dgemv(blas.NoTrans, nc, spatialDim, 1.0/(float64(n*spatialDim)), tmp, spatialDim, ssm, 1, 0, nbc, 1)
		whichblas.Dgemv(blas.Trans, n, channels, 1.0, nbc, channels, bsm, 1, 0, varianceScratch, 1) // E((X_EX)^2)

		// compute and save moving average
		op.MA.Float64s()[0] *= momentum
		op.MA.Float64s()[0]++

		// TODO: write axpby for gonum
		whichblas.Dscal(len(mean), momentum, mean, 1)
		whichblas.Daxpy(len(meanScratch), 1.0, meanScratch, 1, mean, 1)

		m := len(inputF64s) / channels
		correctionFactor := float64(1)
		if m > 1 {
			correctionFactor = float64(m) / (float64(m - 1))
		}
		whichblas.Dscal(len(variance), momentum, variance, 1)
		whichblas.Daxpy(len(varianceScratch), correctionFactor, varianceScratch, 1, variance, 1)
	}

	// normalize variance
	vecf64.Trans(varianceScratch, eps)
	vecf64.Sqrt(varianceScratch)

	// replicate variance to inputsize
	whichblas.Dgemm(blas.NoTrans, blas.NoTrans, n, channels, 1, 1, bsm, 1, varianceScratch, channels, 0, nbc, channels)
	whichblas.Dgemm(blas.NoTrans, blas.NoTrans, nc, spatialDim, 1, 1, nbc, 1, ssm, spatialDim, 0, tmp, spatialDim)
	vecf64.Div(outputF64s, tmp)
	copy(op.xNorm.Float64s(), outputF64s) // caching

	return nil
}

func (op *BatchNormOp) f32s(input, output *tensor.Dense) (err error) {
	n := input.Shape()[0]
	channels := input.Shape()[1]
	nc := channels * n
	spatialDim := input.Shape().TotalSize() / (nc)

	inputF32s := input.Float32s()
	outputF32s := output.Float32s()
	copy(outputF32s, inputF32s)

	meanScratch := op.meanScratch.Float32s()
	mean := op.Mean.Float32s()
	varianceScratch := op.varianceScratch.Float32s()
	variance := op.Variance.Float32s()
	tmp := op.tmpScratch.Float32s()
	ssm := op.spatialSumMultiplier.Float32s()
	nbc := op.numByChans.Float32s()
	bsm := op.batchSumMultiplier.Float32s()

	momentum := float32(op.Momentum)
	eps := float32(op.Epsilon)

	if !op.training {
		// use stored mean/variance estimates
		scaleFactor := float32(1)
		if fst := op.MA.Float32s()[0]; fst != 1 {
			scaleFactor = fst
		}
		copy(meanScratch, mean)
		whichblas.Sscal(len(meanScratch), scaleFactor, meanScratch, 1)
		copy(varianceScratch, variance)
		whichblas.Sscal(len(varianceScratch), scaleFactor, varianceScratch, 1)
	} else {
		// compute mean
		alpha := 1.0 / float32(n*spatialDim)
		whichblas.Sgemv(blas.NoTrans, nc, spatialDim, alpha, inputF32s, spatialDim, ssm, 1, 0, nbc, 1)
		whichblas.Sgemv(blas.Trans, n, channels, 1, nbc, channels, bsm, 1, 0, meanScratch, 1)
	}

	// subtract mean
	whichblas.Sgemm(blas.NoTrans, blas.NoTrans, n, channels, 1, 1, bsm, 1, meanScratch, channels, 0, nbc, channels)
	whichblas.Sgemm(blas.NoTrans, blas.NoTrans, nc, spatialDim, 1, -1, nbc, 1, ssm, spatialDim, 1, outputF32s, spatialDim)

	if op.training {
		// compute variance using var(X) = E(X-EX)^2)
		copy(tmp, outputF32s)
		vecf32.Mul(tmp, tmp) // (X-EX) ^ 2

		whichblas.Sgemv(blas.NoTrans, nc, spatialDim, 1.0/(float32(n*spatialDim)), tmp, spatialDim, ssm, 1, 0, nbc, 1)
		whichblas.Sgemv(blas.Trans, n, channels, 1.0, nbc, channels, bsm, 1, 0, varianceScratch, 1) // E((X_EX)^2)

		// compute and save moving average
		op.MA.Float32s()[0] *= momentum
		op.MA.Float32s()[0]++

		// TODO: write axpby for gonum
		whichblas.Sscal(len(mean), momentum, mean, 1)
		whichblas.Saxpy(len(meanScratch), 1.0, meanScratch, 1, mean, 1)

		m := len(inputF32s) / channels
		correctionFactor := float32(1)
		if m > 1 {
			correctionFactor = float32(m) / (float32(m - 1))
		}
		whichblas.Sscal(len(variance), momentum, variance, 1)
		whichblas.Saxpy(len(varianceScratch), correctionFactor, varianceScratch, 1, variance, 1)
	}

	// normalize variance
	vecf32.Trans(varianceScratch, eps)
	vecf32.Sqrt(varianceScratch)

	// replicate variance to inputsize
	whichblas.Sgemm(blas.NoTrans, blas.NoTrans, n, channels, 1, 1, bsm, 1, varianceScratch, channels, 0, nbc, channels)
	whichblas.Sgemm(blas.NoTrans, blas.NoTrans, nc, spatialDim, 1, 1, nbc, 1, ssm, spatialDim, 0, tmp, spatialDim)
	vecf32.Div(outputF32s, tmp)
	copy(op.xNorm.Float32s(), outputF32s) // caching

	return nil
}

type batchnormDiffOp struct{ *BatchNormOp }

func (op *batchnormDiffOp) Arity() int { return 2 }

func (op *batchnormDiffOp) Type() hm.Type {
	t := TensorType{Dims: 4, Of: hm.TypeVariable('a')}
	return hm.NewFnType(t, t, t)
}

func (op *batchnormDiffOp) InferShape(ns ...DimSizer) (tensor.Shape, error) {
	if err := checkArity(op, len(ns)); err != nil {
		return nil, errors.Wrapf(err, "batchNorm")
	}

	return ns[0].(tensor.Shape).Clone(), nil
}

func (op *batchnormDiffOp) Do(values ...Value) (Value, error) {
	input := values[0].(*tensor.Dense)
	grad := values[1].(*tensor.Dense)
	inputGrad := input.Clone().(*tensor.Dense)
	return op.UsePreallocDo(inputGrad, input, grad)
}

// ReturnsPtr is the same exact characteristics of batchnorm
// CallsExtern is the same exact characteristics of batchnorm
// OverwritesInput is the same exact characteristics of batchnorm

func (op *batchnormDiffOp) WriteHash(h hash.Hash) {
	fmt.Fprintf(h, "batchnormdiff-%1.1f-%1.1f", op.Momentum, op.Epsilon)
}

func (op *batchnormDiffOp) Hashcode() uint32 { return simpleHash(op) }

func (op *batchnormDiffOp) String() string {
	return fmt.Sprintf("batchnormdiff-%1.1f-%1.1f", op.Momentum, op.Epsilon)
}

func (op *batchnormDiffOp) DiffWRT(inputs int) []bool {
	// god help those who want to  do 2nd order differentiation on batchnorm
	return []bool{false, false}
}

func (op *batchnormDiffOp) SymDiff(inputs Nodes, output *Node, grad *Node) (retVal Nodes, err error) {
	// god help those who want to  do 2nd order differentiation on batchnorm
	return nil, nyi("SymDiff", "batchNormDiffOp")
}

func (op *batchnormDiffOp) DoDiff(ctx ExecutionContext, inputs Nodes, output *Node) error {
	// god help those who want to  do 2nd order differentiation on batchnorm
	return nyi("DoDiff", "batchnormDiffOp")
}

func (op *batchnormDiffOp) UsePreallocDo(prealloc Value, inputs ...Value) (retVal Value, err error) {
	input := inputs[0].(*tensor.Dense)
	inGrad := prealloc.(*tensor.Dense)
	outGrad := inputs[1].(*tensor.Dense)

	switch input.Dtype() {
	case Float64:
		err = op.f64s(input, inGrad, outGrad)
	case Float32:
		err = op.f32s(input, inGrad, outGrad)
	default:
		return nil, nyi("batchnormDiffOp", "Do")
	}
	return prealloc, err
}

func (op *batchnormDiffOp) f64s(input, inGrad, outGrad *tensor.Dense) (err error) {
	in := input.Float64s()
	ig := inGrad.Float64s()
	og := outGrad.Float64s()
	tmp := op.tmpScratch.Float64s()
	out := op.xNorm.Float64s()
	ssm := op.spatialSumMultiplier.Float64s()
	nbc := op.numByChans.Float64s()
	bsm := op.batchSumMultiplier.Float64s()
	meanScratch := op.meanScratch.Float64s()

	if !op.training {
		copy(ig, og)
		vecf64.Div(og, tmp)
		return nil
	}

	n := input.Shape()[0]
	channels := input.Shape()[1]
	nc := n * channels
	spatialDim := len(in) / nc

	// if Y = (X-mean(X))/(sqrt(var(X)+eps)), then
	//
	// dE(Y)/dX =
	//   (dE/dY - mean(dE/dY) - mean(dE/dY \cdot Y) \cdot Y)
	//     ./ sqrt(var(X) + eps)
	//
	// where \cdot and ./ are hadamard product and elementwise division,
	// respectively, dE/dY is the top diff, and mean/var/sum are all computed
	// along all dimensions except the channels dimension.  In the above
	// equation, the operations allow for expansion (i.e. broadcast) along all
	// dimensions except the channels dimension where required.

	// sum(dE/dY \cdot Y)
	copy(ig, out)
	vecf64.Mul(ig, og)
	whichblas.Dgemv(blas.NoTrans, nc, spatialDim, 1, ig, spatialDim, ssm, 1, 0, nbc, 1)
	whichblas.Dgemv(blas.Trans, n, channels, 1, nbc, channels, bsm, 1, 0, meanScratch, 1)

	// reshape (broadcast) the above
	whichblas.Dgemm(blas.NoTrans, blas.NoTrans, n, channels, 1, 1, bsm, 1, meanScratch, channels, 0, nbc, channels)
	whichblas.Dgemm(blas.NoTrans, blas.NoTrans, nc, spatialDim, 1, 1, nbc, 1, ssm, spatialDim, 0, ig, spatialDim)

	// sum(dE/dY \cdot Y) \cdot Y
	vecf64.Mul(ig, out)

	// sum(dE/dY)-sum(dE/dY \cdot Y) \cdot Y
	whichblas.Dgemv(blas.NoTrans, nc, spatialDim, 1, og, spatialDim, ssm, 1, 0, nbc, 1)
	whichblas.Dgemv(blas.Trans, n, channels, 1, nbc, channels, bsm, 1, 0, meanScratch, 1)

	// reshape (broadcast) the above to make
	// sum(dE/dY)-sum(dE/dY \cdot Y) \cdot Y
	whichblas.Dgemm(blas.NoTrans, blas.NoTrans, n, channels, 1, 1, bsm, 1, meanScratch, channels, 0, nbc, channels)
	whichblas.Dgemm(blas.NoTrans, blas.NoTrans, nc, spatialDim, 1, 1, nbc, 1, ssm, spatialDim, 1, ig, spatialDim)

	// dE/dY - mean(dE/dY)-mean(dE/dY \cdot Y) \cdot Y
	beta := (-1.0 / float64(nc))
	vecf64.Scale(ig, beta)
	vecf64.Add(ig, og)

	// note: temp_ still contains sqrt(var(X)+eps), computed during the forward
	// pass.
	vecf64.Div(ig, tmp)
	return nil

}

func (op *batchnormDiffOp) f32s(input, inGrad, outGrad *tensor.Dense) (err error) {
	in := input.Float32s()
	ig := inGrad.Float32s()
	og := outGrad.Float32s()
	tmp := op.tmpScratch.Float32s()
	out := op.xNorm.Float32s()
	ssm := op.spatialSumMultiplier.Float32s()
	nbc := op.numByChans.Float32s()
	bsm := op.batchSumMultiplier.Float32s()
	meanScratch := op.meanScratch.Float32s()

	if !op.training {
		copy(ig, og)
		vecf32.Div(og, tmp)
		return nil
	}

	n := input.Shape()[0]
	channels := input.Shape()[1]
	nc := n * channels
	spatialDim := len(in) / nc

	// if Y = (X-mean(X))/(sqrt(var(X)+eps)), then
	//
	// dE(Y)/dX =
	//   (dE/dY - mean(dE/dY) - mean(dE/dY \cdot Y) \cdot Y)
	//     ./ sqrt(var(X) + eps)
	//
	// where \cdot and ./ are hadamard product and elementwise division,
	// respectively, dE/dY is the top diff, and mean/var/sum are all computed
	// along all dimensions except the channels dimension.  In the above
	// equation, the operations allow for expansion (i.e. broadcast) along all
	// dimensions except the channels dimension where required.

	// sum(dE/dY \cdot Y)
	copy(ig, out)
	vecf32.Mul(ig, og)
	whichblas.Sgemv(blas.NoTrans, nc, spatialDim, 1, ig, spatialDim, ssm, 1, 0, nbc, 1)
	whichblas.Sgemv(blas.Trans, n, channels, 1, nbc, channels, bsm, 1, 0, meanScratch, 1)

	// reshape (broadcast) the above
	whichblas.Sgemm(blas.NoTrans, blas.NoTrans, n, channels, 1, 1, bsm, 1, meanScratch, channels, 0, nbc, channels)
	whichblas.Sgemm(blas.NoTrans, blas.NoTrans, nc, spatialDim, 1, 1, nbc, 1, ssm, spatialDim, 0, ig, spatialDim)

	// sum(dE/dY \cdot Y) \cdot Y
	vecf32.Mul(ig, out)

	// sum(dE/dY)-sum(dE/dY \cdot Y) \cdot Y
	whichblas.Sgemv(blas.NoTrans, nc, spatialDim, 1, og, spatialDim, ssm, 1, 0, nbc, 1)
	whichblas.Sgemv(blas.Trans, n, channels, 1, nbc, channels, bsm, 1, 0, meanScratch, 1)

	// reshape (broadcast) the above to make
	// sum(dE/dY)-sum(dE/dY \cdot Y) \cdot Y
	whichblas.Sgemm(blas.NoTrans, blas.NoTrans, n, channels, 1, 1, bsm, 1, meanScratch, channels, 0, nbc, channels)
	whichblas.Sgemm(blas.NoTrans, blas.NoTrans, nc, spatialDim, 1, 1, nbc, 1, ssm, spatialDim, 1, ig, spatialDim)

	// dE/dY - mean(dE/dY)-mean(dE/dY \cdot Y) \cdot Y
	beta := (-1.0 / float32(n*spatialDim))
	vecf32.Scale(ig, beta)
	vecf32.Add(ig, og)

	// note: temp_ still contains sqrt(var(X)+eps), computed during the forward
	// pass.
	vecf32.Div(ig, tmp)
	return nil

}
