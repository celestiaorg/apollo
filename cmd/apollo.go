package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/test/util/testnode"
	"github.com/celestiaorg/celestia-node/nodebuilder"
	"github.com/celestiaorg/celestia-node/nodebuilder/node"
	"github.com/cmwaters/apollo"
	"github.com/cmwaters/apollo/faucet"
	"github.com/cmwaters/apollo/genesis"
	"github.com/cmwaters/apollo/node/bridge"
	"github.com/cmwaters/apollo/node/consensus"
	"github.com/cmwaters/apollo/node/light"
)

const ApolloDir = ".apollo"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		cancel()
	}()

	if err := Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func Run(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(homeDir, ApolloDir)

	cfg := testnode.DefaultConfig().
		WithTendermintConfig(app.DefaultConsensusConfig()).
		WithAppConfig(app.DefaultAppConfig())

	return apollo.Run(ctx, dir, genesis.NewDefaultGenesis(),
		consensus.New(cfg),
		faucet.New(faucet.DefaultConfig()),
		bridge.New(nodebuilder.DefaultConfig(node.Bridge)),
		light.New(nodebuilder.DefaultConfig(node.Light)),
	)
}
