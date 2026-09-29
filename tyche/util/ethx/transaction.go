package ethx

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func Unmarshal(txData string) (*types.Transaction, error) {
	bytes, err := hexutil.Decode(txData)
	if err != nil {
		return nil, err
	}

	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(bytes); err != nil {
		return nil, err
	}

	return tx, nil
}
