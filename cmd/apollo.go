package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/tabwriter"

	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/test/util/testnode"
	"github.com/celestiaorg/celestia-node/nodebuilder"
	"github.com/celestiaorg/celestia-node/nodebuilder/node"
	"github.com/cmwaters/apollo"
	"github.com/cmwaters/apollo/celenium/indexer"
	"github.com/cmwaters/apollo/faucet"
	"github.com/cmwaters/apollo/genesis"
	apolloKeyring "github.com/cmwaters/apollo/keyring"
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

	consensusCfg := testnode.DefaultConfig().
		WithTendermintConfig(app.DefaultConsensusConfig()).
		WithAppConfig(app.DefaultAppConfig())

	lightCfg := nodebuilder.DefaultConfig(node.Light)
	lightCfg.RPC.SkipAuth = true

	gen := genesis.NewDefaultGenesis()
	kr := apolloKeyring.New()
	modifier, err := kr.Setup(ctx, filepath.Join(dir, "keyring"))
	if err != nil {
		return err
	}

	gen = gen.WithModifiers(modifier)

	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)

	fmt.Fprintln(w, "Available Accounts")
	fmt.Fprintln(w, "==================")
	for key, record := range kr.Records {
		address, err := record.Record.GetAddress()
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s:\t%s\n", key, address)
	}
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, "Private Keys")
	fmt.Fprintln(w, "==================")
	for key, _ := range kr.Records {
		privkey, err := kr.UnsafeExportPrivKeyHex(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s:\t%s\n", key, privkey)
	}
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, "Wallet")
	fmt.Fprintln(w, "==================")
	fmt.Fprintf(w, "Mnemonic:\t%s\n", kr.Records[consensus.ConsensusServiceName].Mnemonic)
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, "Chain ID")
	fmt.Fprintln(w, "==================")
	fmt.Fprintf(w, consensusCfg.ChainID)
	fmt.Fprintln(w, "\n")

	fmt.Fprintln(w, "Genesis Timestamp")
	fmt.Fprintln(w, "==================")
	fmt.Fprintln(w, gen.GenesisTime.Unix())
	fmt.Fprintln(w, "")

	w.Flush()

	return apollo.Run(ctx, dir, gen,
		consensus.New(consensusCfg, kr.Keyring),
		faucet.New(faucet.DefaultConfig()),
		bridge.New(nodebuilder.DefaultConfig(node.Bridge)),
		light.New(lightCfg),
		indexer.New(),
	)
}
