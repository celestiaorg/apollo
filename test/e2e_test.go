package test

import (
	"context"
	"testing"
	"time"

	"github.com/celestiaorg/apollo"
	"github.com/celestiaorg/apollo/faucet"
	"github.com/celestiaorg/apollo/genesis"
	"github.com/celestiaorg/apollo/node/bridge"
	"github.com/celestiaorg/apollo/node/consensus"
	"github.com/celestiaorg/apollo/node/light"
	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/test/util/testnode"
	"github.com/celestiaorg/celestia-node/nodebuilder"
	"github.com/celestiaorg/celestia-node/nodebuilder/node"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/rpc/client/http"
)

func TestE2E(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consensusCfg := testnode.DefaultConfig().
		WithTendermintConfig(app.DefaultConsensusConfig()).
		WithAppConfig(app.DefaultAppConfig())

	lightCfg := nodebuilder.DefaultConfig(node.Light)
	lightCfg.RPC.SkipAuth = true

	errCh := make(chan error)
	go func() {
		errCh <- apollo.Run(ctx, t.TempDir(), genesis.NewDefaultGenesis(),
			consensus.New(consensusCfg),
			faucet.New(faucet.DefaultConfig()),
			bridge.New(nodebuilder.DefaultConfig(node.Bridge)),
			light.New(lightCfg),
		)
	}()

	client, err := http.New(consensusCfg.TmConfig.RPC.ListenAddress, "/websocket")
	require.NoError(t, err)

	// wait for the block height to be greater than 1
	require.Eventually(t, func() bool {
		status, err := client.Status(ctx)
		if err != nil {
			return false
		}
		return status.SyncInfo.LatestBlockHeight > int64(1)
	}, 30*time.Second, 2*time.Second, "chain to pass height 1")

	cancel()

	err = <-errCh
	require.Equal(t, err, context.Canceled)
}
