# NECO 现货做市账户 API 交接

## 1. 接入信息

- 测试环境 HTTPS：`https://testapp.flydex.ai`
- 统一 WebSocket：`wss://testapp.flydex.ai/ws/v1`
- 每个账户只有一个独立 `userId` 和 API Key，禁止共用密钥。
- 金额、价格、数量、余额均传十进制字符串，最多 8 位小数。
- `omsOrderId`、`tradeId` 等 64 位 ID 应按字符串保存，不能转成 JavaScript `Number`。

REST 私有接口统一携带：

```http
X-API-Key: <该账户的 API Key>
Content-Type: application/json
```

API Key 只从独立密钥文件或密钥管理服务读取，禁止放入 URL、日志、Git、截图或群聊。

## 2. 账户和市场

每个做市账户初始内部余额如下，资金只存在于现货 OMS/Assets Engine，不对应一次 NECO 充值：

| assetId | 内部资产 | 页面名称 | 初始可用余额 |
| --- | --- | --- | ---: |
| `0` | `USDT` | `USDT` | `1,000,000` |
| `1` | `NETH` | `ETH` | `1,000,000` |
| `2` | `NBNB` | `BNB` | `1,000,000` |
| `3` | `NBTC` | `BTC` | `1,000,000` |

| marketId | 市场 | baseAssetId | quoteAssetId |
| --- | --- | --- | --- |
| `1` | `ETH/USDT` | `1` | `0` |
| `2` | `BNB/USDT` | `2` | `0` |
| `3` | `BTC/USDT` | `3` | `0` |

`BUY` 消耗 USDT、获得基础币；`SELL` 消耗基础币、获得 USDT。`quantity` 始终是基础币数量。

## 3. 身份和余额

```http
GET /api/v1/auth/me
GET /api/v1/balances
GET /api/v1/accounts/{userId}
```

`/auth/me` 用于确认 API Key 绑定的 `userId`。`/balances` 自动读取当前 Key 的账户；路径形式只允许读取自己的 `userId`。余额字段：

```json
{
  "userId": 910001,
  "assets": [
    {
      "assetId": 0,
      "asset": "USDT",
      "available": "1000000.00000000",
      "locked": "0.00000000",
      "total": "1000000.00000000"
    }
  ]
}
```

`available` 可下单，`locked` 被活动订单占用，`total = available + locked`。挂单后余额进入 `locked`，撤单或成交结算后更新。

## 4. 下单

### 4.1 限价单

```http
POST /api/v1/orders
```

```json
{
  "marketId": 1,
  "side": "BUY",
  "orderType": "LIMIT",
  "timeInForce": "GTC",
  "price": "2490.10000000",
  "quantity": "0.25000000",
  "clientOrderId": "mm01-20261015-000001"
}
```

### 4.2 市价单

```json
{
  "marketId": 1,
  "side": "SELL",
  "orderType": "MARKET",
  "timeInForce": "IOC",
  "quantity": "0.25000000",
  "clientOrderId": "mm01-20261015-000002"
}
```

主要字段：

| 字段 | 说明 |
| --- | --- |
| `marketId` | `1`、`2`、`3` |
| `side` | `BUY` 或 `SELL` |
| `orderType` | 常用 `LIMIT`、`MARKET`；系统还支持止损、跟踪止损和冰山类型 |
| `timeInForce` | `GTC`、`GTD`、`IOC`、`FOK` |
| `price` | 限价单必填；市价单省略 |
| `quantity` | 基础币数量，必须大于 0 |
| `clientOrderId` | 客户端幂等键，最长 64 字符 |

成功时返回 `accepted=true`、`omsOrderId` 和状态。网络超时后必须用原 `clientOrderId` 重试；不能生成新 ID，否则可能重复下单。重复提交仍活动的 ID 返回原订单并带 `duplicate=true`。

## 5. 查询、改单和撤单

```http
GET    /api/v1/orders
GET    /api/v1/orders?status=NEW
GET    /api/v1/orders/{omsOrderId}
GET    /api/v1/orders/history?status=FILLED&limit=100&offset=0
PUT    /api/v1/orders/{omsOrderId}
DELETE /api/v1/orders/{omsOrderId}
GET    /api/v1/executions?limit=100&offset=0
GET    /api/v1/positions
```

改单请求至少提供一个字段：

```json
{
  "price": "2491.00000000",
  "quantity": "0.30000000"
}
```

这是撤单重建流程。订单状态主流程：

```text
PENDING_RISK -> PENDING_HOLD -> PENDING_NEW -> NEW
NEW -> PARTIALLY_FILLED -> FILLED
NEW/PARTIALLY_FILLED -> CANCELLED
任一准入阶段也可能 -> REJECTED
```

`positions` 是现货成交形成的基础资产净数量，不是永续合约持仓。

## 6. 公共行情 REST

以下接口不需要 API Key：

```http
GET /api/v1/markets
GET /api/v1/index-prices/ETH-USDT
GET /api/v1/index-prices/BNB-USDT
GET /api/v1/index-prices/BTC-USDT
GET /spot/api/v1/market-tickers
GET /spot/api/v1/market-stats
GET /spot/api/v1/sparklines?timePeriod=ONE_DAY
GET /spot/api/orderbook?marketId=1
GET /spot/api/trades?marketId=1&limit=50
GET /spot/api/candles?marketId=1&interval=1m&limit=200&before=<Unix秒>
```

K 线周期支持 `1m`、`5m`、`15m`、`1h`、`4h`、`1d`。无真实成交时可以返回空数组或 `null`，客户端不得用指导价伪造成交、成交量或 K 线。

## 7. WebSocket

公共行情可匿名连接：

```js
const ws = new WebSocket("wss://testapp.flydex.ai/ws/v1");
```

需要私有订单和成交时，以 WebSocket 子协议携带同一个 API Key：

```js
const ws = new WebSocket(
  "wss://testapp.flydex.ai/ws/v1",
  ["bearer", "<该账户的 API Key>"]
);
```

禁止把 Key 放进查询参数或订阅 JSON。订阅示例：

```json
{
  "op": "subscribe",
  "channels": ["orderbook", "trades", "candles", "orders", "executions"],
  "marketId": 1,
  "interval": "1m"
}
```

- 公共消息：`BOOK_SNAPSHOT`、`BOOK_DELTA`、`TRADES_BATCH`、`CANDLE_HISTORY`、`CANDLE_UPDATE`、`TICKER_STATS`。
- 私有订单消息保持订单对象格式；私有成交先发 `EXECUTION_HISTORY`，新成交再发 `EXECUTION_REPORT`。
- `balances` 频道目前仅预留，不推送实时余额；余额以 REST `/api/v1/balances` 为准。
- `{"op":"ping"}` 返回 `{"type":"PONG"}`。
- 断线重连后重新订阅，并用 REST 重新取得活动订单和余额；不要假定断线期间没有成交。

## 8. 明确禁止

做市 Key 对以下操作必须返回 `403`，程序不得尝试绕过：

```http
POST /api/v1/accounts/{userId}/deposit
POST /api/v1/accounts/{userId}/withdraw
POST /api/v1/neco/withdrawals
GET  /api/v1/neco/withdrawals/{withdrawalId}
任意其它 userId 的账户、订单、成交或余额接口
任意 /api/v1/admin/* 或风险控制接口
```

做市程序不能充值、提现、跨链、修改余额、操作其他账户或执行管理动作。补充资金由平台运维使用临时资金管理员 Key，通过 OMS/Assets Engine 单独完成；不会把管理员 Key 交给做市程序。

## 9. 错误和重试

- `400`：参数或业务拒绝，修正请求，不盲目重试。
- `401`：API Key 缺失、错误或已撤销，立即停止交易并报警。
- `403`：权限拒绝；若出现在正常自账户读写交易流程，立即停止并报警。
- `404`：订单不存在，或为防止枚举而隐藏了无权访问的订单。
- `503`：依赖暂不可用，可指数退避重试。
- 创建订单超时：保持原 `clientOrderId` 重试，然后查询活动/历史订单确认最终状态。
- 撤单超时：查询订单状态；只有订单仍活动时才重试撤单。

交接前由运维运行 `scripts/spot/verify-market-maker-account.ps1`，确认身份、四项余额和所有禁止路径；可选 `-TradingSmoke` 只挂一笔不穿价的小额限价单并立即撤销。
