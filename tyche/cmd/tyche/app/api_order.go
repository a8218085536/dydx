package app

import (
	"ethgo/model/orders"
	"ethgo/proto"
	"ethgo/util/ginx"
	"net/http"
	"strconv"
)

// Order
// @Description 获得订单信息
// @Description
// @Tags 订单
// @Accept application/json
// @Produce application/json
// @Param object body proto.Order{} true "请求参数"
// @Success 200 {object}  proto.Response{data=proto.OrderResponse{}}
// @Router /tyche/api/order/get [post]
func (app *App) Order(c *ginx.Context) {
	var request = new(proto.Order)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	order, err := orders.Get(request.OrderID)
	if err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	c.Success(http.StatusOK, "succ", proto.OrderResponse{
		OrderID:         order.Id,
		Status:          order.Status,
		NumberOfRetries: order.NumberOfRetries,
		TxHash:          order.TxHash,
		Reason:          order.Reason,
		CreatedAt:       strconv.FormatInt(order.CreatedAt, 10),
		UpdatedAt:       strconv.FormatInt(order.UpdatedAt, 10),
		Nonce:           strconv.FormatUint(order.Nonce, 10),
	})
}
