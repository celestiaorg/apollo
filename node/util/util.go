package util

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

func ParsePort(endpoint string) (string, error) {
	split := strings.Split(endpoint, ":")
	if len(split) == 0 {
		return "", fmt.Errorf("failed to parse port from endpoint: %s", endpoint)
	}
	port := strings.Split(split[len(split)-1], "/")[0]

	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("failed to parse port from endpoint: %s", endpoint)
	}

	return port, nil
}
