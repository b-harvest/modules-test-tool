package codec

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/encoding"
)

// Codec is the application-wide Amino codec and is initialized upon package loading.
var (
	AppCodec       codec.Codec
	AminoCodec     *codec.LegacyAmino
	EncodingConfig params.EncodingConfig
)

// SetCodec sets encoding config.
func SetCodec() {
	EncodingConfig = encoding.MakeConfig(app.ModuleBasics)
	AppCodec = EncodingConfig.Marshaler
	AminoCodec = EncodingConfig.Amino
}
