package cmd

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/b-harvest/modules-test-tool/config"
	"github.com/b-harvest/modules-test-tool/wallet"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	txtype "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	liquiditytypes "github.com/crescent-network/crescent/v3/x/liquidity/types"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/simapp"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"google.golang.org/grpc"
)

func MMOrderCmd() *cobra.Command {
	cmd := &cobra.Command{
		//Use:     "swap [pool-id] [offer-coin] [demand-coin-denom] [round] [tx-num] [msg-num]",
		//mm-order [pair-id] [max-sell-price] [min-sell-price] [sell-amount] [max-buy-price] [min-buy-price] [buy-amount]

		Use:     "mm-order [pair-id] [max-sell-price] [min-sell-price] [sell-amount] [max-buy-price] [min-buy-price] [buy-amount] [round] [tx-num]",
		Short:   "mm [pair-id] [market-price] [amount] [round] [tx-num]",
		Aliases: []string{"mm"},
		Args:    cobra.ExactArgs(9),
		Long: `Make an MM(market making) order. An MM order is a group of multiple buy/sell limit orders which are distributed evenly based on its parameters.
Example: $ tester mm-order 1 1.18 5 5 2

[pair-id]: pair id to make order
[max-sell-price]: maximum price of sell orders
[min-sell-price]]: minimum price of sell orders
[sell-amount]: total amount of sell orders
[max-buy-price]: maximum price of buy orders
[min-buy-price]: minimum price of buy orders
[buy-amount]: the total amount of buy orders
[round]: how many rounds to run
[tx-num]: how many transactions to be included in a block
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := SetLogger(logLevel)
			if err != nil {
				return err
			}

			cfg, err := config.Read(config.DefaultConfigPath)
			if err != nil {
				return err
			}

			pairId, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("parse pair id: %w", err)
			}

			maxSellPrice, err := types.NewDecFromStr(args[1])
			if err != nil {
				return fmt.Errorf("invalid max sell price: %w", err)
			}

			minSellPrice, err := types.NewDecFromStr(args[2])
			if err != nil {
				return fmt.Errorf("invalid min sell price: %w", err)
			}

			sellAmt, ok := types.NewIntFromString(args[3])
			if !ok {
				return fmt.Errorf("invalid sell amount: %s", args[3])
			}

			maxBuyPrice, err := types.NewDecFromStr(args[4])
			if err != nil {
				return fmt.Errorf("invalid max buy price: %w", err)
			}

			minBuyPrice, err := types.NewDecFromStr(args[5])
			if err != nil {
				return fmt.Errorf("invalid min buy price: %w", err)
			}

			buyAmt, ok := types.NewIntFromString(args[6])
			if !ok {
				return fmt.Errorf("invalid buy amount: %s", args[3])
			}

			round, err := strconv.Atoi(args[7])
			if err != nil {
				return fmt.Errorf("round must be integer: %s", args[3])
			}

			txNum, err := strconv.Atoi(args[8])
			if err != nil {
				return fmt.Errorf("tx-num must be integer: %s", args[4])
			}

			//var addr string = "cre1zgwx3cwyyx8np35hlzngmkfdalnrjxj23uu4fj"
			var addr string = cfg.Custom.CrescentAddress
			myMne := cfg.Custom.Mnemonics[0]
			_, privKey, err := wallet.RecoverAccountFromMnemonic(myMne, "")
			if err != nil {
				return err
			}
			priv := cryptotypes.PrivKey(privKey)

			// Create msg for MMOrder
			msg := liquiditytypes.MsgMMOrder{
				Orderer:       addr,
				PairId:        pairId,
				MaxSellPrice:  maxSellPrice,
				MinSellPrice:  minSellPrice,
				SellAmount:    sellAmt,
				MaxBuyPrice:   maxBuyPrice,
				MinBuyPrice:   minBuyPrice,
				BuyAmount:     buyAmt,
				OrderLifespan: time.Hour, // 1시간
			}

			// Create a connection to the gRPC server.
			grpcConn, err := grpc.Dial(
				cfg.GRPC.Address,    // Or your gRPC server address.
				grpc.WithInsecure(), // The Cosmos SDK doesn't support any transport security mechanism.
			)
			defer grpcConn.Close()

			// we use Protobuf, given by the following function.
			encCfg := simapp.MakeTestEncodingConfig()
			// Create a new TxBuilder.
			txBuilder := encCfg.TxConfig.NewTxBuilder()
			if err := txBuilder.SetMsgs(&msg); err != nil {
				return err
			}
			txBuilder.SetGasLimit(uint64(cfg.Custom.GasLimit))

			// To find accounts' number & seq, Make authQuery connection
			authClient := authtypes.NewQueryClient(grpcConn)
			queryAccountReq := authtypes.QueryAccountRequest{
				Address: addr,
			}

			for i := 0; i < round; i++ {
				// Check accNum, accSeq
				queryAccountResp, err := authClient.Account(
					context.Background(),
					&queryAccountReq,
				)
				if err != nil {
					return err
				}
				var baseAccount authtypes.BaseAccount
				err = baseAccount.Unmarshal(queryAccountResp.GetAccount().Value)
				if err != nil {
					return err
				}
				accNum := baseAccount.GetAccountNumber()
				accSeq := baseAccount.GetSequence()

				// First round: we gather all the signer infos. We use the "set empty
				// signature" hack to do that.
				sigV2 := signing.SignatureV2{
					PubKey: priv.PubKey(),
					Data: &signing.SingleSignatureData{
						SignMode:  encCfg.TxConfig.SignModeHandler().DefaultMode(),
						Signature: nil,
					},
					Sequence: accSeq,
				}
				err = txBuilder.SetSignatures(sigV2)
				if err != nil {
					return err
				}

				// Second round: all signer infos are set, so each signer can sign.
				sigV2 = signing.SignatureV2{}
				signerData := xauthsigning.SignerData{
					ChainID:       "mooncat-2-external",
					AccountNumber: accNum,
					Sequence:      accSeq,
				}
				sigV2, err = tx.SignWithPrivKey(
					encCfg.TxConfig.SignModeHandler().DefaultMode(), signerData,
					txBuilder, priv, encCfg.TxConfig, accSeq)
				if err != nil {
					return err
				}
				err = txBuilder.SetSignatures(sigV2)
				if err != nil {
					return err
				}

				var txBytesArray [][]byte

				for i := 0; i < txNum; i++ {
					// Generated Protobuf-encoded bytes.
					txBytes, err := encCfg.TxConfig.TxEncoder()(txBuilder.GetTx())
					if err != nil {
						return err
					}

					accSeq = accSeq + 1

					txBytesArray = append(txBytesArray, txBytes)
				}

				log.Info().Msgf("round:%d; txNum:%d; accAddr:%s", i+1, txNum, addr)

				for _, txBytesItem := range txBytesArray {
					// Broadcast the tx via gRPC. We create a new client for the Protobuf Tx service.
					txClient := txtype.NewServiceClient(grpcConn)
					// We then call the BroadcastTx method on this client.
					grpcRes, err := txClient.BroadcastTx(
						context.Background(),
						&txtype.BroadcastTxRequest{
							Mode:    txtype.BroadcastMode_BROADCAST_MODE_SYNC,
							TxBytes: txBytesItem, // Proto-binary of the signed transaction, see previous step.
						},
					)
					if err != nil {
						return err
					}

					log.Info().Msgf("%s/cosmos/tx/v1beta1/txs/%s", cfg.LCD.Address, grpcRes.TxResponse.TxHash)
				}
			}

			return nil
		},
	}
	return cmd
}