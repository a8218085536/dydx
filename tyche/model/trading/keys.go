package trading

import (
	"ethgo/model"
	"fmt"
)

type keygen struct{}

var keys keygen

func (k keygen) Namespace() string {
	return fmt.Sprintf("%s:trading", model.RedisPool.Namespace)
}

func (k keygen) Order(userID, orderID string) string {
	return fmt.Sprintf("%s:order:%s:%s", k.Namespace(), userID, orderID)
}

func (k keygen) ClientOrder(userID, clientOrderID string) string {
	return fmt.Sprintf("%s:client-order:%s:%s", k.Namespace(), userID, clientOrderID)
}

func (k keygen) UserOrders(userID string) string {
	return fmt.Sprintf("%s:user-orders:%s", k.Namespace(), userID)
}

func (k keygen) ReconcilableOrders() string {
	return fmt.Sprintf("%s:reconcilable-orders", k.Namespace())
}

func (k keygen) RetainedCollateral(dydxAccount string, subaccountNumber uint32) string {
	return fmt.Sprintf("%s:dydx-retained-collateral:%s:%d", k.Namespace(), dydxAccount, subaccountNumber)
}

func (k keygen) Event(eventID string) string {
	return fmt.Sprintf("%s:event:%s", k.Namespace(), eventID)
}

func (k keygen) Fill(eventID string) string {
	return fmt.Sprintf("%s:fill:%s", k.Namespace(), eventID)
}

func (k keygen) OrderFills(userID, orderID string) string {
	return fmt.Sprintf("%s:order-fills:%s:%s", k.Namespace(), userID, orderID)
}

func (k keygen) Position(userID, market string) string {
	return fmt.Sprintf("%s:position:%s:%s", k.Namespace(), userID, market)
}

func (k keygen) PositionForSubaccount(userID, market string, subaccountNumber uint32) string {
	if subaccountNumber == 0 {
		return k.Position(userID, market)
	}
	return fmt.Sprintf("%s:position:%s:%s:%d", k.Namespace(), userID, market, subaccountNumber)
}

func (k keygen) UserPositions(userID string) string {
	return fmt.Sprintf("%s:user-positions:%s", k.Namespace(), userID)
}

func (k keygen) OrderStatus(eventID string) string {
	return fmt.Sprintf("%s:order-status:%s", k.Namespace(), eventID)
}

func (k keygen) UserOrderStatuses(userID string) string {
	return fmt.Sprintf("%s:user-order-statuses:%s", k.Namespace(), userID)
}

func (k keygen) Settlement(id string) string {
	return fmt.Sprintf("%s:settlement:%s", k.Namespace(), id)
}

func (k keygen) CloseOrderSettlement(userID, closeOrderID string) string {
	return fmt.Sprintf("%s:close-settlement:%s:%s", k.Namespace(), userID, closeOrderID)
}

func (k keygen) WithdrawalSettlement(withdrawalID string) string {
	return fmt.Sprintf("%s:withdrawal-settlement:%s", k.Namespace(), withdrawalID)
}

func (k keygen) UserSettlements(userID string) string {
	return fmt.Sprintf("%s:user-settlements:%s", k.Namespace(), userID)
}

func (k keygen) StatusSettlements(status string) string {
	return fmt.Sprintf("%s:status-settlements:%s", k.Namespace(), status)
}

func (k keygen) SyncCursor() string {
	return fmt.Sprintf("%s:sync-cursor", k.Namespace())
}

func (k keygen) TPSL(userID, id string) string {
	return fmt.Sprintf("%s:tpsl:%s:%s", k.Namespace(), userID, id)
}

func (k keygen) UserTPSL(userID string) string {
	return fmt.Sprintf("%s:user-tpsl:%s", k.Namespace(), userID)
}

func (k keygen) GroupTPSL(userID, groupID string) string {
	return fmt.Sprintf("%s:group-tpsl:%s:%s", k.Namespace(), userID, groupID)
}

func (k keygen) WaitingTPSL() string {
	return fmt.Sprintf("%s:waiting-tpsl", k.Namespace())
}
