package grpc

import (
	"context"

	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// GetEvmQueryClient returns a object of queryClient
func (c *Client) GetEvmQueryClient() evmtypes.QueryClient {
	return evmtypes.NewQueryClient(c)
}

// GetBaseAccountInfo returns base account information
func (c *Client) GetEvmParams(ctx context.Context) (evmtypes.Params, error) {
	client := c.GetEvmQueryClient()

	req := evmtypes.QueryParamsRequest{}

	resp, err := client.Params(ctx, &req)
	if err != nil {
		return evmtypes.Params{}, err
	}

	return resp.Params, nil
}
