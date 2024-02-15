package codec

import (
	"cosmossdk.io/simapp/params"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/evmos/ethermint/encoding"
)

// Codec is the application-wide Amino codec and is initialized upon package loading.
var (
	AminoCodec     *codec.LegacyAmino
	EncodingConfig params.EncodingConfig
)

// SetCodec sets encoding config.
func SetCodec() {
	EncodingConfig = encoding.MakeTestEncodingConfig()
	AminoCodec = EncodingConfig.Amino
}
