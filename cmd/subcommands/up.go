package cmd

import (
	"context"
	"flag"
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

	// Define command line flag for a single address
	address := flag.String("address", getEnv("ADDRESS", "0.0.0.0"), "Listen address for RPC, API, and GRPC-Web")

	flag.Parse()

	consensusCfg := testnode.DefaultConfig().
		WithTendermintConfig(app.DefaultConsensusConfig()).
		WithAppConfig(app.DefaultAppConfig())

	consensusCfg.TmConfig.RPC.ListenAddress = "tcp://" + *address + ":26657"
	consensusCfg.AppConfig.API.Address = "tcp://" + *address + ":1317"
	consensusCfg.AppConfig.GRPCWeb.Address = *address + ":9091"
	consensusCfg.AppConfig.GRPC.Address = *address + ":9090"

	lightCfg := nodebuilder.DefaultConfig(node.Light)
	lightCfg.RPC.SkipAuth = true
	lightCfg.RPC.Address = *address
	lightCfg.RPC.Port = "26658"

	return apollo.Run(ctx, dir, genesis.NewDefaultGenesis(),
		consensus.New(consensusCfg),
		faucet.New(faucet.DefaultConfig()),
		bridge.New(nodebuilder.DefaultConfig(node.Bridge)),
		light.New(lightCfg),
	)
}

// Helper function to get environment variable or fallback to a default value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
