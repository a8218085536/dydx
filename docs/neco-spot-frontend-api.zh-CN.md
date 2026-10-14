# NECO 现货前端接口文档

> 核对日期：2026-10-12
>
> 本文只描述 NECO 现货系统，不包含 dYdX/Tyche 合约交易接口。
>
> 状态标识：**已上线**、**待发布**、**规划中**。

## 1. 接入地址

| 环境 | HTTPS 地址 | WebSocket 地址 |
| --- | --- | --- |
| 测试 | `https://testapp.flydex.ai` | `wss://testapp.flydex.ai` |
| 正式 | `https://app.flydex.ai` | `wss://app.flydex.ai` |

前端应使用以下统一入口：

- OMS、登录、订单、账户：`/api/v1/*`、`/tyche/api/auth/*`
- 公共行情 REST：`/spot/api/*`
- 统一行情与用户 WebSocket：`/ws/v1`
- 旧公共行情兼容入口：`/spot/ws`（迁移期间保留）

除公开接口外，请求必须携带：

```http
Authorization: Bearer <Tyche sessionToken>
X-Neco-Address: <用户 NECO EVM 钱包地址>
X-Tyche-User-Id: <Tyche userId，可选>
```

## 2. 交易对与资产映射

链上资产保留 `N` 前缀，面向前端的市场名称已去掉前缀。

| marketId | 前端市场 | 链上基础资产 | quoteAssetId | baseAssetId |
| --- | --- | --- | --- | --- |
| `1` | `ETH/USDT` | `NETH` | `0` | `1` |
| `2` | `BNB/USDT` | `NBNB` | `0` | `2` |
| `3` | `BTC/USDT` | `NBTC` | `0` | `3` |

| assetId | 前端展示 | 链上资产 |
| --- | --- | --- |
| `0` | `USDT` | `USDT` |
| `1` | `ETH` | `NETH` |
| `2` | `BNB` | `NBNB` |
| `3` | `BTC` | `NBTC` |

指导价详情接口与其它市场接口一样，对前端统一使用 `ETH`、`BNB`、`BTC`；`NETH`、`NBNB`、`NBTC` 仅作为链内资产名称。

## 3. 数据约定

- OMS 中的价格、数量、余额、金额均使用十进制字符串，最多 8 位小数，例如 `"2.50000000"`。
- `omsOrderId`、`clusterOrderId`、`tradeId` 是 64 位 ID，JSON 中按字符串处理，前端禁止转成 `Number`。
- `userId`、`marketId`、`assetId` 是 JSON 数字。
- 时间字段名带 `Ms` 时为毫秒时间戳；K 线 `time` 为秒时间戳。
- 公共行情接口中的价格和数量是 JSON 数字，用于页面展示；资金计算以 OMS 的十进制字符串为准。
- 行情网关不会伪造市场数据；无成交时，成交相关字段返回 `null`，订单簿、成交与 K 线可返回空数组。

错误响应统一按以下结构处理：

```json
{
  "error": "错误说明",
  "code": "UNAUTHORIZED"
}
```

常见状态码：`400` 参数或业务拒绝、`401` 未登录、`403` 无权访问、`404` 不存在、`503` 后端暂不可用。

## 4. 钱包登录

### 4.1 获取签名挑战

**已上线**

```http
POST /tyche/api/auth/challenge
Content-Type: application/json
```

```json
{
  "necoAddress": "0x用户钱包地址"
}
```

```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "necoAddress": "0x用户钱包地址",
    "nonce": "challenge-uuid",
    "message": "Login to FlyDEX\nnecoAddress:0x用户钱包地址\nnonce:challenge-uuid"
  }
}
```

前端必须直接让钱包签名返回的 `data.message`，不要自行拼接或修改换行。

### 4.2 提交签名登录

**已上线**

```http
POST /tyche/api/auth/login
Content-Type: application/json
```

```json
{
  "necoAddress": "0x用户钱包地址",
  "signature": "0x钱包签名"
}
```

```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "userId": "tyche-user-uuid",
    "sessionToken": "session-token",
    "necoAddress": "0x用户钱包地址",
    "hasDydxBinding": true,
    "dydxAddress": "dydx1..."
  }
}
```

现货前端保存 `sessionToken`、`userId`、`necoAddress`。现货请求使用 `sessionToken` 和 `necoAddress`，不需要再次签名登录。

### 4.3 获取现货用户身份

**已上线**

```http
GET /api/v1/auth/me
```

```json
{
  "userId": 100001,
  "username": "tyche-user-uuid"
}
```

这里的 `userId` 是 OMS 数字用户 ID，和 Tyche 返回的 UUID 不是同一个字段。账户接口中的 `{userId}` 应使用这个数字 ID。

## 5. 公共基础接口

### 5.1 服务健康状态

**已上线，无需登录**

```http
GET /api/v1/health
```

```json
{
  "status": "ok",
  "clusterConnected": true,
  "activeOrders": 0,
  "assetsProjectionReady": true
}
```

### 5.2 市场列表

**已上线，无需登录**

```http
GET /api/v1/markets
```

```json
[
  {
    "marketId": 1,
    "symbol": "ETH/USDT",
    "baseAsset": "ETH",
    "quoteAsset": "USDT",
    "tickSize": "0.10000000",
    "minPrice": "10.00000000",
    "maxPrice": "100000.00000000"
  }
]
```

`tickSize` 是下单价格步长，价格必须在 `minPrice` 与 `maxPrice` 之间。

### 5.3 单个市场指导价

**已上线，无需登录**

```http
GET /api/v1/index-prices/{market}
```

前端使用以下 `{market}`：

- `ETH-USDT`
- `BNB-USDT`
- `BTC-USDT`

```json
{
  "market": "ETH/USDT",
  "baseAsset": "ETH",
  "quoteAsset": "USDT",
  "indexPrice": "2490.93000000",
  "provider": "binance",
  "sourceSymbol": "ETHUSDT",
  "updatedAt": "2026-10-10T03:45:05.797616193Z",
  "stale": false
}
```

`GET /api/v1/index-prices` 的列表形式当前不可用；前端暂时并发请求上述三个详情接口。

## 6. 公共行情 REST

### 6.1 全市场行情列表

**已上线，无需登录，以下三个地址返回相同数据**

```http
GET /spot/api/tickers
GET /spot/api/v1/market-tickers
GET /spot/api/v1/market-stats
```

```json
[
  {
    "marketId": 1,
    "market": "ETH/USDT",
    "lastPrice": null,
    "indexPrice": 2496.53,
    "priceChangePercent1h": null,
    "tradeCount24h": null,
    "openInterest": null,
    "fundingRate": null,
    "nextFundingTime": null,
    "high24h": null,
    "low24h": null,
    "volume24h": null,
    "timestamp": null,
    "priceChange24h": null,
    "priceChangePercent24h": null
  }
]
```

字段说明：

| 字段 | 含义 |
| --- | --- |
| `lastPrice` | 最新成交价；无成交时为 `null` |
| `indexPrice` | 外部指导价，来源与 `/api/v1/index-prices/{market}` 一致；首次取得有效值前为 `null` |
| `priceChangePercent1h` | 1 小时涨跌幅，百分数 |
| `priceChange24h` | 24 小时涨跌额 |
| `priceChangePercent24h` | 24 小时涨跌幅，百分数 |
| `high24h` / `low24h` | 24 小时最高价、最低价 |
| `volume24h` | 24 小时成交量 |
| `tradeCount24h` | 24 小时成交笔数 |
| `timestamp` | 最新成交行情时间，毫秒；无成交时为 `null` |
| `openInterest` / `fundingRate` | 现货不适用，固定为 `null` |

前端建议只使用 `/spot/api/v1/market-tickers`，另外两个保留为兼容别名。

指导价映射为：`ETH/USDT -> NETH-USDT`、`BNB/USDT -> NBNB-USDT`、
`BTC/USDT -> NBTC-USDT`。指导价刷新失败时保留上一次有效值，不以 `0` 覆盖。

### 6.2 24 小时走势线

**已上线，无需登录**

```http
GET /spot/api/v1/sparklines?timePeriod=ONE_DAY
```

兼容地址：

```http
GET /spot/v4/sparklines?timePeriod=ONE_DAY
```

```json
{
  "ETH/USDT": ["2490.10", "2492.30"],
  "BNB/USDT": [],
  "BTC/USDT": ["82510.00", "82562.38"]
}
```

每个数组最多 24 个 1 小时收盘价；没有成交/K 线时返回空数组。当前只支持 `ONE_DAY`。

### 6.3 订单簿

**已上线，无需登录**

```http
GET /spot/api/orderbook?marketId=1
```

```json
{
  "type": "BOOK_SNAPSHOT",
  "marketId": 1,
  "market": "ETH/USDT",
  "timestamp": 1791603840932,
  "bidVersion": 0,
  "askVersion": 0,
  "version": 0,
  "bookVersion": 0,
  "bids": [
    { "price": 2490.1, "quantity": 2.5, "orderCount": 3 }
  ],
  "asks": []
}
```

### 6.4 最近成交

**已上线，无需登录**

```http
GET /spot/api/trades?marketId=1&limit=50
```

- `marketId`：传 `1`、`2`、`3`；传 `0` 或省略时为全市场成交。
- `limit`：默认 `50`，最大 `500`。

```json
{
  "type": "TRADES_BATCH",
  "marketId": 1,
  "market": "ETH/USDT",
  "timestamp": 1791603841386,
  "trades": [
    {
      "price": 2490.1,
      "quantity": 0.25,
      "tradeCount": 1,
      "side": "BUY",
      "timestamp": 1791603840000
    }
  ]
}
```

`side` 是主动成交方方向；旧历史记录可能没有该字段。

### 6.5 K 线

**已上线，无需登录**

```http
GET /spot/api/candles?marketId=1&interval=1m&limit=200
```

- `limit` 默认 `200`，最大 `500`。
- `interval` 支持 `1m`、`5m`、`15m`、`1h`、`4h`、`1d`。
- `before` 可选，Unix 秒时间戳；返回该时间点之前的 K 线，用于向前翻页。
- 未发生成交的时间桶沿用上一根收盘价，`volume=0`、`tradeCount=0`，不会伪造成交。

向前加载下一页时，把当前结果第一根的 `time` 作为 `before`：

```http
GET /spot/api/candles?marketId=1&interval=1m&limit=500&before=1791603780
```

```json
{
  "type": "CANDLE_HISTORY",
  "marketId": 1,
  "market": "ETH/USDT",
  "interval": "1m",
  "candles": [
    {
      "time": 1791603720,
      "open": 2490.0,
      "high": 2491.0,
      "low": 2489.5,
      "close": 2490.1,
      "volume": 12.5,
      "tradeCount": 8
    }
  ]
}
```

### 6.6 FDEX/USDT 旧版 K 线兼容接口

**已上线，无需登录**

这两个入口是 Nginx 对旧版 NECO Swap K 线协议的兼容路由，数据来自 FDEX/USDT 交易池。它们不是 `ETH/USDT`、`BNB/USDT`、`BTC/USDT` 三个现货市场的通用 K 线接口。

#### 6.6.1 查询最新 K 线

```http
GET /spot/api/kline/get?symbol=0x5093ADfc89661d2d288aDACDC0A8F87CaE048d6a&type=1&rows=200
```

- `symbol`：必填，FDEX/USDT 交易池合约地址，必须传上述值。
- `type`：必填，K 线周期，支持 `1`、`5`、`15`、`30`，单位为分钟。
- `rows`：必填，返回条数，取值 `1` 至 `1000`。

#### 6.6.2 向前翻页查询历史 K 线

```http
GET /spot/api/kline/history?symbol=0x5093ADfc89661d2d288aDACDC0A8F87CaE048d6a&type=1&lastTime=1791600000
```

- `symbol`、`type`：含义同上。
- `lastTime`：必填，Unix **秒**时间戳；返回该时间点之前的历史 K 线，用于向前翻页。

两个接口都使用以下返回结构：

```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "open": 0.803094490186749,
      "close": 0.8029414535465936,
      "high": 0.803094490186749,
      "low": 0.8029414535465936,
      "chg": -0.00019055869766937406,
      "amount": 193.537691,
      "time": 1791606540,
      "timestamp": "2026-10-10 04:29:00"
    }
  ]
}
```

- `chg = (close - open) / open`。
- `amount` 为该周期的 USDT 成交额。
- `time` 为 Unix 秒时间戳；`timestamp` 是服务器格式化时间。
- 缺少参数或参数不合法时，返回 `{"code":400,"message":"Bad Request","data":{}}`。

## 7. 账户与成交

以下接口均需要登录请求头。

### 7.1 查询账户余额

**已上线**

```http
GET /api/v1/accounts/{userId}
```

```json
{
  "userId": 100001,
  "assets": [
    {
      "asset": "USDT",
      "assetId": 0,
      "available": "100.00000000",
      "locked": "20.00000000",
      "total": "120.00000000"
    }
  ]
}
```

`available` 可用于下单或提现，`locked` 已被挂单占用，`total = available + locked`。

### 7.2 查询当前用户余额

**待发布**

```http
GET /api/v1/balances
```

该路由已存在于本地 OMS 源码，但当前服务器运行包尚未包含。发布前请继续使用 `GET /api/v1/accounts/{userId}`。

### 7.3 查询成交记录

**已上线**

```http
GET /api/v1/executions?limit=100&offset=0
```

```json
[
  {
    "tradeId": "331884039403208705",
    "omsOrderId": "331884039403208704",
    "userId": 100001,
    "marketId": 1,
    "side": "BUY",
    "price": "2490.10000000",
    "quantity": "0.25000000",
    "maker": false,
    "executedAtMs": 1791603840000
  }
]
```

### 7.4 查询净交易数量

**已上线**

```http
GET /api/v1/positions
```

```json
[
  {
    "userId": 100001,
    "marketId": 1,
    "netQuantity": "0.25000000"
  }
]
```

这是根据成交记录汇总的基础资产净数量，不是永续合约持仓；负数表示净卖出。

## 8. 订单接口

### 8.1 创建订单

**已上线，需要登录**

```http
POST /api/v1/orders
Content-Type: application/json
```

限价单示例：

```json
{
  "marketId": 1,
  "side": "BUY",
  "orderType": "LIMIT",
  "timeInForce": "GTC",
  "price": "2490.10000000",
  "quantity": "0.25000000",
  "clientOrderId": "frontend-unique-id"
}
```

主要字段：

| 字段 | 说明 |
| --- | --- |
| `side` | `BUY` 或 `SELL` |
| `orderType` | `LIMIT`、`MARKET`、`STOP_LOSS`、`STOP_LIMIT`、`TRAILING_STOP`、`ICEBERG` |
| `timeInForce` | `GTC`、`GTD`、`IOC`、`FOK` |
| `price` | 限价；市价单可为 `"0"` 或按后端当前约定省略 |
| `quantity` | 下单数量，必须大于 0 |
| `stopPrice` | 止损/止盈触发价，可选 |
| `trailingDelta` | 跟踪止损价差，可选 |
| `displayQuantity` | 冰山单展示数量，可选 |
| `expiresAtMs` | `GTD` 到期时间，可选 |
| `clientOrderId` | 前端幂等键，最长 64 字符 |

成功返回：

```json
{
  "accepted": true,
  "omsOrderId": "331884039403208704",
  "status": "PENDING_RISK",
  "rejectReason": null,
  "duplicate": false
}
```

相同用户重复提交仍处于活动状态的 `clientOrderId` 时，不会重复创建订单，返回 `200` 且 `duplicate=true`。

### 8.2 查询当前委托

**已上线，需要登录**

```http
GET /api/v1/orders?status=NEW
```

不传 `status` 时返回当前活动订单。订单状态包括：

```text
PENDING_RISK
PENDING_HOLD
PENDING_NEW
PENDING_TRIGGER
NEW
PARTIALLY_FILLED
FILLED
CANCELLED
REJECTED
EXPIRED
```

### 8.3 查询单个订单

**已上线，需要登录**

```http
GET /api/v1/orders/{omsOrderId}
```

### 8.4 查询历史订单

**已上线，需要登录**

```http
GET /api/v1/orders/history?status=FILLED&limit=100&offset=0
```

`limit` 默认 `100`，最大 `1000`。

### 8.5 修改订单

**已上线，需要登录**

```http
PUT /api/v1/orders/{omsOrderId}
Content-Type: application/json
```

```json
{
  "price": "2491.00000000",
  "quantity": "0.30000000"
}
```

这是撤单后重建订单的 cancel-and-replace 操作，`price`、`quantity` 至少传一个。

### 8.6 撤销订单

**已上线，需要登录**

```http
DELETE /api/v1/orders/{omsOrderId}
```

```json
{
  "accepted": true,
  "omsOrderId": "331884039403208704",
  "message": "cancel submitted"
}
```

## 9. NECO 提现

### 9.1 发起提现

**已上线，需要登录**

```http
POST /api/v1/neco/withdrawals
Content-Type: application/json
```

```json
{
  "clientWithdrawalId": "frontend-withdrawal-unique-id",
  "destinationAddress": "0x收款地址",
  "assetId": 1,
  "amount": "2.50000000"
}
```

```json
{
  "success": true,
  "duplicate": false,
  "withdrawalId": "withdrawal-uuid",
  "clientWithdrawalId": "frontend-withdrawal-unique-id",
  "userId": 100001,
  "destinationAddress": "0x收款地址",
  "assetId": 1,
  "amount": "2.50000000",
  "requestedAmount": "2.50000000",
  "withdrawalFee": "0.00375000",
  "netAmount": "2.49625000",
  "withdrawalFeeRate": "0.0015",
  "status": "reserved",
  "txHash": null
}
```

`clientWithdrawalId` 是幂等键，网络重试时必须保持不变，避免重复扣款。

提现手续费统一为 `0.15%`。账户扣除 `requestedAmount`，链上实际到账 `netAmount`；
若链上出款失败，系统退回完整的 `requestedAmount`。`amount` 在用户接口中等于申请金额。

### 9.2 查询提现状态

**已上线，需要登录**

```http
GET /api/v1/neco/withdrawals/{withdrawalId}
```

状态可能包括：

```text
reserved -> broadcasting -> broadcasted -> confirmed
                                  \-> failed -> refunding -> refunded
```

## 10. 统一 WebSocket

### 10.1 匿名公共行情

**已上线，无需登录**

```text
wss://testapp.flydex.ai/ws/v1
```

页面打开后直接建立匿名连接，并订阅当前市场和 K 线周期：

```json
{
  "op": "subscribe",
  "channels": ["orderbook", "trades", "candles"],
  "marketId": 1,
  "interval": "1m"
}
```

`interval` 支持 `1m`、`5m`、`15m`、`1h`、`4h`、`1d`。服务端先返回
`SUBSCRIBED`，再按“快照在前、实时更新在后”的顺序发送：

- `BOOK_SNAPSHOT`、`BOOK_DELTA`：完整订单簿及增量
- `TRADES_BATCH`：最近成交快照及实时成交
- `CANDLE_HISTORY`、`CANDLE_UPDATE`：指定周期的历史和实时 K 线
- `TICKER_STATS`：实时行情统计（上游提供时）

切换市场或周期时，在同一连接重新发送 `subscribe`，新订阅会替换旧订阅。
盘口检测到版本断层时也重新发送当前订阅，以取得新的完整快照。

### 10.2 登录后的订单与成交

登录后使用同一地址重连，并通过 WebSocket 子协议传递 `sessionToken`：

```ts
const ws = new WebSocket(
  "wss://testapp.flydex.ai/ws/v1",
  ["bearer", sessionToken]
);
```

不要把 token 放入 URL、查询参数或订阅 JSON。订阅内容增加私有频道：

```json
{
  "op": "subscribe",
  "channels": ["orderbook", "trades", "candles", "orders", "executions"],
  "marketId": 1,
  "interval": "1m"
}
```

客户端提交的 `userId` 不参与鉴权，服务端只使用登录会话中的用户身份。

订单状态仍使用兼容格式：直接返回订单对象，通过 `omsOrderId` 判断，不带
`type` 包装。成交频道先返回当前用户最近 100 条历史：

```json
{
  "type": "EXECUTION_HISTORY",
  "executions": []
}
```

新成交随后返回：

```json
{
  "type": "EXECUTION_REPORT",
  "execution": {
    "tradeId": "9001",
    "omsOrderId": "5001",
    "userId": 100001,
    "marketId": 1,
    "side": "BUY",
    "price": "2500.00000000",
    "quantity": "1.00000000",
    "maker": false,
    "executedAtMs": 1791603840000
  }
}
```

历史一定先于订阅期间到达的实时成交。一个用户打开多个标签页时，每个标签页都能
收到该用户事件；不同用户之间不会互相收到数据。

### 10.3 心跳、错误与兼容入口

```json
{ "op": "ping" }
```

返回 `{"type":"PONG"}`。匿名连接请求 `orders`、`executions` 或 `balances`
时返回 `ERROR/AUTH_REQUIRED`，但连接不会关闭，已订阅的公共行情继续推送。
其他错误码包括 `INVALID_CHANNEL`、`INVALID_MARKET`、`INVALID_INTERVAL`、
`UPSTREAM_UNAVAILABLE`、`HISTORY_UNAVAILABLE`。

`balances` 频道仍为预留状态，没有实时余额推送。旧
`wss://testapp.flydex.ai/spot/ws` 在迁移期间继续可用，但新前端只需要一个
`/ws/v1` 连接。

## 11. 后续给前端增加的接口

以下接口当前没有在线实现。这里记录前端需要的目标能力，不得按已上线接口调用；字段为接口草案，未知数据必须返回 `null` 或空数组，禁止伪造。

### 11.1 资产配置

**规划中**

```http
GET /api/v1/assets
```

```json
[
  {
    "assetId": 1,
    "symbol": "ETH",
    "chainSymbol": "NETH",
    "contractAddress": null,
    "decimals": null,
    "depositEnabled": null,
    "withdrawalEnabled": null
  }
]
```

用于获取资产名称、链上合约、精度和充提开关。后端未提供的数据保持 `null`。

### 11.2 费率配置

**已上线，无需登录**

```http
GET /api/v1/fees
```

```json
{
  "makerFeeRate": "0",
  "takerFeeRate": "0",
  "withdrawalFeeRate": "0.0015",
  "withdrawalFeePercent": "0.15",
  "withdrawalFees": [
    {"assetId": 0, "asset": "USDT", "feeRate": "0.0015", "feePercent": "0.15"},
    {"assetId": 1, "asset": "ETH", "feeRate": "0.0015", "feePercent": "0.15"},
    {"assetId": 2, "asset": "BNB", "feeRate": "0.0015", "feePercent": "0.15"},
    {"assetId": 3, "asset": "BTC", "feeRate": "0.0015", "feePercent": "0.15"}
  ]
}
```

现货成交不收 Maker、Taker 手续费，只记录成交；实际手续费仅在提现时收取。
前端应读取该接口，不要自行写死费率。公网地址：
`https://testapp.flydex.ai/spot/api/v1/fees`。

### 11.3 充值地址

**已上线，需要登录**

```http
GET /api/v1/deposit-addresses
```

```json
[
  {
    "chain": "NECO",
    "assetId": 0,
    "asset": "USDT",
    "chainAsset": "USDT",
    "address": "0x公共托管地址",
    "tokenAddress": "0xUSDT合约地址",
    "memo": null,
    "enabled": true
  },
  {
    "chain": "NECO",
    "assetId": 1,
    "asset": "ETH",
    "chainAsset": "NETH",
    "address": "0x公共托管地址",
    "tokenAddress": "0xNETH合约地址",
    "memo": null,
    "enabled": true
  }
]
```

接口实际返回 `USDT`、`ETH/NETH`、`BNB/NBNB`、`BTC/NBTC` 四项。四种资产共用公共托管地址，`tokenAddress` 是对应的 NECO ERC-20 合约地址；前端必须按用户所选资产使用对应合约发起转账。链适配器按登录钱包的转出地址识别用户，本接口不生成每用户独立地址。配置缺失或地址格式无效时返回 `503 UNAVAILABLE`。

### 11.4 充值记录

**规划中**

```http
GET /api/v1/deposits?assetId=&status=&limit=100&offset=0
GET /api/v1/deposits/{depositId}
```

建议返回字段：

```json
{
  "depositId": "deposit-id",
  "txHash": "0x...",
  "logIndex": 0,
  "assetId": 1,
  "asset": "ETH",
  "amount": "1.00000000",
  "fromAddress": "0x...",
  "toAddress": "0x...",
  "status": "credited",
  "confirmations": null,
  "createdAtMs": null,
  "creditedAtMs": null
}
```

链适配器已有充值扫描和内部入账记录，但尚未开放用户态充值历史接口。

### 11.5 指导价列表

**规划修正**

```http
GET /api/v1/index-prices
```

目标是一次返回三个市场。当前线上可按 `ETH-USDT`、`BNB-USDT`、`BTC-USDT` 查询详情，并统一输出 `ETH/USDT`、`BNB/USDT`、`BTC/USDT`；列表形式仍未上线。

### 11.6 成交与余额 WebSocket

**成交已上线；余额推送仍为规划状态**

目标订阅格式：

```json
{
  "op": "subscribe",
  "channels": ["orders", "executions", "balances"]
}
```

当前 `/ws/v1` 已推送 `orders`、`EXECUTION_HISTORY` 和
`EXECUTION_REPORT`。`balances` 尚未推送，余额仍通过以下接口读取：

```http
GET /api/v1/accounts/{userId}
```

## 12. 前端推荐调用顺序

公开行情与登录状态无关。页面打开后应立即执行以下步骤，不要等待钱包登录：

1. 加载公开市场列表、指导价和 `/spot/api/v1/market-tickers`。
2. 加载 `/spot/api/orderbook`、`/spot/api/trades`、`/spot/api/candles` 和 `/spot/api/v1/sparklines`。
3. 匿名连接 `/ws/v1`，订阅 `orderbook`、`trades`、`candles`。

只有用户进入账户、下单、充值或提现流程时才登录：

1. 调用 `/tyche/api/auth/challenge` 获取签名原文。
2. 钱包签名后调用 `/tyche/api/auth/login`。
3. 保存 `sessionToken`、Tyche `userId` 和 `necoAddress`。
4. 携带登录请求头调用 `/api/v1/auth/me`，取得 OMS 数字 `userId`。
5. 使用 `sessionToken` 重连同一个 `/ws/v1`，在公共频道之外订阅 `orders` 和 `executions`。
6. 使用 `/api/v1/accounts/{userId}`、`/api/v1/orders`、充值与提现接口构建用户功能；`/api/v1/executions` 作为成交历史 REST 兜底。

登录失败或用户未登录时，公开行情页面仍必须正常展示和持续刷新；只禁用用户资产与交易操作。
