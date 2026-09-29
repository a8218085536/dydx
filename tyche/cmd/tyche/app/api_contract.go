package app

import (
	"context"
	"encoding/json"
	"ethgo/proto"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Call
// @Description 调用智能合约中的方法
// @Description
// @Description Call executes a message call transaction, which is directly executed in the VM of the node, but never mined into the blockchain.
// @Tags 智能合约
// @Accept application/json
// @Produce application/json
// @Param object body proto.Call{args=object} true "请求参数"
// @Success 200 {object} proto.Response{data=object}
// @Router /tyche/api/call [post]
func (app *App) Call(c *ginx.Context) {
	var request = new(proto.Call)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	var bytes, err = json.MarshalIndent(request, "", "  ")
	if err != nil {
		panic(err)
	}
	log.Debugf("%v", string(bytes))

	request.Address = strings.ToLower(request.Address)
	if !common.IsHexAddress(request.Address) {
		c.Failure(http.StatusBadRequest, "无效的参数: address", nil)
		return
	}
	var caller = tyche.Caller{
		Address:    common.HexToAddress(request.Address),
		MethodName: request.Method,
		Args:       request.Args,
	}

	runner, err := app.evmTransactor()
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	res, err := runner.Call(context.Background(), caller)
	if err != nil {
		log.Errorf("Failed to %v: %v, %v", c.Request.URL, err, caller)
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.Success(http.StatusOK, "succ", res)
}

// Transact
// @Description 调用智能合约中的付费方法
// @Description
// @Description Transact invokes the (paid) contract method with params as input values.
// @Tags 智能合约
// @Accept application/json
// @Produce application/json
// @Param object body proto.Transact{args=object} true "请求参数"
// @Success 200 {object} proto.Response{data=object}
// @Router /tyche/api/transact [post]
func (app *App) Transact(c *ginx.Context) {
	var request = new(proto.Transact)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	var bytes, err = json.MarshalIndent(request, "", "  ")
	if err != nil {
		panic(err)
	}

	log.Debugf("%v", string(bytes))

	request.Address = strings.ToLower(request.Address)
	if !common.IsHexAddress(request.Address) {
		c.Failure(http.StatusBadRequest, "无效的参数: address", nil)
		return
	}

	var transactor = tyche.Transactor{
		Address:    common.HexToAddress(request.Address),
		MethodName: request.Method,
		Args:       request.Args,
	}

	runner, err := app.evmTransactor()
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if err = runner.Transact(context.Background(), request.OrderID, transactor); err != nil {
		log.Errorf("Failed to %v: %v, %v", c.Request.URL, err, transactor)
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	c.Success(http.StatusOK, "succ", nil)
}
