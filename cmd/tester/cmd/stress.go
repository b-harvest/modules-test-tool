package cmd

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ghodss/yaml"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/evmos/ethermint/encoding"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	"github.com/b-harvest/modules-test-tool/client"
	"github.com/b-harvest/modules-test-tool/config"
)

type RawValidatorList []RawValidator

var (
	accountFilePath = "account.yaml"
)

func StressTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stress-test [calldata] [contract-address] [amount] [round] [txs-per-round] [raw-max-account]",
		Short: "run stress test",
		Args:  cobra.ExactArgs(6),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			ctx := context.Background()

			encodingConfig := encoding.MakeTestEncodingConfig()
			txConfig := encodingConfig.TxConfig

			err := SetLogger(logLevel)
			if err != nil {
				return fmt.Errorf("set logger: %w", err)
			}

			cfg, err := config.Read(config.DefaultConfigPath)
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}

			client, err := client.NewClient(cfg.RPC.Address, cfg.GRPC.Address)
			if err != nil {
				return fmt.Errorf("new client: %w", err)
			}
			defer client.Stop() // nolint: errcheck

			calldata, err := hexutil.Decode(args[0])
			if err != nil {
				return fmt.Errorf("failed to decode ethereum tx hex bytes: %w", err)
			}

			contractAddr := common.HexToAddress(args[1])

			amount, ok := new(big.Int).SetString(args[2], 10)
			if !ok {
				return fmt.Errorf("failed to conver %s to big.Int", args[2])
			}

			gasLimit := uint64(cfg.Custom.GasLimit)

			rawRound := args[3]
			round, err := strconv.Atoi(rawRound)

			rawTps := args[4]
			numTps, err := strconv.Atoi(rawTps)
			if err != nil {
				return fmt.Errorf("Cannot parse round, numTps\n%s", err)
			}

			rawMaxAccountCount := args[5]
			maxAccountCount, err := strconv.Atoi(rawMaxAccountCount)
			if err != nil {
				return fmt.Errorf("Cannot parse maxAccountCount\n%s", err)
			}

			var accounts RawValidatorList
			bytes, err := os.ReadFile(accountFilePath)
			if err != nil {
				return fmt.Errorf("failed to read account file: %w", err)
			}
			err = yaml.Unmarshal(bytes, &accounts)
			if err != nil {
				return fmt.Errorf("failed to unmarshal accounts: %w", err)
			}
			log.Debug().Msg("finished to parse arguments and accounts")

			var (
				mnemonics []string
				addresses []string
			)
			for _, account := range accounts {
				mnemonics = append(mnemonics, account.Mnemonic)
				addresses = append(addresses, account.Address)
			}

			if maxAccountCount > len(accounts) {
				return fmt.Errorf("maxAccountCount is hgiher than accounts total count. \nCheckup your account json file: %w", err)
			}

			scenarios := []Scenario{
				{round, numTps},
			}

			accSeqs := make([]uint64, maxAccountCount)
			var wg sync.WaitGroup
			log.Debug().Msgf("getting account sequences (%d)", len(accounts))
			for i, account := range accounts[:maxAccountCount] {
				wg.Add(1)
				go func(i int, account RawValidator) {
					defer wg.Done()
					acc, err := client.GRPC.GetBaseAccountInfo(context.Background(), account.Address)
					if err != nil {
						log.Error().Err(err).Msg("get base account info")
						return
					}
					accSeq := acc.GetSequence()
					accSeqs[i] = accSeq
				}(i, account)
			}
			wg.Wait()
			log.Debug().Msg("done getting account sequences")

			for no, scenario := range scenarios {
				log.Info().Msgf("starting simulation #%d, rounds = %d, tps = %d", no, scenario.Rounds, scenario.NumTps)

				d := NewAccountDispenser(client, mnemonics, addresses)
				var accountSec int
				if err = d.Next(); err != nil {
					return fmt.Errorf("get next account: %w", err)
				}
				for i := 0; i < scenario.Rounds; i++ {

					var txs [][]byte
					log.Info().Msgf("round %d::signing loop", i)
					for j := 0; j < scenario.NumTps; j++ {
						//accSeq := d.IncAccSeq()
						nowGas := big.NewInt(cfg.Custom.GasPrice)
						unsignedTx := gethtypes.NewTransaction(accSeqs[accountSec], contractAddr, amount, gasLimit, nowGas, calldata)
						signedTx, err := gethtypes.SignTx(unsignedTx, gethtypes.NewEIP155Signer(big.NewInt(cfg.Custom.ChainID)), d.ecdsaPrivKey)
						if err != nil {
							return err
						}
						accSeqs[accountSec]++
						accountSec++
						accountSec %= maxAccountCount

						ethTxBytes, err := rlp.EncodeToBytes(signedTx)
						if err != nil {
							return err
						}

						msg := &evmtypes.MsgEthereumTx{}
						if err := msg.UnmarshalBinary(ethTxBytes); err != nil {
							return err
						}

						if err := msg.ValidateBasic(); err != nil {
							return err
						}

						tx, err := msg.BuildTx(txConfig.NewTxBuilder(), d.evmDenom)
						if err != nil {
							return err
						}

						txBytes, err := txConfig.TxEncoder()(tx)
						if err != nil {
							return fmt.Errorf("sign tx: %w", err)
						}

						if err := d.Next(); err != nil {
							return fmt.Errorf("get next account: %w", err)
						}

						txs = append(txs, txBytes)
					}

					started := time.Now()
					log.Info().Msgf("round %d::sending loop (go-routines)", i)
					wg := sync.WaitGroup{}
					for _, txByte := range txs {
						wg.Add(1)
						go func(w *sync.WaitGroup, tx []byte) {
							defer w.Done()
							resp, err := client.GRPC.BroadcastTx(ctx, tx)
							if err != nil {
								log.Err(err).Msg("broadcast tx")
							}

							if resp.TxResponse.Code != 0 {
								if resp.TxResponse.Code == 0x14 {
									log.Warn().Msg("mempool is full, stopping")
									return
								}
								if resp.TxResponse.Code == 0x5 {
									log.Printf("Insufficient funds!!! %s", d.addr)
									return
								}
								if resp.TxResponse.Code == 0x13 || resp.TxResponse.Code == 0x20 {
									log.Warn().Str("addr", d.Addr()).Uint64("seq", d.AccSeq()).Msgf("received %#v, using next account", resp.TxResponse)
								} else {
									log.Warn().Msg("panic")
									panic(fmt.Sprintf("%#v, %s\n", resp.TxResponse, d.addr[d.i]))
								}
							}
						}(&wg, txByte)
					}
					wg.Wait()
					timeSpent := time.Since(started)
					log.Debug().Msgf("took %s broadcasting %d txs", timeSpent, len(txs))
					// sleep 1sec - timeSpent
					log.Debug().Msgf("sleeping for %s", time.Second-timeSpent)
					time.Sleep(time.Second - timeSpent)
				}

				started := time.Now()
				log.Debug().Msg("cooling down")
				for {
					st, err := client.RPC.NumUnconfirmedTxs(ctx)
					if err != nil {
						return fmt.Errorf("get status: %w", err)
					}
					if st.Total == 0 {
						break
					}
					time.Sleep(5 * time.Second)
				}
				log.Debug().Str("elapsed", time.Since(started).String()).Msg("done cooling down")
				time.Sleep(time.Minute)
			}
			return nil
		},
	}
	return cmd
}
