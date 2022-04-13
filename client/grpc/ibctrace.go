package grpc

import (
	"context"
	
	"github.com/cosmos/cosmos-sdk/types/query"
	ibcchantypes "github.com/cosmos/ibc-go/v2/modules/core/04-channel/types"
)

type OpenChannel struct {
	ChannelId     string
	ClientId      string
	ClientChainId string
	ConnectionIds []string
}

func (c *Client) GetIBCChannQueryClient() ibcchantypes.QueryClient {
	return ibcchantypes.NewQueryClient(c)
}

func (c *Client) AllChainsTrace(ctx context.Context) ([]OpenChannel, error) {
	client := c.GetIBCChannQueryClient()

	var OpenChannels []OpenChannel

	channelsres, err := client.Channels(
		context.Background(),
		&ibcchantypes.QueryChannelsRequest{
				Pagination: &query.PageRequest{
				Limit: 1000000,
			},
		},
	)
	if err != nil {
		return nil, err
	}
	Channels := channelsres.GetChannels()

	for _, Channel := range Channels {
		var OpenChannel OpenChannel
		if Channel.State == 3 {
			OpenChannel.ChannelId = Channel.ChannelId
			clientstateres, err := client.ChannelClientState(
				context.Background(),
				&ibcchantypes.QueryChannelClientStateRequest{
					PortId:    "transfer",
					ChannelId: Channel.ChannelId,
				},
			)
			if err != nil {
				return nil, err
			}
			clientstate := clientstateres.GetIdentifiedClientState()
			OpenChannel.ClientId = clientstate.ClientId
			State := clientstate.GetClientState()
			err = State.Unmarshal(State.Value)
			if err != nil {
				return nil, err
			}
			OpenChannel.ClientChainId = State.TypeUrl

			channelres, err := client.Channel(
				context.Background(),
				&ibcchantypes.QueryChannelRequest{
					PortId:    "transfer",
					ChannelId: Channel.ChannelId,
				},
			)
			if err != nil {
				return nil, err
			}
			channelinfo := channelres.GetChannel()

			for _, connectionId := range channelinfo.ConnectionHops {
				ConnectionIds := OpenChannel.ConnectionIds
				OpenChannel.ConnectionIds = append(ConnectionIds, connectionId)
			}

		}
		OpenChannels = append(OpenChannels, OpenChannel)

	}
	return OpenChannels, nil

}
