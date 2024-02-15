package cmd

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	txtype "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/crypto/hd"
	"github.com/evmos/ethermint/encoding"
	etherminttypes "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	"github.com/b-harvest/modules-test-tool/config"
)

func NewEvmTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evmtx [calldata] [contract-address] [round] [tx-num]",
		Short: "Broadcast evm tx",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			encodingConfig := encoding.MakeConfig(app.ModuleBasics)
			txConfig := encodingConfig.TxConfig

			err := SetLogger(logLevel)
			if err != nil {
				return err
			}

			cfg, err := config.Read(config.DefaultConfigPath)
			if err != nil {
				return err
			}

			// make calldata
			//
			// var NativeMetaData = &bind.MetaData{
			// 	 ABI: "[{\"inputs\":[],\"name\":\"add\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"subtract\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getCounter\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
			// }
			//
			// func main() {
			// 	 abi, err := NativeMetaData.GetAbi()
			// 	 if err != nil {
			// 	 	panic(err)
			// 	 }
			// 	 payload, err := abi.Pack("add")
			// 	 if err != nil {
			// 	 	panic(err)
			// 	 }
			// 	 fmt.Println("Calldata in hex format:", hex.EncodeToString(payload))
			// }
			calldata, err := hexutil.Decode(args[0])
			if err != nil {
				return fmt.Errorf("failed to decode ethereum tx hex bytes: %w", err)
			}

			contractAddr := common.HexToAddress(args[1])

			round, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("round must be integer: %s", args[2])
			}

			txNum, err := strconv.Atoi(args[3])
			if err != nil {
				return fmt.Errorf("tx-num must be integer: %s", args[3])
			}

			// var addr string = "canto1xtpwsznx7sp9jucefmxdy37yvexztu04t3nskj"
			var addr string = cfg.Custom.CantoAddress
			mnemonic := cfg.Custom.Mnemonics[0]
			bz, err := hd.EthSecp256k1.Derive()(mnemonic, keyring.DefaultBIP39Passphrase, etherminttypes.BIP44HDPath)
			if err != nil {
				return err
			}
			privKey := hd.EthSecp256k1.Generate()(bz)
			ecdsaPrivKey, err := crypto.ToECDSA(privKey.Bytes())
			if err != nil {
				return err
			}

			// Create a connection to the gRPC server.
			grpcConn, _ := grpc.Dial(
				cfg.GRPC.Address,    // Or your gRPC server address.
				grpc.WithInsecure(), // The Cosmos SDK doesn't support any transport security mechanism.
			)
			defer grpcConn.Close()

			// Broadcast the tx via gRPC. We create a new client for the Protobuf Tx service.
			txClient := txtype.NewServiceClient(grpcConn)

			// To find accounts' number & seq, Make authQuery connection
			authClient := authtypes.NewQueryClient(grpcConn)
			queryAccountReq := authtypes.QueryAccountRequest{
				Address: addr,
			}

			evmClient := evmtypes.NewQueryClient(grpcConn)
			queryParamsReq := evmtypes.QueryParamsRequest{}
			queryParamsResp, err := evmClient.Params(
				context.Background(),
				&queryParamsReq,
			)
			if err != nil {
				return err
			}
			evmDenom := queryParamsResp.Params.EvmDenom

			gasLimit := uint64(cfg.Custom.GasLimit)
			gasPrice := big.NewInt(cfg.Custom.GasPrice)

			for i := 0; i < round; i += 1 {
				// Check accNum, accSeq
				queryAccountResp, err := authClient.Account(
					context.Background(),
					&queryAccountReq,
				)
				if err != nil {
					return err
				}
				var ethAccount etherminttypes.EthAccount
				err = ethAccount.Unmarshal(queryAccountResp.GetAccount().Value)
				if err != nil {
					return err
				}
				accSeq := ethAccount.GetSequence()

				var txBytesArray [][]byte
				for j := 0; j < txNum; j++ {
					unsignedTx := gethtypes.NewTransaction(accSeq, contractAddr, nil, gasLimit, gasPrice, calldata)
					signedTx, err := gethtypes.SignTx(unsignedTx, gethtypes.NewEIP155Signer(big.NewInt(cfg.Custom.ChainID)), ecdsaPrivKey)
					if err != nil {
						return err
					}

					txBytes, err := rlp.EncodeToBytes(signedTx)
					if err != nil {
						return err
					}
					txBytesArray = append(txBytesArray, txBytes)
					accSeq = accSeq + 1
				}

				log.Info().Msgf("round:%d; txNum:%d; accAddr:%s", i+1, txNum, addr)

				for _, txBytesItem := range txBytesArray {
					msg := &evmtypes.MsgEthereumTx{}
					if err := msg.UnmarshalBinary(txBytesItem); err != nil {
						return err
					}

					if err := msg.ValidateBasic(); err != nil {
						return err
					}

					tx, err := msg.BuildTx(txConfig.NewTxBuilder(), evmDenom)
					if err != nil {
						return err
					}

					txBytes, err := txConfig.TxEncoder()(tx)
					if err != nil {
						return err
					}

					// We then call the BroadcastTx method on this client.
					grpcRes, err := txClient.BroadcastTx(
						context.Background(),
						&txtype.BroadcastTxRequest{
							Mode:    txtype.BroadcastMode_BROADCAST_MODE_SYNC,
							TxBytes: txBytes, // Proto-binary of the signed transaction, see previous step.
						},
					)
					if err != nil {
						return err
					}

					log.Info().Msgf("%s/cosmos/tx/v1beta1/txs/%s", cfg.LCD.Address, grpcRes.TxResponse.TxHash)
				}
				time.Sleep(3 * time.Second)
			}

			return nil
		},
	}
	return cmd
}
