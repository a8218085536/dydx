package proto

// Call executes a message call transaction, which is directly executed in the VM
// of the node, but never mined into the blockchain.
type Call struct {
	// 合约地址
	Address string `json:"address" example:"0xa19844250b2b37c8518cb837b58ffed67f2e915D"`
	// 方法名(大小写敏感)
	Method string `json:"method" example:"getDNA"`
	// 合约方法参数
	Args interface{} `json:"args" swaggertype:"object,string" example:"id:1020"`
}

// Transact invokes the (paid) contract method with params as input values.
// 订单ID
type Transact struct {
	OrderID string `json:"orderID" example:"ORDER_001"`
	// 合约地址
	Address string `json:"address" example:"0xa19844250b2b37c8518cb837b58ffed67f2e915D"`
	// 方法名(大小写敏感)
	Method string `json:"method" example:"mint"`
	// 合约方法参数
	Args interface{} `json:"args" swaggertype:"object,string" example:"to:0xa70a1a4fb9143e6e9ef8b44d01c98794626b21b3,ids:[]int{2001},amounts:[]int{12},data:nothing"`
}
