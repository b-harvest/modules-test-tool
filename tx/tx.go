package tx

import (
	"context"
	"fmt"

	"github.com/b-harvest/modules-test-tool/client"

	sdkclienttx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	exchangetypes "github.com/crescent-network/crescent/v5/x/exchange/types"
)

// Transaction is an object that has common fields when signing transaction.
type Transaction struct {
	Client   *client.Client `json:"client"`
	ChainID  string         `json:"chain_id"`
	GasLimit uint64         `json:"gas_limit"`
	Fees     sdktypes.Coins `json:"fees"`
	Memo     string         `json:"memo"`
}

// NewTransaction returns new Transaction object.
func NewTransaction(client *client.Client, chainID string, gasLimit uint64, fees sdktypes.Coins, memo string) *Transaction {
	return &Transaction{
		Client:   client,
		ChainID:  chainID,
		GasLimit: gasLimit,
		Fees:     fees,
		Memo:     memo,
	}
}

// Sign signs message(s) with the account's private key and braodacasts the message(s).
func (t *Transaction) Sign(ctx context.Context, accSeq uint64, accNum uint64, privKey *secp256k1.PrivKey, msgs ...sdktypes.Msg) ([]byte, error) {
	txBuilder := t.Client.CliCtx.TxConfig.NewTxBuilder()
	if err := txBuilder.SetMsgs(msgs...); err != nil {
		return nil, err
	}
	txBuilder.SetGasLimit(t.GasLimit)
	txBuilder.SetFeeAmount(t.Fees)
	txBuilder.SetMemo(t.Memo)

	signMode := t.Client.CliCtx.TxConfig.SignModeHandler().DefaultMode()

	sigV2 := signing.SignatureV2{
		PubKey: privKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  signMode,
			Signature: nil,
		},
		Sequence: accSeq,
	}

	err := txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, fmt.Errorf("failed to set signatures: %s", err)
	}

	signerData := authsigning.SignerData{
		ChainID:       t.ChainID,
		AccountNumber: accNum,
		Sequence:      accSeq,
	}

	sigV2, err = sdkclienttx.SignWithPrivKey(signMode, signerData, txBuilder, privKey, t.Client.CliCtx.TxConfig, accSeq)
	if err != nil {
		return nil, fmt.Errorf("failed to sign with private key: %s", err)
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, fmt.Errorf("failed to set signatures: %s", err)
	}

	txByte, err := t.Client.CliCtx.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, fmt.Errorf("failed to encode tx and get raw tx data: %s", err)
	}

	return txByte, nil
}

// CreateSwapBot creates a bot that makes multiple swaps which increases and decreases
func (t *Transaction) MsgsMarketOrder(sender_address string, marketId uint64, isbuy bool, quantity sdktypes.Dec, msgNum int) ([]sdktypes.Msg, error) {
	var msgs []sdktypes.Msg

	// randomize order price
	for i := 0; i < msgNum; i++ {
		msg := exchangetypes.MsgPlaceMarketOrder{
			Sender:   sender_address,
			MarketId: marketId,
			IsBuy:    isbuy,
			Quantity: quantity,
		}

		msgs = append(msgs, &msg)
	}

	return msgs, nil
}
