# FlyDEX 前端交易接口接入文档

本文档描述前端从 NECO 钱包签名登录，到自动创建/绑定 dYdX 代理钱包，再到用 NECO 链上�?DEDX 发起 dYdX 交易的完整调用方式�?
当前产品方向是：

```text
用户看到的是 NECO 钱包里的 DEDX
-> 用户想在 dYdX 上交�?-> Tyche �?NECO swap 价格�?DEDX 折算�?USD 交易额度
-> 用户按提示把缺少�?DEDX 转到 Tyche �?NECO 托管地址
-> NECO 监听确认后写�?Ledger
-> Tyche 通过 dYdX 代理钱包下单
-> 后台同步订单状态、成交、手续费、盈�?```

dYdX 只负责交易，不负责外部充�?提现。外部出入金入口只在 NECO 链�?
## 基础约定

Base URL 示例�?
```text
https://api.example.com
```

所有接口返�?HTTP 200，真实业务结果看 JSON 里的 `code`�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {}
}
```

常见 `code`�?
| code | 含义 | 前端处理 |
| --- | --- | --- |
| `200` | 成功 | 继续流程 |
| `400` | 参数错误或业务错�?| 展示 `msg`，不要盲目重�?|
| `401` | 登录签名失败或鉴权失�?| 重新登录 |
| `500` | 服务配置或内部异�?| 展示系统繁忙/维护中，并上�?|

注意：当前受保护接口使用�?
```http
X-Tyche-Internal-Key: <internalAPIKey>
```

这个 key 不能放到浏览器前端代码里。正式前端建议走�?
```text
浏览器前�?-> 前端服务/BFF/session 网关 -> Tyche 受保护接�?```

`/auth/login` 会返�?`sessionToken`，但当前 Tyche 受保护接口还没有直接使用 `Authorization: Bearer <sessionToken>` 做用户态鉴权�?
## 前端主流�?
```text
1. 连接 NECO 钱包
2. 请求登录挑战
3. 用户钱包签名 message
4. 提交签名登录
5. Tyche 自动创建/绑定 dYdX 代理钱包
6. 查询绑定和交易余额概�?7. 用户输入订单
8. 调用下单接口
9. 如果返回 funding_required，提示用户在 NECO 链转 DEDX
10. �?NECO 交易确认并被后台监听入账
11. 使用同一�?clientOrderId 再次调用下单接口
12. 轮询订单状态，展示 broadcasted / submitted / filled / rejected / cancelled 等结�?```

## 1. 获取登录挑战

```http
POST /tyche/api/auth/challenge
Content-Type: application/json
```

请求参数�?
```json
{
  "necoAddress": "0xUserNECOAddress"
}
```

成功返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "necoAddress": "0xUserNECOAddress",
    "nonce": "challenge-uuid",
    "message": "Login to FlyDEX\nnecoAddress:0xUserNECOAddress\nnonce:challenge-uuid"
  }
}
```

字段含义�?
| 字段 | 含义 |
| --- | --- |
| `necoAddress` | 用户 NECO 钱包地址 |
| `nonce` | 本次登录随机�?|
| `message` | 前端需要让钱包签名的原�?|

前端处理�?
```text
�?data.message 调用 NECO 钱包签名
签名结果传给 /tyche/api/auth/login
```

## 2. 签名登录

```http
POST /tyche/api/auth/login
Content-Type: application/json
```

请求参数�?
```json
{
  "necoAddress": "0xUserNECOAddress",
  "signature": "0xWalletSignature"
}
```

成功返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "userId": "user-uuid",
    "sessionToken": "session-token",
    "necoAddress": "0xUserNECOAddress",
    "hasDydxBinding": true,
    "dydxAddress": "dydx1proxywallet..."
  }
}
```

字段含义�?
| 字段 | 含义 |
| --- | --- |
| `userId` | Tyche 内部用户 ID，后续接口需要传 |
| `sessionToken` | 登录�?token，当前主要给前端保存，后续可接正式用户鉴�?|
| `hasDydxBinding` | 是否已有可用 dYdX 代理钱包 |
| `dydxAddress` | Tyche 为用户创�?绑定�?dYdX 代理钱包地址 |

登录时后台会自动做两件事�?
```text
如果用户之前已经往 Tyche �?NECO 托管地址转过 FDEX/USDT�?  自动认领 pending deposit 到这�?userId �?Ledger

如果用户还没�?dYdX 绑定钱包�?  自动创建 dYdX 代理钱包并绑�?```

失败返回示例�?
```json
{
  "code": 401,
  "msg": "auth invalid signature",
  "data": null
}
```

前端处理：让用户重新签名登录�?
## 3. 查询 dYdX 绑定概览

```http
POST /tyche/api/wallet-binding/overview
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求参数�?
```json
{
  "userId": "user-uuid"
}
```

成功返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "userId": "user-uuid",
    "hasActiveDYDXBinding": true,
    "binding": {
      "id": "binding-uuid",
      "userId": "user-uuid",
      "necoAddress": "0xUserNECOAddress",
      "dydxAddress": "dydx1proxywallet...",
      "status": "active"
    }
  }
}
```

字段含义�?
| 字段 | 含义 |
| --- | --- |
| `hasActiveDYDXBinding` | 是否存在可交易的 dYdX 代理钱包 |
| `binding.status` | `active` 表示可用，`pending` 表示还未完成绑定 |

正常登录流程下，前端一般不需要手动创建绑定；登录接口会自动处理�?
## 4. 手动绑定备用接口

只有自动创建绑定不可用，或者未来改成用户自�?dYdX 钱包时才需要�?
创建绑定�?
```http
POST /tyche/api/wallet-binding/create
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求参数�?
```json
{
  "userId": "user-uuid",
  "necoAddress": "0xUserNECOAddress",
  "dydxAddress": "dydx1userwallet..."
}
```

返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "id": "binding-uuid",
    "userId": "user-uuid",
    "necoAddress": "0xUserNECOAddress",
    "dydxAddress": "dydx1userwallet...",
    "nonce": "binding-nonce",
    "message": "Bind FlyDEX account\nuserId:user-uuid\nnecoAddress:0xUserNECOAddress\ndydxAddress:dydx1userwallet...\nnonce:binding-nonce",
    "status": "pending"
  }
}
```

验证绑定�?
```http
POST /tyche/api/wallet-binding/verify
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求参数�?
```json
{
  "bindingId": "binding-uuid",
  "necoSignature": "0xNecoWalletSignature",
  "dydxSignature": "dydx-wallet-signature"
}
```

返回中的 `status` �?`active` 表示绑定成功�?
## 5. 查询交易余额概览

```http
POST /tyche/api/trading/balance/overview
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求参数�?
```json
{
  "userId": "user-uuid",
  "necoAddress": "0xUserNECOAddress",
  "asset": "DEDX"
}
```

成功返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "userId": "user-uuid",
    "necoAddress": "0xUserNECOAddress",
    "asset": "DEDX",
    "necoTokenAddress": "0xDedxTokenOnNECO",
    "ledgerAvailable": "1000000000000000000",
    "ledgerFrozen": "0",
    "necoWalletAvailable": "5000000000000000000",
    "displayAvailable": "6000000000000000000"
  }
}
```

字段含义�?
| 字段 | 含义 |
| --- | --- |
| `ledgerAvailable` | 已经�?Tyche 监听确认并记到账本的可用 DEDX |
| `ledgerFrozen` | 提现或其他后台处理中冻结的金�?|
| `necoWalletAvailable` | 用户 NECO 钱包当前 DEDX 余额 |
| `displayAvailable` | 前端可展示的总可用感知余额，等于 `ledgerAvailable + necoWalletAvailable` |

金额说明�?
```text
FDEX/USDT 等链�?token 金额使用原子单位字符�?前端展示时按 token decimals 换算
```

## 6. 发起 dYdX 下单

```http
POST /tyche/api/trading/place-order
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求参数�?
```json
{
  "userId": "user-uuid",
  "necoAddress": "0xUserNECOAddress",
  "clientOrderId": "frontend-stable-order-id-001",
  "market": "ETH-USD",
  "side": "BUY",
  "orderType": "LIMIT",
  "price": "3000",
  "size": "0.01",
  "marginAsset": "DEDX",
  "marginAmount": ""
}
```

参数含义�?
| 参数 | 必填 | 含义 |
| --- | --- | --- |
| `userId` | �?| 登录返回的用�?ID |
| `necoAddress` | 建议�?| 用户 NECO 钱包地址，用于检查钱包余额和生成转账动作 |
| `clientOrderId` | �?| 前端生成的幂�?ID，同一笔订单重试必须复�?|
| `market` | �?| dYdX 市场，例�?`ETH-USD` |
| `side` | �?| `BUY` �?`SELL` |
| `orderType` | �?| 例如 `LIMIT` |
| `price` | �?| 委托价格，十进制字符�?|
| `size` | �?| 委托数量，十进制字符�?|
| `marginAsset` | 建议�?`DEDX` | 用户侧扣款资�?|
| `marginAmount` | 通常留空 | 留空时由 Tyche 根据 NECO swap 价格计算 |

Tyche 计算逻辑�?
```text
订单 USD 价�?= price * size
FDEX/USD 价格 = DEDX/NECO swap 价格 * NECO/USDT swap 价格
需�?DEDX = 订单 USD 价�?/ FDEX/USD 价格
```

前端不要自己决定最终需要扣多少 DEDX，应�?Tyche 返回�?`marginAmount`、`dedxUsdPrice`、`priceSnapshotId` 为准�?
### 6.1 资金已足够，订单提交成功

返回示例�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "id": "order-uuid",
    "userId": "user-uuid",
    "necoAddress": "0xUserNECOAddress",
    "clientOrderId": "frontend-stable-order-id-001",
    "market": "ETH-USD",
    "side": "BUY",
    "orderType": "LIMIT",
    "price": "3000",
    "size": "0.01",
    "marginAsset": "DEDX",
    "marginAmount": "30000000000000000000",
    "marginUsd": "30",
    "priceSnapshotId": "dedx-usd-snapshot-id",
    "dedxUsdPrice": "1",
    "status": "broadcasted"
  }
}
```

前端处理�?
```text
展示订单已提�?开始轮�?/tyche/api/trading/order/get �?/tyche/api/trading/order/list
```

### 6.2 需要用户先�?DEDX

如果 Ledger 里余额不够，但用�?NECO 钱包有足�?DEDX，会返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "id": "order-uuid",
    "userId": "user-uuid",
    "necoAddress": "0xUserNECOAddress",
    "clientOrderId": "frontend-stable-order-id-001",
    "market": "ETH-USD",
    "side": "BUY",
    "orderType": "LIMIT",
    "price": "3000",
    "size": "0.01",
    "marginAsset": "DEDX",
    "marginAmount": "30000000000000000000",
    "marginUsd": "30",
    "priceSnapshotId": "dedx-usd-snapshot-id",
    "dedxUsdPrice": "1",
    "fundingStatus": "wallet_action_required",
    "fundingAction": "transfer",
    "fundingAsset": "DEDX",
    "fundingAmount": "30000000000000000000",
    "fundingTokenAddress": "0xDedxTokenOnNECO",
    "fundingFromAddress": "0xUserNECOAddress",
    "fundingToAddress": "0xTycheNecoDepositAddress",
    "fundingReason": "transfer DEDX before submitting dydx order",
    "fundingWalletBalance": "50000000000000000000",
    "fundingLedgerAvailable": "0",
    "status": "funding_required"
  }
}
```

前端处理�?
```text
1. 弹出钱包转账
2. token = fundingTokenAddress
3. from = fundingFromAddress
4. to = fundingToAddress
5. amount = fundingAmount
6. 等待 NECO 链交易成�?7. 等后�?NECO 监听入账
8. 使用同一�?clientOrderId 再次调用 /tyche/api/trading/place-order
```

非常重要�?
```text
资金准备后重试必须复用同一�?clientOrderId
不要生成新的 clientOrderId
否则可能被认为是另一笔新订单
```

### 6.3 用户 NECO 钱包余额不足

返回示例�?
```json
{
  "code": 400,
  "msg": "trading funding wallet balance insufficient",
  "data": null
}
```

前端处理�?
```text
提示用户 NECO 钱包 DEDX 不足
引导用户先通过 swap 或充值获�?DEDX
```

### 6.4 dYdX 拒单或广播失�?
返回示例�?
```json
{
  "code": 400,
  "msg": "dydx order rejected",
  "data": null
}
```

前端处理�?
```text
展示失败原因
刷新订单详情和余�?不要自动重复提交新订�?```

后台会根据拒�?失败规则处理 Ledger 退款或状态回滚�?
## 7. 查询单笔订单

```http
POST /tyche/api/trading/order/get
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求参数二选一，推荐传 `clientOrderId`�?
```json
{
  "userId": "user-uuid",
  "clientOrderId": "frontend-stable-order-id-001",
  "orderId": ""
}
```

或：

```json
{
  "userId": "user-uuid",
  "orderId": "order-uuid"
}
```

成功返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "id": "order-uuid",
    "userId": "user-uuid",
    "clientOrderId": "frontend-stable-order-id-001",
    "market": "ETH-USD",
    "side": "BUY",
    "orderType": "LIMIT",
    "price": "3000",
    "size": "0.01",
    "marginAsset": "DEDX",
    "marginAmount": "30000000000000000000",
    "status": "partial_filled"
  }
}
```

## 8. 查询用户订单列表

```http
POST /tyche/api/trading/order/list
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求参数�?
```json
{
  "userId": "user-uuid"
}
```

成功返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": [
    {
      "id": "order-uuid",
      "clientOrderId": "frontend-stable-order-id-001",
      "market": "ETH-USD",
      "side": "BUY",
      "orderType": "LIMIT",
      "price": "3000",
      "size": "0.01",
      "status": "broadcasted"
    }
  ]
}
```

前端处理�?
```text
用于订单列表页、交易页底部当前委托/历史委托展示
```

## 9. 撤单

```http
POST /tyche/api/trading/cancel-order
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求参数�?
```json
{
  "userId": "user-uuid",
  "clientOrderId": "frontend-stable-order-id-001",
  "orderId": "order-uuid"
}
```

成功返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "id": "order-uuid",
    "userId": "user-uuid",
    "clientOrderId": "frontend-stable-order-id-001",
    "status": "cancel_pending"
  }
}
```

前端处理�?
```text
展示撤单�?继续轮询订单状�?最终可能变�?cancelled / partial_filled / filled / rejected
```

## 9.1 平仓

```http
POST /tyche/api/trading/close-position
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求参数�?
```json
{
  "userId": "user-uuid",
  "clientOrderId": "frontend-stable-close-order-id-001",
  "market": "ETH-USD",
  "orderType": "MARKET",
  "executionOrderType": "market",
  "size": ""
}
```

字段说明�?
| 字段 | 是否必填 | 含义 |
| --- | --- | --- |
| `userId` | �?| 当前登录用户 ID |
| `clientOrderId` | �?| 本次平仓动作的前端幂�?ID，重试必须复�?|
| `market` | �?| 要平仓的市场，例�?`ETH-USD` |
| `orderType` | �?| `MARKET` �?`LIMIT`，默认按普通订单规则处�?|
| `executionOrderType` | �?| 市价平仓建议�?`market` |
| `price` | 限价必填 | 限价平仓价格 |
| `referencePrice` | �?| 市价平仓参考价，未传时后台�?dYdX 行情取价 |
| `slippageBps` | �?| 市价平仓滑点，例�?`100` 表示 1% |
| `size` | �?| 不传或空字符串表示全平；传值表示部分平�?|

成功返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": {
    "id": "close-order-uuid",
    "userId": "user-uuid",
    "clientOrderId": "frontend-stable-close-order-id-001",
    "market": "ETH-USD",
    "side": "SELL",
    "orderType": "market",
    "size": "0.01",
    "reduceOnly": true,
    "closePosition": true,
    "dydxOrderTxHash": "D9A...",
    "status": "close_broadcasted"
  }
}
```

前端处理�?
```text
先调�?/tyche/api/dydx/sync/position/get 确认有持�?用户点击平仓后调�?close-position
返回 close_broadcasted / close_submitted 时展示平仓中
继续轮询 /tyche/api/trading/order/get
最终可能变�?closed / close_failed / close_cancelled / close_partial_filled
```

注意：平仓单�?dYdX `reduceOnly` 订单，不再要求用户追�?NECO 资金，也不会返回 `funding_required`�?
## 10. 订单状态含�?
| status | 含义 | 前端展示建议 |
| --- | --- | --- |
| `pending_submit` | Tyche 已扣�?准备提交 dYdX | 提交�?|
| `broadcasted` | 已广播到 dYdX，等待链/indexer 确认 | 广播�?|
| `submitted` | dYdX 已确认接收订�?| 已委�?|
| `funding_required` | 需要用户先�?NECO 链转 DEDX | 需要资金准�?|
| `cancel_pending` | 已发起撤单，等待 dYdX 确认 | 撤单�?|
| `rejected` | dYdX 拒单或提交失�?| 已拒�?失败 |
| `partial_filled` | 部分成交 | 部分成交 |
| `filled` | 完全成交 | 已成�?|
| `cancelled` | 已撤�?| 已撤�?|
| `close_pending` | Tyche 已准备提交平仓单 | 平仓提交�?|
| `close_broadcasted` | 平仓单已广播�?dYdX，等待链/indexer 确认 | 平仓广播�?|
| `close_submitted` | dYdX 已确认接收平仓单 | 平仓�?|
| `close_partial_filled` | 平仓单部分成�?| 部分平仓 |
| `closed` | 平仓完成 | 已平�?|
| `close_failed` | 平仓单被拒绝或提交失�?| 平仓失败 |
| `close_cancelled` | 平仓单被撤销 | 平仓已撤销 |

## 11. 资金准备状态含�?
| 字段 | 可能�?| 含义 |
| --- | --- | --- |
| `fundingStatus` | `wallet_action_required` | 需要用户钱包动�?|
| `fundingStatus` | `pending_confirmation` | 已提交链上动作，等待确认，预留状�?|
| `fundingStatus` | `ready` | 资金已准备好，预留状�?|
| `fundingAction` | `transfer` | 前端需要发�?NECO token 转账 |
| `fundingAction` | `approve` | 预留，当�?MVP 不使�?|
| `fundingAction` | `permit` | 预留，当�?MVP 不使�?|

当前 MVP 只使用：

```text
fundingStatus = wallet_action_required
fundingAction = transfer
```

## 12. dYdX 代理钱包手续�?
dYdX 代理钱包需要链�?gas 主币�?
```text
gas denom: adedx
```

前端不用给代理钱包打 gas，也不用显示这个步骤�?
后台职责�?
```text
下单前检查用�?dYdX 代理钱包 adedx 余额
余额不足时从 Tyche gas/custody 钱包补一点手续费
�?gas 成功后再�?dYdX 订单
```

如果�?gas 失败，前端只需要按普通下单失败处理�?
## 13. dYdX 同步查询接口

这些接口主要给管理端、调试页�?BFF 使用，普通交易页面不一定直接调用�?
### 查询成交事件

```http
POST /tyche/api/dydx/sync/fill/get
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求�?
```json
{
  "eventId": "fill-event-id"
}
```

返回字段�?
```json
{
  "eventId": "fill-event-id",
  "userId": "user-uuid",
  "orderId": "order-uuid",
  "market": "ETH-USD",
  "asset": "DEDX",
  "releasedMargin": "0",
  "fee": "10000000000000000",
  "realizedPnl": "0",
  "orderStatus": "filled"
}
```

### 查询仓位

```http
POST /tyche/api/dydx/sync/position/get
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求�?
```json
{
  "userId": "user-uuid",
  "market": "ETH-USD"
}
```

返回字段�?
```json
{
  "userId": "user-uuid",
  "market": "ETH-USD",
  "size": "0.01",
  "entryPrice": "3000",
  "markPrice": "3010",
  "unrealizedPnl": "0.1",
  "marginAsset": "DEDX",
  "margin": "30000000000000000000"
}
```

### 查询失败同步事件

```http
POST /tyche/api/dydx/sync/failures
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求�?
```json
{
  "userId": "user-uuid"
}
```

返回�?
```json
{
  "code": 200,
  "msg": "succ",
  "data": [
    {
      "eventId": "status-event-id",
      "userId": "user-uuid",
      "orderId": "order-uuid",
      "status": "rejected",
      "reason": "dydx rejected order",
      "refundAsset": "DEDX",
      "refundAmount": "30000000000000000000"
    }
  ]
}
```

### 手动重试失败事件

```http
POST /tyche/api/dydx/sync/retry
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

请求�?
```json
{
  "eventId": "status-event-id"
}
```

这个接口建议只给管理端使用�?
## 14. 前端必须遵守的幂等规�?
下单前生成稳定的 `clientOrderId`�?
```text
clientOrderId = 用户本地订单唯一 ID
```

规则�?
| 场景 | clientOrderId |
| --- | --- |
| 第一次下�?| 生成新的 |
| 返回 `funding_required` 后转账成�?| 复用同一�?|
| 网络超时后不确定是否成功 | 复用同一�?|
| 用户明确重新下一笔新订单 | 生成新的 |

不要因为用户完成 NECO 转账就生成新订单 ID�?
## 15. 前端展示建议

交易页建议展示：

```text
NECO 钱包 DEDX: necoWalletAvailable
已入账可交易 DEDX: ledgerAvailable
可感知总额: displayAvailable
本单预计占用 DEDX: marginAmount
本单 USD 价�? marginUsd
FDEX/USD 快照价格: dedxUsdPrice
```

用户点击下单时：

```text
如果直接 broadcasted:
  显示已提�?
如果 funding_required:
  显示需要转�?DEDX
  调起 NECO 钱包转账
  转账成功后等待确�?  用同一�?clientOrderId 自动或手动重试下�?
如果 rejected / 400:
  显示失败原因并刷新余�?```

## 16. 不需要前端处理的事项

以下动作�?Tyche 后台处理，前端不要直接做�?
```text
创建 dYdX 代理钱包私钥
保管 dYdX 代理钱包私钥
�?dYdX 代理钱包�?adedx gas
直接调用 dydxtrading sidecar
直接广播 dYdX 原生交易
计算最�?FDEX/USD 报价
�?Ledger
处理重复 NECO 充值事�?处理重复 dYdX fill/status 事件
拒单或撤单后的账本退�?```

## 17. 最小接入伪代码

```ts
async function loginWithNecoWallet(necoAddress: string) {
  const challenge = await post("/tyche/api/auth/challenge", { necoAddress });
  const signature = await wallet.signMessage(challenge.data.message);
  return post("/tyche/api/auth/login", { necoAddress, signature });
}

async function placeEthOrder(userId: string, necoAddress: string) {
  const clientOrderId = getOrCreateStableClientOrderId();

  const order = await protectedPost("/tyche/api/trading/place-order", {
    userId,
    necoAddress,
    clientOrderId,
    market: "ETH-USD",
    side: "BUY",
    orderType: "LIMIT",
    price: "3000",
    size: "0.01",
    marginAsset: "DEDX",
    marginAmount: ""
  });

  if (order.data.status === "funding_required") {
    await wallet.transferToken({
      token: order.data.fundingTokenAddress,
      to: order.data.fundingToAddress,
      amount: order.data.fundingAmount
    });

    await waitForNECOConfirmation();

    return protectedPost("/tyche/api/trading/place-order", {
      userId,
      necoAddress,
      clientOrderId,
      market: "ETH-USD",
      side: "BUY",
      orderType: "LIMIT",
      price: "3000",
      size: "0.01",
      marginAsset: "DEDX",
      marginAmount: ""
    });
  }

  return order;
}
```
## Close/Cancel Settlement API

After a close-position order is confirmed by dYdX reconcile, Tyche creates a close settlement record and pays DEDX on NECO exactly once.

After a funded order is cancelled and dYdX reconcile confirms the stateful order is gone, Tyche creates a cancel settlement record and pays DEDX on NECO exactly once. Cancel settlement is based on this order's own synced fills: if no fill exists, Tyche refunds the original margin; if fills exist, Tyche uses released margin, fee, and realized PnL accounting.

Tyche does not expose dYdX as a withdrawal surface. The dYdX subaccount value related to a settled close is marked `systemReserved=true`; the user-facing payout is the NECO DEDX transfer.

When `closeSettlementMode = "full_accounting"`, close-position settlement uses confirmed dYdX fill accounting instead of the original margin only:

```text
settlementUsd = sum(releasedMargin) + sum(realizedPnl) - sum(fee)
```

Tyche then converts `settlementUsd` to the configured NECO payout asset, normally DEDX, using the current NECO price route. If a close-position order has no synced fills, missing accounting fields, zero released margin, or an invalid final amount, Tyche creates `manual_review_required` and does not broadcast a NECO payout. A fully unfilled cancelled order can still refund the original margin because there are no fills, fees, or realized PnL.

### Query Settlement

```http
POST /tyche/api/trading/settlement/get
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

Request by settlement ID:

```json
{
  "userId": "user-uuid",
  "settlementId": "settlement-uuid"
}
```

Request by close order ID:

```json
{
  "userId": "user-uuid",
  "closeOrderId": "close-order-uuid"
}
```

Request by source order ID. Use this for cancel settlements:

```json
{
  "userId": "user-uuid",
  "orderId": "order-uuid"
}
```

### List User Settlements

```http
POST /tyche/api/trading/settlement/list
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

```json
{
  "userId": "user-uuid"
}
```

### Retry Settlement

```http
POST /tyche/api/trading/settlement/retry
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

```json
{
  "userId": "user-uuid",
  "settlementId": "settlement-uuid"
}
```

### Settlement Statuses

| Status | Meaning | Frontend behavior |
| --- | --- | --- |
| `pending` | Close confirmed and settlement record exists, payout not prepared yet | Keep polling or show processing |
| `dedx_credited` | Internal DEDX payout balance was created and frozen for withdrawal | Keep polling |
| `neco_payout_broadcasted` | NECO DEDX transfer was broadcast | Show payout transaction hash when available |
| `neco_payout_paid` | NECO receipt confirmed successful | Show settlement completed |
| `manual_review_required` | Accounting data is missing or ambiguous, such as raw-like `marginUsd` | Stop automatic retry and show manual review |
| `failed` | Payout failed before completion | Allow admin retry or manual handling |

## TP/SL Conditional Orders

TP/SL is not a normal opening order. It is a Tyche conditional close rule:

```text
frontend creates TP/SL rule
-> Tyche stores waiting_trigger
-> Tyche worker polls market price
-> when trigger price is crossed, Tyche calls close-position
-> close-position submits reduce-only IOC order
-> the sibling TP/SL rule in the same group is cancelled
```

Create TP/SL:

```http
POST /tyche/api/trading/tpsl/create
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

```json
{
  "userId": "user-uuid",
  "market": "ETH-USD",
  "size": "0.5",
  "takeProfitPrice": "2100",
  "stopLossPrice": "1900",
  "slippageBps": 100
}
```

Notes:

| Field | Meaning |
| --- | --- |
| `size` | Optional. Empty means close the whole current position. Must not exceed current position size. |
| `takeProfitPrice` | Optional, but at least one of TP/SL must be provided. |
| `stopLossPrice` | Optional, but at least one of TP/SL must be provided. |
| `slippageBps` | Optional. Used when Tyche submits the reduce-only IOC close order. |

Success response:

```json
{
  "groupId": "tpsl-group-uuid",
  "orders": [
    {
      "id": "take-profit-rule-id",
      "groupId": "tpsl-group-uuid",
      "userId": "user-uuid",
      "market": "ETH-USD",
      "kind": "take_profit",
      "positionSide": "long",
      "triggerPrice": "2100",
      "size": "0.5",
      "closeClientOrderId": "123456789",
      "status": "waiting_trigger"
    },
    {
      "id": "stop-loss-rule-id",
      "groupId": "tpsl-group-uuid",
      "kind": "stop_loss",
      "triggerPrice": "1900",
      "status": "waiting_trigger"
    }
  ]
}
```

List TP/SL:

```http
POST /tyche/api/trading/tpsl/list
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

```json
{
  "userId": "user-uuid"
}
```

Cancel TP/SL:

```http
POST /tyche/api/trading/tpsl/cancel
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

Cancel one rule:

```json
{
  "userId": "user-uuid",
  "id": "rule-id"
}
```

Cancel the whole TP/SL group:

```json
{
  "userId": "user-uuid",
  "groupId": "tpsl-group-uuid"
}
```

TP/SL statuses:

| status | Meaning |
| --- | --- |
| `waiting_trigger` | Waiting for trigger price. |
| `triggered` | Triggered and close-position order has been submitted. See `closeOrderId` and `closeClientOrderId`. |
| `cancelled` | Cancelled by user or by OCO sibling trigger. |
| `failed` | Trigger condition was met, but close-position submission failed. Check `reason`. |
