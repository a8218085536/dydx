package gasprice

import (
	"ethgo/util/bigx"
	"io/ioutil"
	"math/big"

	"github.com/robertkrimen/otto"
)

type Estimator interface {
	SuggestGasPrice(gasPrice *big.Int) (*big.Int, error)
	BumpingGas(gasPrice *big.Int) (*big.Int, error)
}

func NewJSEstimator(file string, chainID *big.Int) (Estimator, error) {
	vm := otto.New()
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	_, err = vm.Run(bytes)
	if err != nil {
		return nil, err
	}

	return &jsEstimator{vm: vm, chainID: chainID}, nil
}

type jsEstimator struct {
	vm      *otto.Otto
	chainID *big.Int
}

func (p *jsEstimator) SuggestGasPrice(gasPrice *big.Int) (*big.Int, error) {
	value, err := p.vm.Call("suggestGasPrice", nil, p.chainID.Uint64(), gasPrice.Uint64())
	if err != nil {
		return nil, err
	}

	v, err := value.ToInteger()
	if err != nil {
		return nil, err
	}
	return big.NewInt(v), nil
}

func (p *jsEstimator) BumpingGas(gasPrice *big.Int) (*big.Int, error) {
	value, err := p.vm.Call("bumpingGas", nil, p.chainID.Uint64(), gasPrice.Uint64())
	if err != nil {
		return nil, err
	}

	v, err := value.ToInteger()
	if err != nil {
		return nil, err
	}
	return big.NewInt(v), nil
}

type defaultEstimator struct {
}

func (p *defaultEstimator) SuggestGasPrice(gasPrice *big.Int) (*big.Int, error) {
	return gasPrice, nil
}

func (p *defaultEstimator) BumpingGas(gasPrice *big.Int) (*big.Int, error) {
	return bigx.Mul(gasPrice, 1.1), nil
}
