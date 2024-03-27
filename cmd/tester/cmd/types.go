package cmd

type Scenario struct {
	Rounds int
	NumTps int
}

type AccountConfig struct {
	AccountName string `json:"accountName"`
	Address     string `json:"address"`
	Mnemonic    string `json:"mnemonic"`
}

type AccountsConfig struct {
	Accounts []AccountConfig `json:"accounts"`
}
type RawValidator struct {
	Moniker string `yaml:"Moniker"`
	Address string `yaml:"Address"`
	//BalAmount    string `yaml:"BalAmount"`
	//StakeAmount  string `yaml:"StakeAmount"`
	ValidatorKey string `yaml:"ValidatorKey"`
	Mnemonic     string `yaml:"Mnemonic"`
}
