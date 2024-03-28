package cmd

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/evmos/ethermint/crypto/hd"
	etherminttypes "github.com/evmos/ethermint/types"
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

			ethClient, err := ethclient.Dial(cfg.ETHRPC.Address)
			if err != nil {
				return err
			}

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
			if maxAccountCount > len(accounts) {
				return fmt.Errorf("maxAccountCount is hgiher than accounts total count. \nCheckup your account json file: %w", err)
			}
			log.Debug().Msg("finished to parse arguments and accounts")
			// cut accounts to maxAccountCount
			accounts = accounts[:maxAccountCount]

			log.Debug().Msg("prepare private keys (concurrent)")
			ecdsaPrivateKeys := make([]*ecdsa.PrivateKey, len(accounts))
			wg := sync.WaitGroup{}
			for i, account := range accounts {
				wg.Add(1)
				go func(wg *sync.WaitGroup, mnemonic string, idx int) {
					defer wg.Done()
					bz, err := hd.EthSecp256k1.Derive()(mnemonic, keyring.DefaultBIP39Passphrase, etherminttypes.BIP44HDPath)
					if err != nil {
						panic(err)
					}
					privKey := hd.EthSecp256k1.Generate()(bz)
					ecdsaPrivKey, err := crypto.ToECDSA(privKey.Bytes())
					if err != nil {
						panic(err)
					}
					ecdsaPrivateKeys[idx] = ecdsaPrivKey
				}(&wg, account.Mnemonic, i)
			}
			wg.Wait()
			log.Debug().Msg("finished to prepare private keys")

			scenarios := []Scenario{
				{round, numTps},
			}

			accSeqs := make([]uint64, len(accounts))
			wg = sync.WaitGroup{}
			log.Debug().Msgf("getting account sequences (%d)", len(accounts))
			for i, account := range accounts {
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
			log.Debug().Msg("done getting account sequences, sleep 10 sec to make node stable")
			time.Sleep(10 * time.Second)

			evmParams, err := client.GRPC.GetEvmParams(context.Background())
			if err != nil {
				return err
			}
			evmDenom := evmParams.EvmDenom

			for no, scenario := range scenarios {
				log.Info().Msgf("starting simulation #%d, rounds = %d, tps = %d", no, scenario.Rounds, scenario.NumTps)

				var accPointer int
				gp := big.NewInt(cfg.Custom.GasPrice)

				invalidNonceCounter := make(map[string]int)
				var signedEthTxs []*gethtypes.Transaction
				for i := 0; i < scenario.Rounds; i++ {
					txs := make([][]byte, scenario.NumTps)
					log.Info().Msgf("round %d::signing loop (concurrent)", i)
					started := time.Now()
					wg := sync.WaitGroup{}
					var mu sync.Mutex
					accPointerMap := make(map[int]int)
					for j := 0; j < scenario.NumTps; j++ {
						// remember j's account pointer
						accPointerMap[j] = accPointer
						wg.Add(1)
						go func(w *sync.WaitGroup, accSeq uint64, ecdsaPk *ecdsa.PrivateKey, idx int) {
							defer w.Done()
							unsignedTx := gethtypes.NewTransaction(accSeq, contractAddr, amount, gasLimit, gp, calldata)
							signedTx, err := gethtypes.SignTx(unsignedTx, gethtypes.NewEIP155Signer(big.NewInt(cfg.Custom.ChainID)), ecdsaPk)
							if err != nil {
								log.Err(err).Msg("sign tx")
								return
							}

							mu.Lock()
							signedEthTxs = append(signedEthTxs, signedTx)
							mu.Unlock()
							ethTxBytes, err := rlp.EncodeToBytes(signedTx)
							if err != nil {
								log.Err(err).Msg("encode to bytes")
								return
							}

							msg := &evmtypes.MsgEthereumTx{}
							if err := msg.UnmarshalBinary(ethTxBytes); err != nil {
								log.Err(err).Msg("unmarshal binary")
								return
							}

							if err := msg.ValidateBasic(); err != nil {
								log.Err(err).Msg("validate basic")
								return
							}

							tx, err := msg.BuildTx(txConfig.NewTxBuilder(), evmDenom)
							if err != nil {
								log.Err(err).Msg("build tx")
								return
							}

							txBytes, err := txConfig.TxEncoder()(tx)
							if err != nil {
								log.Err(err).Msg("tx encoder")
								return
							}

							txs[idx] = txBytes
						}(&wg, accSeqs[accPointer], ecdsaPrivateKeys[accPointer], j)
						// increase pointer
						accPointer = (accPointer + 1) % maxAccountCount
					}
					wg.Wait()
					log.Debug().Msgf("finished took %s signing %d txs", time.Since(started), len(txs))

					log.Info().Msgf("round %d::sending loop (go-routines)", i)
					started = time.Now()
					wg = sync.WaitGroup{}
					for j, txByte := range txs {
						wg.Add(1)
						go func(w *sync.WaitGroup, accIdx int, tx []byte) {
							defer w.Done()
							resp, err := client.GRPC.BroadcastTx(ctx, tx)
							if err != nil {
								log.Err(err).Msg("broadcast tx")
							}

							if resp.TxResponse.Code != 0 {
								log.Warn().Msgf("tx failed, reason code: %d", resp.TxResponse.Code)
							}
							if resp.TxResponse.Code == 3 {
								// handle invalid nonce
								// query nonce
								idx := accPointerMap[accIdx]
								account := accounts[idx]
								acc, err := client.GRPC.GetBaseAccountInfo(ctx, account.Address)
								if err != nil {
									log.Err(err).Msg("get base account info")
									return
								}
								// update account sequence
								mu.Lock()
								accSeqs[idx] = acc.GetSequence()
								invalidNonceCounter[account.Address]++
								mu.Unlock()
							} else if resp.TxResponse.Code == 0 {
								mu.Lock() // increment account sequence when tx is successful
								accSeqs[accPointerMap[accIdx]]++
								mu.Unlock()
							}
						}(&wg, j, txByte)
					}
					wg.Wait()
					timeSpent := time.Since(started)
					log.Debug().Msgf("took %s broadcasting %d txs", timeSpent, len(txs))
					// sleep 1sec - timeSpent
					if timeSpent < time.Second {
						log.Debug().Msgf("sleeping for %s", time.Second-timeSpent)
						time.Sleep(time.Second - timeSpent)
					}
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

				// check tx is successful by querying receipt
				total := len(signedEthTxs)
				var succeeded, failed int

				// print invalid nonce counter
				for addr, count := range invalidNonceCounter {
					log.Warn().Msgf("invalid nonce count for %s: %d", addr, count)
				}

				log.Debug().Msg("checking tx receipts... (it takes time)")
				wg = sync.WaitGroup{}
				maxGoroutines := 100   // 동시에 실행할 최대 고루틴 수
				txsPerGoroutine := 100 // 각 고루틴이 처리할 트랜잭션 수
				txLen := len(signedEthTxs)
				goroutines := int(math.Ceil(float64(txLen) / float64(txsPerGoroutine)))

				semaphore := make(chan struct{}, maxGoroutines) // 고루틴 수를 제한하기 위한 세마포어
				results := make(chan bool, txLen)               // 결과를 저장할 채널

				for i := 0; i < goroutines; i++ {
					semaphore <- struct{}{} // 세마포어에 자리를 차지하며, 자리가 없으면 대기
					wg.Add(1)
					start := i * txsPerGoroutine
					end := start + txsPerGoroutine
					if end > txLen {
						end = txLen
					}
					go func(txs []*gethtypes.Transaction) {
						defer wg.Done()
						for _, tx := range txs {
							if tx == nil {
								results <- false
								continue
							}
							if _, err := ethClient.TransactionReceipt(ctx, tx.Hash()); err != nil {
								results <- false
								continue
							}
							results <- true
						}
						<-semaphore // 작업 완료 후 세마포어에서 자리를 반환
					}(signedEthTxs[start:end])
				}
				go func() {
					wg.Wait()
					close(results)
				}()

				for result := range results {
					if result {
						succeeded++
					} else {
						failed++
					}
				}

				log.Info().Msgf("total txs: %d, succeeded: %d, failed: %d", total, succeeded, failed)
				time.Sleep(5 * time.Second)
			}
			return nil
		},
	}
	return cmd
}
