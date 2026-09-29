package eth

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrWasSuccessful = errors.New("transaction was successful")
	ErrStillPending  = errors.New("transaction is still pending")
	ErrNilReason     = errors.New("nil reason returned")
)

type jsonError interface {
	error
	ErrorCode() int
	ErrorData() interface{}
}

// Parity errors
var (
	// Non-fatal
	parTooCheapToReplace    = regexp.MustCompile("^Transaction gas price .+is too low. There is another transaction with same nonce in the queue")
	parLimitReached         = "There are too many transactions in the queue. Your transaction was dropped due to limit. Try increasing the fee."
	parAlreadyImported      = "Transaction with the same hash was already imported."
	parNonceTooLow          = "Transaction nonce is too low. Try incrementing the nonce."
	parInsufficientGasPrice = regexp.MustCompile("^Transaction gas price is too low. It does not satisfy your node's minimal gas price")
	parInsufficientEth      = regexp.MustCompile("^(Insufficient funds. The account you tried to send transaction from does not have enough funds.|Insufficient balance for transaction.)")

	// Fatal
	parInsufficientGas  = regexp.MustCompile("^Transaction gas is too low. There is not enough gas to cover minimal cost of the transaction")
	parGasLimitExceeded = regexp.MustCompile("^Transaction cost exceeds current gas limit. Limit:")
	parInvalidSignature = regexp.MustCompile("^Invalid signature")
	parInvalidGasLimit  = "Supplied gas is beyond limit."
	parSenderBanned     = "Sender is banned in local queue."
	parRecipientBanned  = "Recipient is banned in local queue."
	parCodeBanned       = "Code is banned in local queue."
	parNotAllowed       = "Transaction is not permitted."
	parTooBig           = "Transaction is too big, see chain specification for the limit."
	parInvalidRlp       = regexp.MustCompile("^Invalid RLP data:")
)

// Geth and geth-compatible errors
var (
	gethNonceTooLow                       = regexp.MustCompile(`(: |^)nonce too low$`)
	gethReplacementTransactionUnderpriced = regexp.MustCompile(`(: |^)replacement transaction underpriced$`)
	gethKnownTransaction                  = regexp.MustCompile(`(: |^)(?i)(known transaction|already known)`)
	gethTransactionUnderpriced            = regexp.MustCompile(`(: |^)transaction underpriced$`)
	gethInsufficientEth                   = regexp.MustCompile(`(: |^)(insufficient funds for transfer|insufficient funds for gas \* price \+ value|insufficient balance for transfer)$`)
	gethTxFeeExceedsCap                   = regexp.MustCompile(`(: |^)tx fee \([0-9\.]+ ether\) exceeds the configured cap \([0-9\.]+ ether\)$`)

	// Fatal Errors
	// See: https://github.com/ethereum/go-ethereum/blob/b9df7ecdc3d3685180ceb29665bab59e9f614da5/core/tx_pool.go#L516
	gethFatal = regexp.MustCompile(`(: |^)(exceeds block gas limit|invalid sender|negative value|oversized data|gas uint64 overflow|intrinsic gas too low|nonce too high)$`)
)

// Geth/parity returns these errors if the transaction failed in such a way that:
// 1. It will never be included into a block as a result of this send
// 2. Resending the transaction at a different gas price will never change the outcome
func IsFatalSendError(err error) bool {
	if err == nil {
		return false
	}
	str := err.Error()
	return isGethFatal(str) || isParityFatal(str)
}

// IsReplacementUnderpriced indicates that a transaction already exists in the mempool with this nonce but a different gas price or payload
func IsReplacementUnderpriced(err error) bool {
	if err == nil {
		return false
	}

	str := err.Error()
	switch {
	case gethReplacementTransactionUnderpriced.MatchString(str):
		return true
	case parTooCheapToReplace.MatchString(str):
		return true
	default:
		return false
	}
}

func UnpackRevert(resp []byte, err error) (string, error) {
	if err == nil {
		reason, uerr := abi.UnpackRevert(resp)
		if uerr != nil {
			return fmt.Sprintf("failed to unpack revert: %s", string(resp)), nil
		}
		return reason, nil
	}

	if je, ok := err.(jsonError); ok {
		// Some RPC servers returns "revert" data as a hex encoded string, here
		// we're trying to parse
		str, _ := je.ErrorData().(string)

		reason, err := abi.UnpackRevert(common.FromHex(str))
		if err != nil {
			return fmt.Sprintf("failed to unpack revert: %s", str), nil
		}
		return reason, nil
	}
	return "", err
}

func IsRpcError(err error) bool {

	switch err.(type) {
	case jsonError:
		return false
	case *url.Error, *net.OpError, net.Error:
		return true
	default:
		return true
	}
}

// func mapError(err error) error {
// 	if err != nil {
// 		switch re := err.(type) {
// 		case *jsonrpc.Error:
// 			//fmt.Printf("jrResp.Error:%+v", re)
// 			switch re.Code {
// 			case JsonrpcErrorCodeTxPoolOverflow:
// 				return module.ErrSendFailByOverflow
// 			case JsonrpcErrorCodeSystem:
// 				if subEc, err := strconv.ParseInt(re.Message[1:5], 0, 32); err == nil {
// 					//TODO return JsonRPC Error
// 					switch subEc {
// 					case ExpiredTransactionError:
// 						return module.ErrSendFailByExpired
// 					case FutureTransactionError:
// 						return module.ErrSendFailByFuture
// 					case TransactionPoolOverflowError:
// 						return module.ErrSendFailByOverflow
// 					}
// 				}
// 			case JsonrpcErrorCodePending, JsonrpcErrorCodeExecuting:
// 				return module.ErrGetResultFailByPending
// 			}
// 		case *common.HttpError:
// 			fmt.Printf("*common.HttpError:%+v", re)
// 			return module.ErrConnectFail
// 		case *url.Error:
// 			if common.IsConnectRefusedError(re.Err) {
// 				//fmt.Printf("*url.Error:%+v", re)
// 				return module.ErrConnectFail
// 			}
// 		}
// 	}
// 	return err
// }

func FilterError(err error) error {
	switch {
	case err == nil:
		return nil
	case IsNonceTooLowError(err), IsTransactionAlreadyInMempool(err):
		// Nonce too low or transaction known errors are expected since
		// the SendTransaction may well have succeeded already
		return nil
	case IsReplacementUnderpriced(err):
		// It is possible that an external wallet can have messed with the account and
		// sent a transaction on this nonce. In this case, the onus is on the client
		// operator since this is explicitly unsupported.
		return nil
	default:
		return err
	}
}

func IsNonceTooLowError(err error) bool {
	if err == nil {
		return false
	}

	str := err.Error()
	switch {
	case gethNonceTooLow.MatchString(str):
		return true
	case str == parNonceTooLow:
		return true
	default:
		return false
	}
}

// Geth/parity returns this error if the transaction is already in the node's mempool
func IsTransactionAlreadyInMempool(err error) bool {
	if err == nil {
		return false
	}

	str := err.Error()
	switch {
	case gethKnownTransaction.MatchString(str):
		return true
	case str == parAlreadyImported:
		return true
	default:
		return false
	}
}

// IsTerminallyUnderpriced indicates that this transaction is so far
// underpriced the node won't even accept it in the first place
func IsTerminallyUnderpriced(err error) bool {
	if err == nil {
		return false
	}

	str := err.Error()
	switch {
	case gethTransactionUnderpriced.MatchString(str):
		return true
	case parInsufficientGasPrice.MatchString(str):
		return true
	default:
		return false
	}
}

func IsTemporarilyUnderpriced(err error) bool {
	return err != nil && err.Error() == parLimitReached
}

func IsInsufficientEth(err error) bool {
	if err == nil {
		return false
	}

	str := err.Error()
	switch {
	case gethInsufficientEth.MatchString(str):
		return true
	case parInsufficientEth.MatchString(str):
		return true
	default:
		return false
	}
}

// IsTooExpensive returns true if the transaction and gas price are combined in
// some way that makes the total transaction too expensive for the eth node to
// accept at all. No amount of retrying at this or higher gas prices can ever
// succeed.
func IsTooExpensive(err error) bool {
	if err == nil {
		return false
	}

	str := err.Error()

	return gethTxFeeExceedsCap.MatchString(str)
}

func isGethFatal(s string) bool {
	return gethFatal.MatchString(s)
}

// See: https://github.com/openethereum/openethereum/blob/master/rpc/src/v1/helpers/errors.rs#L420
func isParityFatal(s string) bool {
	return s == parInvalidGasLimit ||
		s == parSenderBanned ||
		s == parRecipientBanned ||
		s == parCodeBanned ||
		s == parNotAllowed ||
		s == parTooBig ||
		(parInsufficientGas.MatchString(s) ||
			parGasLimitExceeded.MatchString(s) ||
			parInvalidSignature.MatchString(s) ||
			parInvalidRlp.MatchString(s))
}
