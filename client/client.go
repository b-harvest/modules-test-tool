package client

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	ethrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/b-harvest/modules-test-tool/client/clictx"
	"github.com/b-harvest/modules-test-tool/client/grpc"
	"github.com/b-harvest/modules-test-tool/client/rpc"
)

var (
	DefaultRPCTimeout  = int64(5)
	DefaultGRPCTimeout = int64(5)
)

// Client is a wrapper for various clients.
type Client struct {
	CliCtx *clictx.Client
	RPC    *rpc.Client
	ETHRPC *ethrpc.Client
	GRPC   *grpc.Client
}

// NewClient creates a new Client with the given configuration.
func NewClient(rpcURL string, ethrpcURL string, grpcURL string) (*Client, error) {
	rpcClient, err := rpc.NewClient(rpcURL, DefaultRPCTimeout)
	if err != nil {
		return &Client{}, err
	}

	ethrpcClient, err := ethrpc.Dial(ethrpcURL)
	if err != nil {
		return &Client{}, err
	}

	grpcClient, err := grpc.NewClient(grpcURL, DefaultGRPCTimeout)
	if err != nil {
		return &Client{}, err
	}

	cliCtx := clictx.NewClient(rpcURL, rpcClient.Client)

	return &Client{
		CliCtx: cliCtx,
		RPC:    rpcClient,
		ETHRPC: ethrpcClient,
		GRPC:   grpcClient,
	}, nil
}

// GetCLIContext returns client context for the network.
func (c *Client) GetCLIContext() sdkclient.Context {
	return c.CliCtx.Context
}

// GetRPCClient returns RPC client.
func (c *Client) GetRPCClient() *rpc.Client {
	return c.RPC
}

// GetETHRPCClient returns RPC client.
func (c *Client) GetETHRPCClient() *ethrpc.Client {
	return c.ETHRPC
}

// GetGRPCClient returns GRPC client.
func (c *Client) GetGRPCClient() *grpc.Client {
	return c.GRPC
}

// Stop defers the node stop execution to the RPC and GRPC clients.
func (c Client) Stop() error {
	err := c.RPC.Stop()
	if err != nil {
		return err
	}

	c.ETHRPC.Close()

	err = c.GRPC.Close()
	if err != nil {
		return err
	}
	return nil
}
