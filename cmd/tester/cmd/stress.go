package cmd

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/ghodss/yaml"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/evmos/ethermint/crypto/hd"
	"github.com/evmos/ethermint/encoding"
	etherminttypes "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	"github.com/b-harvest/modules-test-tool/client"
	"github.com/b-harvest/modules-test-tool/config"
)

type AccountDispenser struct {
	c            *client.Client
	mnemonics    []string
	i            int
	addr         []string
	privKey      cryptotypes.PrivKey
	ecdsaPrivKey *ecdsa.PrivateKey
	accSeq       uint64
	accNum       uint64
	evmDenom     string
}

func NewAccountDispenser(c *client.Client, mnemonics []string, canto_addrs []string) *AccountDispenser {
	return &AccountDispenser{
		c:         c,
		mnemonics: mnemonics,
		addr:      canto_addrs,
	}
}

func (d *AccountDispenser) Next() error {
	mnemonic := d.mnemonics[d.i]
	bz, err := hd.EthSecp256k1.Derive()(mnemonic, keyring.DefaultBIP39Passphrase, etherminttypes.BIP44HDPath)
	if err != nil {
		return err
	}
	privKey := hd.EthSecp256k1.Generate()(bz)
	ecdsaPrivKey, err := crypto.ToECDSA(privKey.Bytes())
	if err != nil {
		return err
	}

	d.privKey = privKey
	d.ecdsaPrivKey = ecdsaPrivKey
	acc, err := d.c.GRPC.GetBaseAccountInfo(context.Background(), d.addr[d.i])
	if err != nil {
		return fmt.Errorf("get base account info: %w", err)
	}
	d.accNum = acc.GetAccountNumber()
	d.i++
	if d.i >= len(d.mnemonics) {
		d.i = 0
	}

	evmParams, err := d.c.GRPC.GetEvmParams(context.Background())
	if err != nil {
		return err
	}
	d.evmDenom = evmParams.EvmDenom

	return nil
}

func (d *AccountDispenser) Addr() string {
	return d.addr[d.i]
}

func (d *AccountDispenser) PrivKey() cryptotypes.PrivKey {
	return d.privKey
}

func (d *AccountDispenser) AccSeq() uint64 {
	return d.accSeq
}

func (d *AccountDispenser) AccNum() uint64 {
	return d.accNum
}

func (d *AccountDispenser) IncAccSeq() uint64 {
	r := d.accSeq
	d.accSeq++
	return r
}

func (d *AccountDispenser) DecAccSeq() {
	d.accSeq--
}

type Scenario struct {
	Rounds int
	Tps    int
}

type RawValidator struct {
	Moniker string `yaml:"Moniker"`
	Address string `yaml:"Address"`
	//BalAmount    string `yaml:"BalAmount"`
	//StakeAmount  string `yaml:"StakeAmount"`
	ValidatorKey string `yaml:"ValidatorKey"`
	Mnemonic     string `yaml:"Mnemonic"`
	AccSequence  uint64 `yaml:"accSequence"`
}

type RawValidatorList []RawValidator

var (
	accountFilePath = "account.yaml"
	//scenarios = []Scenario{
	//      {2000, 20},
	//      {2000, 50},
	//      {2000, 200},
	//      {2000, 500},
	//}
	//scenarios = []Scenario{
	//      {5, 10},
	//      {5, 50},
	//      {5, 100},
	//      {5, 200},
	//      {5, 300},
	//      {5, 400},
	//      {5, 500},
	//}
)

func StressTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stress-test [calldata] [contract-address] [amount] [round] [tps] [max-account-count]",
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

			var (
				mnemonics []string
				addresses []string
			)
			for _, account := range accounts {
				mnemonics = append(mnemonics, account.Mnemonic)
				addresses = append(addresses, account.Address)
			}

			//rawAddGasAmount := args[6]
			//addGasAmount, err := strconv.Atoi(rawAddGasAmount)
			if err != nil {
				return fmt.Errorf("Cannot parse addGasAmount: %s\n", err)
			}

			if maxAccountCount > len(accounts) {
				return fmt.Errorf("maxAccountCount is hgiher than accounts total count. \nCheckup your account json file: %w", err)
			}

			//var (
			//      mnemonics []string
			//      addresses []string
			//)
			//
			//for _, account := range accounts {
			//      mnemonics = append(mnemonics, account.Mnemonic)
			//      addresses = append(addresses, account.Address)
			//}

			scenarios := []Scenario{
				{round, numTps},
			}

			var (
				senderFlag int
			)
			for idx, account := range accounts {
				acc, err := client.GRPC.GetBaseAccountInfo(context.Background(), account.Address)
				if err != nil {
					return fmt.Errorf("get base account info: %w", err)
				}
				accSeq := acc.GetSequence()
				accounts[idx].AccSequence = accSeq
				println(accounts[idx].AccSequence)
			}

			f, err := os.ReadFile("sender.txt")
			if err != nil {
				senderFile, _ := os.Create("sender.txt")
				senderFile.Write([]byte("0"))
				senderFlag = 0
			} else {
				senderFlag, err = strconv.Atoi(string(f))
				if err != nil {
					return fmt.Errorf("Cannot get sender flag!!!!!!\n")
				}
			}

			for _, scenario := range scenarios {
				d := NewAccountDispenser(client, mnemonics, addresses)
				if err = d.Next(); err != nil {
					return fmt.Errorf("get next account: %w", err)
				}

				nowGas := big.NewInt(cfg.Custom.GasPrice * int64(2))
				for i := 0; i < scenario.Rounds; i++ {
					started := time.Now()
					sent := 0

					log.Info().Msgf("start round %d", i)
					var txsBytes [][]byte

					for j := 0; j < scenario.Tps; j++ {
						unsignedTx := gethtypes.NewTransaction(accounts[senderFlag].AccSequence, contractAddr, amount, gasLimit, nowGas, calldata)
						signedTx, err := gethtypes.SignTx(unsignedTx, gethtypes.NewEIP155Signer(big.NewInt(cfg.Custom.ChainID)), d.ecdsaPrivKey)
						accounts[senderFlag].AccSequence++

						senderFlag++
						senderFlag %= maxAccountCount
						//fmt.Println(cfg.Custom.ChainID)
						if err != nil {
							return err
						}

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

						txsBytes = append(txsBytes, txBytes)
					}

					var (
						wg sync.WaitGroup
						mu = &sync.Mutex{}
					)
					errCh := make(chan error, scenario.Tps) // Error channel with a buffer size of Tps

					for j := 0; j < len(txsBytes); j++ {
						wg.Add(1)
						go func(k int) { // Pass the loop variable as a parameter to avoid capturing it
							defer wg.Done()

							resp, err := client.GRPC.BroadcastTx(ctx, txsBytes[k])
							if err != nil {
								errCh <- fmt.Errorf("broadcast tx: %w", err)
								return
							}

							if resp.TxResponse.Code != 0 {
								switch resp.TxResponse.Code {
								case 0x14:
									log.Warn().Msg("mempool is full, stopping")
									d.DecAccSeq()
									errCh <- fmt.Errorf("mempool is full")
									return
								case 0x5:
									errCh <- fmt.Errorf("insufficient funds!!! %s", d.addr)
									return // Using return instead of break since we're in a goroutine
								case 0x13, 0x20:
									if err := d.Next(); err != nil {
										errCh <- fmt.Errorf("get next account: %w", err)
										return
									}
									log.Warn().Str("addr", d.Addr()).Uint64("seq", d.AccSeq()).Msgf("received %#v, using next account", resp.TxResponse)
									time.Sleep(500 * time.Millisecond)
								default:
									errCh <- fmt.Errorf("unexpected error code: %#v, %s", resp.TxResponse, d.addr[d.i])
									return
								}
							}

							if err := d.Next(); err != nil {
								errCh <- fmt.Errorf("get next account: %w", err)
								return
							}

							mu.Lock()
							sent++
							mu.Unlock()
						}(j)
					}

					// Start a goroutine to listen on the errCh channel and print errors
					go func() {
						for err := range errCh {
							log.Info().Msgf("Error: %s", err)
						}
					}()

					wg.Wait() // Wait for all goroutines to finish
					close(errCh)

					elapsed := time.Since(started)

					log.Debug().Msgf("took %s broadcasting txs", elapsed)
					if elapsed < time.Second {
						time.Sleep(time.Second - elapsed)
					}

					log.Printf("taks %d completed, took %s", i, time.Since(started))
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

				senderFile, _ := os.Create("sender.txt")
				senderFlagStr := strconv.Itoa(senderFlag)
				senderFile.Write([]byte(senderFlagStr))

				time.Sleep(time.Minute)
			}

			return nil
		},
	}
	return cmd
}
