package proto

type Order struct {
	// 订单ID
	OrderID string `json:"orderID" example:"00ed4bf26f1e48e79f3e4d0b430fe380"`
}

type OrderResponse struct {
	// 订单ID
	OrderID string `json:"orderID" example:"00ed4bf26f1e48e79f3e4d0b430fe380"`
	// 创建时间
	CreatedAt string `json:"createdAt" example:"1652174442"`
	// 状态（可选值: pending | sent | succ | fail | error）
	Status string `json:"status" example:"succ"`
	// 重试次数
	NumberOfRetries int64 `json:"numberOfRetries,omitempty" example:"1"`
	// 更新时间
	UpdatedAt string `json:"updatedAt,omitempty" example:"1652672618"`
	// Nonce 值
	Nonce string `json:"nonce,omitempty" example:"9159"`
	// 交易Hash
	TxHash string `json:"txHash,omitempty" example:"0x66e3076f604491c0944b3c885d451424fd644c4ebf61c333e0d4622d567af38b"`
	// 错误或失败原因（仅当 Status 为: fail 或 error 时有效）
	Reason string `json:"reason,omitempty" example:""`
}
