package grpc

import (
	"context"

	"github.com/cosmos/cosmos-sdk/types/query"
	ibcclienttypes "github.com/cosmos/ibc-go/v3/modules/core/02-client/types"
	ibcchantypes "github.com/cosmos/ibc-go/v3/modules/core/04-channel/types"
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

func (c *Client) GetIBCClientQueryClient() ibcclienttypes.QueryClient {
	return ibcclienttypes.NewQueryClient(c)
}
func (c *Client) AllChainsTrace(ctx context.Context) ([]OpenChannel, error) {
	client := c.GetIBCChannQueryClient()
	client_status := c.GetIBCClientQueryClient()

	var OpenChannels []OpenChannel

	channelsres, err := client.Channels(
		context.Background(),
		&ibcchantypes.QueryChannelsRequest{
			Pagination: &query.PageRequest{
				Limit: 100000,
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

			clientstateres, err := client.ChannelClientState(
				context.Background(),
				&ibcchantypes.QueryChannelClientStateRequest{
					PortId:    Channel.PortId,
					ChannelId: Channel.ChannelId,
				},
			)
			if err != nil {
				return nil, err
			}

			clientstate := clientstateres.GetIdentifiedClientState()
			clientstatus, err := client_status.ClientStatus(
				context.Background(),
				&ibcclienttypes.QueryClientStatusRequest{
					ClientId: clientstate.ClientId,
				},
			)
			if err != nil {
				return nil, err
			}
			if clientstatus.Status == "Expired" {
				continue
			}

			State := clientstate.GetClientState()
			err = State.Unmarshal(State.Value)
			if err != nil {
				return nil, err
			}
			OpenChannel.ClientId = clientstate.ClientId
			OpenChannel.ChannelId = Channel.ChannelId
			OpenChannel.ClientChainId = State.TypeUrl

			channelres, err := client.Channel(
				context.Background(),
				&ibcchantypes.QueryChannelRequest{
					PortId:    Channel.PortId,
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
