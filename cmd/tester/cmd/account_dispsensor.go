package cmd

import (
	"context"
	"crypto/ecdsa"
	"fmt"

	"github.com/b-harvest/modules-test-tool/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/crypto/hd"
	etherminttypes "github.com/evmos/ethermint/types"
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
