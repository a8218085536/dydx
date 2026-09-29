package proto

type Balance struct {
	// 钱包地址
	Address string `json:"address" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
}
type Address struct {
	// 钱包地址
	Address string `json:"address" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
}

type Contract struct {
	// 合约地址
	Contract string `json:"contract" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
}

type ContractData struct {
	// 合约地址 SourceCode
	Bytecode   string `json:"bytecode" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	SourceCode string `json:"sourceCode" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
}
type ABI struct {
	// 合约地址 SourceCode
	ABI string `json:"abi" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
}

type BalanceResponse struct {
	// 钱包地址
	Address string `json:"address" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	// 余额（WEI）
	Wei string `json:"wei" example:"49335849638413224831"`
}

type IsContractAddressResponse struct {
	// 钱包地址
	Address string `json:"address" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	// 余额（WEI）
	IsContract bool `json:"isContract" example:"49335849638413224831"`
}

type GasPrice struct {
}

type ContractTxCount struct {
	// 合约地址
	Contract string `json:"contract" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	// 交易总数
	Count string `json:"count" example:"49335849638413224831"`
}

type CompareBytecodeAndSourceCode struct {
	BytecodeString   string `json:"bytecodeString" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	Code             string `json:"code" example:"49335849638413224831"`
	SolcVersion      string `json:"solcVersion" example:"49335849638413224831"`
	OptimizationRuns int    `json:"optimizationRuns" example:"49335849638413224831"`
}

type ContractCreationTime struct {
	// 合约地址
	Contract string `json:"contract" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	// 交易总数
	TimeData uint64 `json:"timeData" example:"49335849638413224831"`
}

// Create create a wallet address
type Create struct {
}

// Create create a wallet address
type IsPaused struct {
	Paused bool `json:"paused" example:"false"`

	//MD5加密
	RequestPassword string `json:"requestPassword" example:"870cb32ae1445b2f736025d4dbf0546843a91e3cf2851bb07f5d14b3463d27b9"`
}

type SetAccountRespons struct {
	// 钱包地址
	Address string `json:"address" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	// 私钥
	Key string `json:"key" example:"870cb32ae1445b2f736025d4dbf0546843a91e3cf2851bb07f5d14b3463d27b9"`
	//MD5加密
	RequestPassword string `json:"requestPassword" example:"870cb32ae1445b2f736025d4dbf0546843a91e3cf2851bb07f5d14b3463d27b9"`
}

// Create create a wallet address
type GetAccount struct {
	// 钱包地址
	Address string `json:"address" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	Paused  bool   `json:"paused" example:"false"`
}

type CreateResponse struct {
	// 钱包地址
	Address string `json:"address" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	// 私钥
	Key string `json:"key" example:"870cb32ae1445b2f736025d4dbf0546843a91e3cf2851bb07f5d14b3463d27b9"`
}

type Sign struct {
	// 签名私钥
	Key string `json:"key" example:"870cb32ae1445b2f736025d4dbf0546843a91e3cf2851bb07f5d14b3463d27b9"`
	// 类型数组
	Types []string `json:"types" swaggertype:"array,string" example:"address,uint256,address,uint256,uint256"`
	// 值数组
	Values []interface{} `json:"values" swaggertype:"array,string" example:"0x00b5d3cb5fB6D2B69cE249707C398843d2Da5004,100000339,0xeD24FC36d5Ee211Ea25A80239Fb8C4Cfd80f12Ee,9000000000000000000,1653649525"`
}

type SignResponse struct {
	// 数据Hash
	Hash string `json:"hash" example:"0x6c97990b8853fe45851ba955af61231d2557114cac46943c1b0eef93d7023aa2"`
	// 签名
	Sign string `json:"sign" example:"0x6f7a8ccc3d18512700bf82a6a0ca3599b0a382744f93dc052bf13374f62f562f578aac9c27deff815e88328ee9f17e95150309a0bd145476aae3a435ed7f79f21c"`
}

type Minter struct {
	// 是否返回余额
	Balance bool `json:"balance,omitempty" example:"true"`
	// 是否返回链ID
	ChainID bool `json:"chainID,omitempty" example:"false"`
	// 是否返回 Nonce 信息
	NonceAt bool `json:"nonceAt,omitempty" example:"true"`
}

type MinterResponse struct {
	// 矿工地址
	Address string `json:"address" example:"0x51E72BDbA3A6Fc6337251581CB95625fa3A7767F"`
	// 余额（WEI）
	Balance string `json:"balance,omitempty" example:"49335849638413224831"`
	// 链ID
	ChainID string `json:"chainID,omitempty" example:"80001"`
	// 本地缓存的新交易可用 Nonce
	LocalNonceAt string `json:"localNonceAt,omitempty" example:"111"`
	// 已被链上确认的最新 Nonce
	LatestNonceAt string `json:"latestNonceAt,omitempty" example:"123"`
	// 链上返回的新交易可用 Nonce
	PendingNonceAt string `json:"pendingNonceAt,omitempty" example:"150"`
}
