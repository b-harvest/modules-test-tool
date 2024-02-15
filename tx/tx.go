package tx

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/b-harvest/modules-test-tool/client"

	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// Transaction is an object that has common fields when signing transaction.
type Transaction struct {
	Client *client.Client `json:"client"`
}

// NewTransaction returns new Transaction object.
func NewTransaction(client *client.Client) *Transaction {
	return &Transaction{
		Client: client,
	}
}

func (t *Transaction) MsgEthereumTx(unsignedTx *gethtypes.Transaction, ecdsaPrivKey *ecdsa.PrivateKey) (*evmtypes.MsgEthereumTx, error) {
	signedTx, err := gethtypes.SignTx(unsignedTx, gethtypes.NewEIP155Signer(big.NewInt(1)), ecdsaPrivKey)
	if err != nil {
		return nil, err
	}

	data, err := rlp.EncodeToBytes(signedTx)
	if err != nil {
		return nil, err
	}

	msg := &evmtypes.MsgEthereumTx{}
	if err := msg.UnmarshalBinary(data); err != nil {
		return nil, err
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	return msg, nil
}
