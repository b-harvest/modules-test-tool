package cmd

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/b-harvest/modules-test-tool/client"
	"github.com/b-harvest/modules-test-tool/config"
	"github.com/b-harvest/modules-test-tool/tx"
	"github.com/b-harvest/modules-test-tool/wallet"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
)

type AccountDispenser struct {
	c         *client.Client
	mnemonics []string
	i         int
	addr      string
	privKey   *secp256k1.PrivKey
	accSeq    uint64
	accNum    uint64
}

func NewAccountDispenser(c *client.Client, mnemonics []string) *AccountDispenser {
	return &AccountDispenser{
		c:         c,
		mnemonics: mnemonics,
	}
}

func (d *AccountDispenser) Next() error {
	mnemonic := d.mnemonics[d.i]
	addr, privKey, err := wallet.RecoverAccountFromMnemonic(mnemonic, "")
	if err != nil {
		return err
	}
	d.addr = addr
	d.privKey = privKey
	acc, err := d.c.GRPC.GetBaseAccountInfo(context.Background(), addr)
	if err != nil {
		return fmt.Errorf("get base account info: %w", err)
	}
	d.accSeq = acc.GetSequence()
	d.accNum = acc.GetAccountNumber()
	d.i++
	if d.i >= len(d.mnemonics) {
		d.i = 0
	}
	return nil
}

func (d *AccountDispenser) Addr() string {
	return d.addr
}

func (d *AccountDispenser) PrivKey() *secp256k1.PrivKey {
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
	Rounds         int
	NumTxsPerBlock int
	NumMsgsPerTx   int
}

var (
	scenarios = []Scenario{
		{2000, 20, 1},
		{2000, 50, 1},
		{2000, 200, 1},
		{2000, 500, 1},
	}
	//scenarios = []Scenario{
	//	{5, 10},
	//	{5, 50},
	//	{5, 100},
	//	{5, 200},
	//	{5, 300},
	//	{5, 400},
	//	{5, 500},
	//}
)

func StressTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stress-test [market-id] [is-buy] [quantity]",
		Short: "run stress test",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			ctx := context.Background()

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

			marketId, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("parse market id: %w", err)
			}

			isbuy, err := strconv.ParseBool(args[1])
			if err != nil {
				return fmt.Errorf("invalid isbuy: %w", err)
			}

			quantity, ok := sdk.NewIntFromString(args[2])
			if !ok {
				return fmt.Errorf("invalid buy amount: %s", args[2])
			}

			chainID, err := client.RPC.GetNetworkChainID(ctx)
			if err != nil {
				return err
			}

			gasLimit := uint64(cfg.Custom.GasLimit)
			fees := sdk.NewCoins(sdk.NewCoin(cfg.Custom.FeeDenom, sdk.NewInt(cfg.Custom.FeeAmount)))
			memo := cfg.Custom.Memo
			tx := tx.NewTransaction(client, chainID, gasLimit, fees, memo)

			d := NewAccountDispenser(client, cfg.Custom.Mnemonics)
			if err := d.Next(); err != nil {
				return fmt.Errorf("get next account: %w", err)
			}

			blockTimes := make(map[int64]time.Time)

			for no, scenario := range scenarios {
				st, err := client.RPC.Status(ctx)
				if err != nil {
					return fmt.Errorf("get status: %w", err)
				}
				startingHeight := st.SyncInfo.LatestBlockHeight + 2
				log.Info().Msgf("current block height is %d, waiting for the next block to be committed", st.SyncInfo.LatestBlockHeight)

				if err := rpcclient.WaitForHeight(client.RPC, startingHeight-1, nil); err != nil {
					return fmt.Errorf("wait for height: %w", err)
				}
				log.Info().Msgf("starting simulation #%d, rounds = %d, num txs per block = %d", no+1, scenario.Rounds, scenario.NumTxsPerBlock)

				targetHeight := startingHeight

				for i := 0; i < scenario.Rounds; i++ {
					st, err := client.RPC.Status(ctx)
					if err != nil {
						return fmt.Errorf("get status: %w", err)
					}
					if st.SyncInfo.LatestBlockHeight != targetHeight-1 {
						log.Warn().Int64("expected", targetHeight-1).Int64("got", st.SyncInfo.LatestBlockHeight).Msg("mismatching block height")
						targetHeight = st.SyncInfo.LatestBlockHeight + 1
					}

					started := time.Now()
					sent := 0
				loop:
					for sent < scenario.NumTxsPerBlock {
						msgs, err := tx.MsgsMarketOrder(d.Addr(), marketId, isbuy, quantity, scenario.NumMsgsPerTx)
						if err != nil {
							return fmt.Errorf("create msgs: %w", err)
						}

						for sent < scenario.NumTxsPerBlock {
							accSeq := d.IncAccSeq()
							txByte, err := tx.Sign(ctx, accSeq, d.AccNum(), d.PrivKey(), msgs...)
							if err != nil {
								return fmt.Errorf("sign tx: %w", err)
							}
							resp, err := client.GRPC.BroadcastTx(ctx, txByte)
							if err != nil {
								return fmt.Errorf("broadcast tx: %w", err)
							}
							if resp.TxResponse.Code != 0 {
								if resp.TxResponse.Code == 0x14 {
									log.Warn().Msg("mempool is full, stopping")
									d.DecAccSeq()
									break loop
								}
								if resp.TxResponse.Code == 0x13 || resp.TxResponse.Code == 0x20 {
									if err := d.Next(); err != nil {
										return fmt.Errorf("get next account: %w", err)
									}
									log.Warn().Str("addr", d.Addr()).Uint64("seq", d.AccSeq()).Msgf("received %#v, using next account", resp.TxResponse)
									time.Sleep(500 * time.Millisecond)
									break
								} else {
									panic(fmt.Sprintf("%#v\n", resp.TxResponse))
								}
							}
							sent++
						}
					}
					log.Debug().Msgf("took %s broadcasting txs", time.Since(started))

					if err := rpcclient.WaitForHeight(client.RPC, targetHeight, nil); err != nil {
						return fmt.Errorf("wait for height: %w", err)
					}

					r, err := client.RPC.Block(ctx, &targetHeight)
					if err != nil {
						return err
					}
					var blockDuration time.Duration
					bt, ok := blockTimes[targetHeight-1]
					if !ok {
						log.Warn().Msg("past block time not found")
					} else {
						blockDuration = r.Block.Time.Sub(bt)
						delete(blockTimes, targetHeight-1)
					}
					blockTimes[targetHeight] = r.Block.Time
					log.Info().
						Int64("height", targetHeight).
						Str("block-time", r.Block.Time.Format(time.RFC3339Nano)).
						Str("block-duration", blockDuration.String()).
						Int("broadcast-txs", sent).
						Int("committed-txs", len(r.Block.Txs)).
						Msg("block committed")

					targetHeight++
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
