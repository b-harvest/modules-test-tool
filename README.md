<p align="center">
  <a href="https://github.com/b-harvest/modules-test-tool" target="_blank"><img width="140" src="https://avatars.githubusercontent.com/u/57690767?s=200&v=4" alt="B-Harvest"></a>
</p>

<h1 align="center">
    Cosmos Modules Testing Program 🔧
</h1>

## Overview

This program performs stress testing for the Cosmos module. Support: Liquidity , IBC transfer

**Note**: Requires [Go 1.17+](https://golang.org/dl/)
## Version

- [Liquidity Module v1.2.4](https://github.com/Gravity-Devs/liquidity/tree/v1.4.2) 
- [Cosmos SDK v0.44.5](https://github.com/cosmos/cosmos-sdk/tree/v0.44.5)
- [Tendermint v0.34.14](https://github.com/tendermint/tendermint/tree/v0.34.14)

## Usage

### Configuration

This stress testing program for the Cosmos module requires a configuration file, `config.toml` in current working directory. An example of configuration file is available in `example.toml` and the config source code can be found in [here](./config.config.go).
### Build

```bash
# Clone the project 
git clone https://github.com/b-harvest/modules-test-tool
cd modules-test-tool

# Build executable
make install
```

### Setup local testnet

Just by running simple command `make localnet`, it bootstraps a single local testnet in your local computer and it
automatically creates 4 genesis accounts with enough amounts of different types of coins. You can customize them in [this script](https://github.com/b-harvest/modules-test-tool/blob/main/scripts/localnet.sh#L9-L13) for your own usage.

```bash
# Run a single blockchain in your local computer 
make localnet
```

### CLI Commands

`$ tester -h`

```bash
comos module stress testing program

Usage:
  tester [command]

Available Commands:
  create-pools   create liquidity pools with the sample denom pairs.
  deposit        deposit coins to a liquidity pool in round times with a number of transaction messages
  help           Help about any command
  ibcbalances    
  ibctrace       
  muilt-transfer muilt Transfer a fungible token through IBC
  stress-test    run stress test
  swap           swap offer coin with demand coin.
  transfer       Transfer a fungible token through IBC
  withdraw       withdraw pool coin from the pool in round times with a number of transaction messages

Flags:
  -h, --help                help for tester
      --log-format string   logging format; must be either json or text; (default "text")
      --log-level string    logging level; (default "debug")
```

## Test

### localnet

```bash
# This command is useful for local testing.
tester ca

# tester deposit [pool-id] [deposit-coins] [round] [tx-num] [flags]
tester d 1 2000000uakt,2000000uatom 5 5

# tester withdraw [pool-id] [pool-coin] [round] [tx-num] [flags]
tester w 1 10pool94720F40B38D6DD93DCE184D264D4BE089EDF124A9C0658CDBED6CA18CF27752 5 5

# tester swap [pool-id] [offer-coin] [demand-coin-denom][round] [tx-num] [msg-num]
tester s 1 1000000uakt uatom 2 2 5

# tester transfer [src-port] [src-channel] [receiver] [amount] [round] [tx-num] [msg-num]
tester transfer transfer channel-0 cosmos18zh6zd2kwtekjeg0ns5xvn2x28hgj8n6gxhe8c 1stake 1 1 1

#tester muilt-transfer [src-chains] [dst-chains] [amount] [blocks] [tx-num] [msg-num]
tester muilt-transfer gaia,iris terra,osmo 10 1 1 1

tester ibcbalances
#persian-cat  |  5550ibc/265435C653FE85CD659E88CD51D4A735BDD4D3804871400378A488C71D68C72B,13566ibc/ED07A3391A112B175915CD8FAF43A2DA8E4790EDE12566649D0C2F97716B8518,1000000000000000ubnb,1000000000000000ubtc,999999899952109ucre,1000000000000000ueth,1000000000000000usol
#osmosis-testnet  |  31191ibc/1AA2D0DA14D24CEC9CCCE698F3B113B32F651365F6C91FFB5F301CFA33A175E1,999999899985768uosmo
#terra-testnet  |  16700ibc/7A0FAE01EB4FD6930A0111759B22BB631BB089C75F7186E4F9ACC0E139DE678C,1000ibc/A7304EE764FD4AAE4D81A75F0F396D3C2038F4BB8DA655ED2F8735F2F9F36295,999999899993400uluna,1000000000000000uusd

tester ibctrace
#osmosis-testnet
#{persian-cat:07-tendermint-0[connection-0(channel-0,)],},
#{persian-cat:07-tendermint-1[connection-1(channel-1,)],},

#terra-testnet
#{persian-cat:07-tendermint-0[connection-0(channel-0,)],},
#{persian-cat:07-tendermint-1[connection-1(channel-1,)],},
#{persian-cat:07-tendermint-2[connection-2(channel-2,)],},
```



