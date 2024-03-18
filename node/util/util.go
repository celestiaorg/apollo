package util

import (
	"context"
	"fmt"

	rpcclient "github.com/tendermint/tendermint/rpc/client/http"
)

func GetTrustedHash(ctx context.Context, rpcEndpoint string) (string, error) {
	client, err := rpcclient.New(rpcEndpoint, "/websocket")
	if err != nil {
		return "", fmt.Errorf("failed to create RPC client: %w", err)
	}
	firstHeight := int64(1)
	header, err := client.Header(ctx, &firstHeight)
	if err != nil {
		return "", fmt.Errorf("failed to query header at height 1: %w", err)
	}

	return header.Header.Hash().String(), nil
}
