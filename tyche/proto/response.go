package proto

type Response struct {
	Code    int         `json:"code" example:"200"` // 错误码, 200-成功， 其它为失败
	Message string      `json:"msg" example:"succ"` // 错误消息
	Data    interface{} `json:"data"`               // 数据对象
	//Data    interface{} `json:"data" swaggertype:"string" example:"any{}"` // 数据对象
}
