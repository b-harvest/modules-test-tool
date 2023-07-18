package codec

import (
	"github.com/cosmos/cosmos-sdk/codec"

	liqapp "github.com/crescent-network/crescent/v5/app"
	liqappparams "github.com/crescent-network/crescent/v5/app/params"
)

// Codec is the application-wide Amino codec and is initialized upon package loading.
var (
	AppCodec       codec.Codec
	AminoCodec     *codec.LegacyAmino
	EncodingConfig liqappparams.EncodingConfig
)

// SetCodec sets encoding config.
func SetCodec() {
	EncodingConfig = liqapp.MakeEncodingConfig()
	AppCodec = EncodingConfig.Marshaler
	AminoCodec = EncodingConfig.Amino
}
