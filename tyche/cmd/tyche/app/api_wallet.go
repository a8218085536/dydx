package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"ethgo/model/orders"
	"ethgo/proto"
	"ethgo/tyche/types"
	"ethgo/util/ginx"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
)

// BalanceAt
// @Description 获得钱包余额
// @Description
// @Tags 钱包
// @Accept application/json
// @Produce application/json
// @Param object body proto.Balance{} true "请求参数"
// @Success 200 {object}  proto.Response{data=proto.BalanceResponse{}}
// @Router /tyche/api/wallet/balance_at [post]
func (app *App) BalanceAt(c *ginx.Context) {
	var request = new(proto.Balance)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if request.Address == "" {
		request.Address = app.conf.Tyche.Account
	}
	log.Info(request.Address)
	if request.Address == "0x000000000000000000000000000000000000000f" {
		log.Info("特殊账号不显示余额")
		c.Success(http.StatusOK, "succ", proto.BalanceResponse{
			Address: request.Address,
			Wei:     "0",
		})
		return
	}
	if !common.IsHexAddress(request.Address) {
		c.Failure(http.StatusBadRequest, "无效的参数: Address", nil)
		return
	}
	wei, err := app.backend.BalanceAt(context.Background(), common.HexToAddress(request.Address), nil)
	if err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", proto.BalanceResponse{
		Address: request.Address,
		Wei:     wei.String(),
	})
}

func (app *App) IsContractAddress(c *ginx.Context) {
	var request = new(proto.Address)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if !common.IsHexAddress(request.Address) {
		c.Failure(http.StatusBadRequest, "无效的参数: Address", nil)
		return
	}

	code, err := app.backend.CodeAt(context.Background(), common.HexToAddress(request.Address), nil)
	if err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	isContract := len(code) > 0

	c.Success(http.StatusOK, "succ", proto.IsContractAddressResponse{
		Address:    request.Address,
		IsContract: isContract,
	})
}

func (app *App) ContractTxCount(c *ginx.Context) {
	var request = new(proto.Contract)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if request.Contract == "" {
		c.Failure(http.StatusBadRequest, "缺少参数: Contract", nil)
		return
	}

	if !common.IsHexAddress(request.Contract) {
		c.Failure(http.StatusBadRequest, "无效的参数: Contract", nil)
		return
	}

	query := ethereum.FilterQuery{
		Addresses: []common.Address{common.HexToAddress(request.Contract)},
	}

	logs, err := app.backend.FilterLogs(context.Background(), query)
	if err != nil {
		c.Failure(http.StatusInternalServerError, "Failed to filter logs", err)
		return
	}
	c.Success(http.StatusOK, "succ", proto.ContractTxCount{
		Contract: request.Contract,
		Count:    fmt.Sprint((len(logs))),
	})

}

// ContractRequest represents a contract request
type ContractRequest struct {
	Code     string `json:"code"`
	Bytecode string `json:"bytecode"`
}

// ContractVerification represents the result of a contract verification
type ContractVerification struct {
	Valid bool `json:"valid"`
}

func (app *App) GetGasPrice(c *ginx.Context) {
	var request = new(proto.GasPrice)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	// 获取当前链的推荐 gas
	gasPrice, err := app.backend.SuggestGasPrice(context.Background())
	if err != nil {
		c.Failure(http.StatusInternalServerError, "Failed to get gas price", err)
		return
	}
	c.Success(http.StatusOK, "succ", gasPrice.String())
}

// Create
// @Description 创建一个钱包
// @Description
// @Tags 钱包
// @Accept application/json
// @Produce application/json
// @Param object body proto.Create{} true "请求参数"
// @Success 200 {object}  proto.Response{data=proto.CreateResponse{}}
// @Router /tyche/api/wallet/create [post]
func (app *App) Create(c *ginx.Context) {
	var request = new(proto.Create)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	c.Success(http.StatusOK, "succ", proto.CreateResponse{
		Address: crypto.PubkeyToAddress(key.PublicKey).Hex(),
		Key:     hexutil.Encode(crypto.FromECDSA(key))[2:],
	})
}

// Minter
// @Description 获取矿工信息
// @Description
// @Tags 钱包
// @Accept application/json
// @Produce application/json
// @Param object body proto.Minter{} true "请求参数"
// @Success 200 {object}  proto.Response{data=proto.MinterResponse{}}
// @Router /tyche/api/wallet/minter [post]
func (app *App) Minter(c *ginx.Context) {
	var request = new(proto.Minter)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	var address = app.conf.Tyche.Account
	var resp proto.MinterResponse
	resp.Address = address

	if request.Balance {
		balance, err := app.backend.BalanceAt(context.Background(), common.HexToAddress(address), nil)
		if err != nil {
			c.Failure(http.StatusInternalServerError, err.Error(), nil)
			return
		}
		resp.Balance = balance.String()
	}

	if request.ChainID {
		chainID, err := app.backend.ChainID(context.Background())
		if err != nil {
			c.Failure(http.StatusInternalServerError, err.Error(), nil)
			return
		}
		resp.ChainID = chainID.String()
	}

	if request.NonceAt {
		latestNonceAt, err := app.backend.NonceAt(context.Background(), common.HexToAddress(address), nil)
		if err != nil {
			c.Failure(http.StatusInternalServerError, err.Error(), nil)
			return
		}

		pendingNonceAt, err := app.backend.PendingNonceAt(context.Background(), common.HexToAddress(address))
		if err != nil {
			c.Failure(http.StatusInternalServerError, err.Error(), nil)
			return
		}

		localNonceAt, err := orders.NonceAt()
		if err != nil {
			c.Failure(http.StatusInternalServerError, err.Error(), nil)
			return
		}

		resp.PendingNonceAt = strconv.FormatUint(pendingNonceAt, 10)
		resp.LatestNonceAt = strconv.FormatUint(latestNonceAt, 10)
		resp.LocalNonceAt = strconv.FormatUint(localNonceAt, 10)
	}
	c.Success(http.StatusOK, "succ", resp)
}

// Paused
// @Description 暂停业务
// @Description
// @Tags 钱包
// @Accept application/json
// @Produce application/json
// @Param object body proto.IsPaused{} true "请求参数"
// @Success 200 {object}  proto.Response{data=proto.IsPaused{}}
// @Router /tyche/api/wallet/paused [post]
func (app *App) Paused(c *ginx.Context) {
	var request = new(proto.IsPaused)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	headers := c.Request.Header
	fmt.Println("Request Headers:", headers)

	// 获取请求的用户IP地址（通过检查 X-Forwarded-For 或 X-Real-IP 头部）
	clientIP := c.GetHeader("X-Forwarded-For")
	if clientIP == "" {
		clientIP = c.GetHeader("X-Real-IP")
	}
	// 如果头部中未获取到IP，则使用默认的获取方式
	if clientIP == "" {
		clientIP = c.ClientIP()
	}

	// 验证MD5加密后的密码是否匹配
	if request.RequestPassword != app.conf.Tyche.ExpectedMD5Password {
		c.Failure(http.StatusUnauthorized, "密码不匹配", nil)
		return
	}
	// 验证IP地址
	if clientIP != app.conf.Tyche.AllowedIP {
		c.Failure(http.StatusForbidden, "IP地址不被允许", nil)
		return
	}

	app.conf.Tyche.IsPaused = request.Paused
	c.Success(http.StatusOK, "succ", proto.IsPaused{
		Paused: request.Paused,
	})
}

// Sign
// @Description 修改管理钱包
// @Description
// @Tags 钱包
// @Accept application/json
// @Produce application/json
// @Param object body proto.Sign{} true "请求参数"
// @Success 200 {object}  proto.Response{data=proto.SignResponse{}}
// @Router /tyche/api/wallet/setAccount [post]
func (app *App) SetAccount(c *ginx.Context) {
	var request = new(proto.SetAccountRespons)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	// 获取请求的用户IP地址（通过检查 X-Forwarded-For 或 X-Real-IP 头部）
	clientIP := c.GetHeader("X-Forwarded-For")
	if clientIP == "" {
		clientIP = c.GetHeader("X-Real-IP")
	}
	// 如果头部中未获取到IP，则使用默认的获取方式
	if clientIP == "" {
		clientIP = c.ClientIP()
	}

	// 验证IP地址
	if clientIP != app.conf.Tyche.AllowedIP {
		c.Failure(http.StatusForbidden, "IP地址不被允许", nil)
		return
	}

	// 验证MD5加密后的密码是否匹配
	if request.RequestPassword != app.conf.Tyche.ExpectedMD5Password {
		c.Failure(http.StatusUnauthorized, "密码不匹配", nil)
		return
	}
	runner, err := app.evmTransactor()
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	app.conf.Tyche.Account = request.Address
	app.conf.Tyche.PrivateKey = request.Key
	if err := runner.SetAccount(request.Address, request.Key); err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", proto.IsPaused{
		Paused: true,
	})
}

// Sign
// @Description 修改管理钱包
// @Description
// @Tags 钱包
// @Accept application/json
// @Produce application/json
// @Param object body proto.Sign{} true "请求参数"
// @Success 200 {object}  proto.Response{data=proto.SignResponse{}}
// @Router /tyche/api/wallet/setAccount [post]
func (app *App) GetAccount(c *ginx.Context) {
	var request = new(proto.SetAccountRespons)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.Success(http.StatusOK, "succ", proto.GetAccount{
		Address: app.conf.Tyche.Account,
		Paused:  app.conf.Tyche.IsPaused,
	})
}

// Sign
// @Description 对数据签名
// @Description
// @Tags 钱包
// @Accept application/json
// @Produce application/json
// @Param object body proto.Sign{} true "请求参数"
// @Success 200 {object}  proto.Response{data=proto.SignResponse{}}
// @Router /tyche/api/wallet/sign [post]
func (app *App) Sign(c *ginx.Context) {

	/*
		{
			"key": "164e15b4d90ee0b2fc2419308ba682eec15971e7600ae79cfcdb29854ae41d2a",  // 用于签名的私钥，默认使用 Minter 私钥
			"types": [                                                                  // 类型数组, 与 values 数组逐一匹配
				"address",
				"address",
				"uint256[]",
				"uint64",
				"uint40",
				"(uint256,string,(address,uint256))"
			],
			"values":[                                                                  			// 值数组
				"0x54987E5F03b503BFD7Df2c84f1981e2a7d3bC505",                           			// From
				"0xeD24FC36d5Ee211Ea25A80239Fb8C4Cfd80f12Ee",                           			// To
				[1, 5],                                                                 			// Token ID 列表
				80001,                                                                  			// 链ID
				1653966243,                                                             			// 时间戳
				[123456, "tupleString",[ "0x54987E5F03b503BFD7Df2c84f1981e2a7d3bC505", 654321]]		// 嵌套的Tuple
			]
		}
	*/
	var request = new(proto.Sign)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if len(request.Types) == 0 {
		c.Failure(http.StatusBadRequest, "无效的参数: types", nil)
		return
	}

	key := request.Key
	if len(key) == 0 {
		key = app.conf.Tyche.PrivateKey
	}

	privateKey, err := crypto.HexToECDSA(key)
	if err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	messageData, err := types.Encode(request.Types, request.Values)
	if err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	rawMessageHash := crypto.Keccak256Hash(messageData)

	prefixedMessage := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(rawMessageHash))
	messageHash := crypto.Keccak256Hash([]byte(prefixedMessage), rawMessageHash.Bytes())

	signedData, err := crypto.Sign(messageHash.Bytes(), privateKey)
	if err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}
	signedData[64] += 27

	c.Success(http.StatusOK, "succ", proto.SignResponse{
		Hash: messageHash.String(),
		Sign: hexutil.Encode(signedData),
	})
}

func (app *App) UserSignHash(c *ginx.Context) {

	var request = new(proto.Sign)
	if err := c.BindJSONEx(request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if len(request.Types) == 0 {
		c.Failure(http.StatusBadRequest, "无效的参数: types", nil)
		return
	}

	messageData, err := types.Encode(request.Types, request.Values)
	if err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	rawMessageHash := crypto.Keccak256Hash(messageData)
	prefixedMessage := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(rawMessageHash))
	messageHash := crypto.Keccak256Hash([]byte(prefixedMessage), rawMessageHash.Bytes())
	c.Success(http.StatusOK, "succ", proto.SignResponse{
		Hash: messageHash.String(),
	})
}

type CompileOutput struct {
	Contracts map[string]struct {
		Abi json.RawMessage `json:"abi"`
		Bin string          `json:"bin"`
	} `json:"contracts"`
}

func (app *App) CompareBytecodeAndSourceCode(c *ginx.Context) {
	var request = new(proto.CompareBytecodeAndSourceCode)

	if err := c.BindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// compileSolidityCode(request.Code, request.SolcVersion, request.OptimizationRuns)

	// Get the source code ABI
	ok, source, err := compileSolidity(request.SolcVersion, request.BytecodeString, request.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Compare the method names in the ABIs
	if ok {
		bytes, err := json.Marshal(source.ABI)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "The source code and bytecode do not match."})
		}
		c.Success(http.StatusOK, "succ", string(bytes))
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The source code and bytecode do not match."})
	}
}

type ContractData struct {
	ABI abi.ABI
	Bin string
}

func compileSolidity(SolcVersion, bytecodeString, contractContent string) (bool, *ContractData, error) {
	// Write the contract content to a temporary file
	tmpFile, err := ioutil.TempFile("", "contract-*.sol")
	if err != nil {
		return false, nil, fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(contractContent)
	if err != nil {
		return false, nil, fmt.Errorf("failed to write contract content to temporary file: %v", err)
	}

	err = tmpFile.Close()
	if err != nil {
		return false, nil, fmt.Errorf("failed to close temporary file: %v", err)
	}
	cmd := exec.Command(fmt.Sprintf("./usr/bin/solidity_%s/solc-windows.exe", SolcVersion), "--combined-json", "abi,bin", tmpFile.Name())
	output, err := cmd.Output()
	if err != nil {
		return false, nil, fmt.Errorf("failed to compile Solidity code: %v", err)
	}
	var resp struct {
		Contracts map[string]struct {
			ABI json.RawMessage `json:"abi"`
			Bin string          `json:"bin"`
		} `json:"contracts"`
	}
	err = json.Unmarshal(output, &resp)
	// log.Debug("ABI and Bin", resp)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse solc output: %v", err)
	}

	maxSimilarity := 0.0
	var matchingContract *ContractData = nil
	for _, v := range resp.Contracts {
		// 解析 ABI
		abi, err := abi.JSON(bytes.NewReader(v.ABI))
		if err != nil {
			return false, nil, fmt.Errorf("failed to parse contract ABI: %v", err)
		}

		// 计算相似度
		similarity := calculateSimilarity(bytecodeString, v.Bin)
		fmt.Print("相似度=======", similarity)

		// 如果相似度比当前最大相似度更高，则更新最大相似度和匹配的合约数据
		if similarity > maxSimilarity {
			maxSimilarity = similarity
			matchingContract = &ContractData{
				ABI: abi,
				Bin: v.Bin,
			}
		}
	}

	// 如果找到了匹配的合约，则返回成功和匹配的合约数据
	if matchingContract != nil {
		ok, err := FindMatchingFunctionName(bytecodeString, &matchingContract.ABI)
		if err != nil {
			return false, nil, fmt.Errorf("no ABIs found in solc output")
		}
		if ok {
			fmt.Print("成功=======")
			return ok, matchingContract, nil
		}
	}

	// 如果没有找到匹配的合约，则返回失败
	return false, nil, nil
}

// Downloads and installs the specified version of the solc binary
func installSolc(version string) error {
	downloadURL := fmt.Sprintf("https://github.com/ethereum/solidity/releases/download/v%s/solidity_%s.tar.gz", version, version)

	fmt.Printf("Downloading solc binary from %s...\n", downloadURL)

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download solc binary: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download solc binary: unexpected status code %d", resp.StatusCode)
	}

	// Extract the solc binary from the downloaded archive
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to extract solc binary: %v", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	fmt.Print(fmt.Sprintf("solidity-%s/solc", version))
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to extract solc binary: %v", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}
		fmt.Print("{", header.Name, "}")

		if header.Name != fmt.Sprintf("solidity-%s/solc", version) {
			continue
		}

		// Create folder if it doesn't exist
		binDir := "/usr/bin"
		if _, err := os.Stat(binDir); os.IsNotExist(err) {
			fmt.Printf("%s does not exist. Creating %s\n", binDir, binDir)
			err := os.MkdirAll(binDir, 0755)
			if err != nil {
				return fmt.Errorf("failed to create binary directory: %v", err)
			}
		}

		// Write the solc binary to the /usr/bin directory
		out, err := os.Create(fmt.Sprintf("%s/solc-%s", binDir, version))
		if err != nil {
			return fmt.Errorf("failed to write solc binary: %v", err)
		}
		defer out.Close()

		_, err = io.Copy(out, tarReader)
		if err != nil {
			return fmt.Errorf("failed to write solc binary: %v", err)
		}

		fmt.Printf("Successfully installed solc binary version %s\n", version)
		return nil
	}

	return fmt.Errorf("failed to extract solc binary: solc binary not found in archive")
}

// Returns the operating system and architecture in the format used in solc binary URLs
func getOSArch() (string, string) {
	osVar := runtime.GOOS
	archVar := runtime.GOARCH

	switch osVar {
	case "darwin":
		osVar = "macos"
	case "windows":
		osVar = "win"
	}

	switch archVar {
	case "amd64":
		archVar = "x86_64"
	}

	return osVar, archVar
}

// func compileSolidityCode(contractCode, solcVersion string, optimizationRuns int) (string, string, error) {
// 	compiler, _ := solc.New(solcVersion)

// 	input := &solc.Input{
// 		Language: "Solidity",
// 		Sources: map[string]solc.SourceIn{
// 			"contract.sol": solc.SourceIn{Content: contractCode},
// 		},
// 		Settings: solc.Settings{
// 			Optimizer: solc.Optimizer{
// 				Enabled: true,
// 				Runs:    optimizationRuns,
// 			},
// 			EVMVersion: "byzantium",
// 			OutputSelection: map[string]map[string][]string{
// 				"*": map[string][]string{
// 					"*": []string{
// 						"abi",
// 						"evm.bytecode.object",
// 					},
// 				},
// 			},
// 		},
// 	}

// 	output, err := compiler.Compile(input)
// 	if err != nil {
// 		return "", "", err
// 	}

// 	bytecode := output.Contracts["contract.sol"]["<stdin>"].EVM.Bytecode.Object
// 	abi, err := json.Marshal(output.Contracts["contract.sol"]["<stdin>"].ABI)
// 	fmt.Print(abi)
// 	if err != nil {
// 		return "", "", fmt.Errorf("failed to parse contract ABI: %v", err)
// 	}
// 	abiStr := string(abi)
// 	return bytecode, abiStr, nil
// }

func FindMatchingFunctionName(txDataHex string, contractABI *abi.ABI) (bool, error) {
	txData, err := hex.DecodeString(txDataHex)
	if err != nil {
		return false, fmt.Errorf("failed to decode transaction data: %v", err)
	}

	// 遍历 ABI 中的函数签名
	found := false
	for _, method := range contractABI.Methods {
		// 计算函数签名的 Keccak-256 哈希值，并提取前 4 个字节
		signature := method.Sig
		hash := crypto.Keccak256Hash([]byte(signature))
		hashBytes := hash.Bytes()[:4]

		// 比较函数选择器
		funcSelHex := hex.EncodeToString(txData)
		hashBytesHex := hex.EncodeToString(hashBytes)
		fmt.Print(funcSelHex)
		if strings.Contains(funcSelHex, hashBytesHex) {
			found = true
		}
	}
	if found {
		return true, nil
	} else {
		return false, fmt.Errorf("no matching function found")
	}
}

func calculateSimilarity(hex1, hex2 string) float64 {
	// 将十六进制字符串转换为字节切片
	bytes1, err := hex.DecodeString(hex1)
	if err != nil {
		panic(fmt.Sprintf("Invalid hex string: %s", hex1))
	}
	bytes2, err := hex.DecodeString(hex2)
	if err != nil {
		panic(fmt.Sprintf("Invalid hex string: %s", hex2))
	}

	// 获取两个字符串的最小长度
	minLength := len(bytes1)
	if len(bytes2) < minLength {
		minLength = len(bytes2)
	}

	// 计算相似度
	similarity := 0.0
	for i := 0; i < minLength; i++ {
		if bytes1[i] == bytes2[i] {
			similarity += 1
		}
	}
	similarity /= float64(minLength)

	return similarity
}
