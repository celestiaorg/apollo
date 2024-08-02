package cmd

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/celestiaorg/apollo"
	"github.com/celestiaorg/apollo/faucet"
	"github.com/celestiaorg/apollo/genesis"
	"github.com/celestiaorg/apollo/node/bridge"
	"github.com/celestiaorg/apollo/node/consensus"
	"github.com/celestiaorg/apollo/node/light"
	"github.com/spf13/cobra"

	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/test/util/testnode"
	"github.com/celestiaorg/celestia-node/nodebuilder"
	"github.com/celestiaorg/celestia-node/nodebuilder/node"
)

const ApolloDir = ".apollo"

func NewUpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Starts the Apollo network.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)

			go func() {
				<-c
				cancel()
			}()

			return Run(ctx)
		},
	}

	return cmd
}

func Run(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(homeDir, ApolloDir)

	consensusCfg := testnode.DefaultConfig().
		WithTendermintConfig(app.DefaultConsensusConfig()).
		WithAppConfig(app.DefaultAppConfig())

	lightCfg := nodebuilder.DefaultConfig(node.Light)
	lightCfg.RPC.SkipAuth = true

	return apollo.Run(ctx, dir, genesis.NewDefaultGenesis(),
		consensus.New(consensusCfg),
		faucet.New(faucet.DefaultConfig()),
		bridge.New(nodebuilder.DefaultConfig(node.Bridge)),
		light.New(lightCfg),
	)
}
