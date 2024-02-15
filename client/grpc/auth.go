package grpc

import (
	"context"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	etherminttypes "github.com/evmos/ethermint/types"
)

// GetAuthQueryClient returns a object of queryClient
func (c *Client) GetAuthQueryClient() authtypes.QueryClient {
	return authtypes.NewQueryClient(c)
}

// GetBaseAccountInfo returns base account information
func (c *Client) GetBaseAccountInfo(ctx context.Context, address string) (etherminttypes.EthAccount, error) {
	client := c.GetAuthQueryClient()

	req := authtypes.QueryAccountRequest{
		Address: address,
	}

	resp, err := client.Account(ctx, &req)
	if err != nil {
		return etherminttypes.EthAccount{}, err
	}

	var acc etherminttypes.EthAccount
	err = acc.Unmarshal(resp.GetAccount().Value)
	if err != nil {
		return etherminttypes.EthAccount{}, err
	}

	return acc, nil
}
