package bridge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/app/encoding"
	"github.com/celestiaorg/celestia-node/nodebuilder"
	"github.com/celestiaorg/celestia-node/nodebuilder/node"
	"github.com/celestiaorg/celestia-node/nodebuilder/p2p"
	"github.com/cmwaters/apollo"
	"github.com/cmwaters/apollo/genesis"
	"github.com/cmwaters/apollo/node/consensus"
	"github.com/cmwaters/apollo/node/util"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/tendermint/tendermint/types"
)

var _ apollo.Service = &Service{}

const (
	BridgeServiceName = "bridge-node"
	RPCEndpointLabel  = "bridge-rpc"
	P2PEndpointLabel  = "bridge-p2p"
	// use a different port for the bridge
	RPCPort = "36658"
)

type Service struct {
	node    *nodebuilder.Node
	store   nodebuilder.Store
	chainID string
	config  *nodebuilder.Config
}

func New(config *nodebuilder.Config) *Service {
	return &Service{
		config: config,
	}
}

func (s *Service) Name() string {
	return BridgeServiceName
}

func (s *Service) EndpointsNeeded() []string {
	return []string{consensus.RPCEndpointLabel, consensus.GRPCEndpointLabel}
}

func (s *Service) EndpointsProvided() []string {
	return []string{RPCEndpointLabel, P2PEndpointLabel}
}

func (s *Service) Setup(ctx context.Context, dir string, pendingGenesis *types.GenesisDoc) (genesis.Modifier, error) {
	return nil, nodebuilder.Init(*s.config, dir, node.Bridge)
}

func (s *Service) Start(ctx context.Context, dir string, genesis *types.GenesisDoc, inputs apollo.Endpoints) (apollo.Endpoints, error) {
	s.chainID = genesis.ChainID
	rpcEndpoint, ok := inputs[consensus.RPCEndpointLabel]
	if !ok {
		return nil, fmt.Errorf("RPC endpoint not provided")
	}

	headerHash, err := util.GetTrustedHash(ctx, rpcEndpoint)
	if err != nil {
		return nil, err
	}
	s.config.Header.TrustedHash = headerHash
	s.config.RPC.Port = RPCPort
	s.config.RPC.SkipAuth = true

	encConf := encoding.MakeConfig(app.ModuleEncodingRegisters...)

	keysPath := filepath.Join(dir, "keys")
	ring, err := keyring.New(app.Name, s.config.State.KeyringBackend, keysPath, os.Stdin, encConf.Codec)
	if err != nil {
		return nil, err
	}

	s.store, err = nodebuilder.OpenStore(dir, ring)
	if err != nil {
		return nil, err
	}

	s.node, err = nodebuilder.NewWithConfig(node.Bridge, p2p.Network(s.chainID), s.store, s.config)
	if err != nil {
		return nil, err
	}

	addrInfo, err := host.InfoFromHost(s.node.Host).MarshalJSON()
	if err != nil {
		return nil, err
	}

	endpoints := map[string]string{
		RPCEndpointLabel: fmt.Sprintf("ws://localhost:%s", s.config.RPC.Port),
		P2PEndpointLabel: string(addrInfo),
	}

	return endpoints, s.node.Start(ctx)
}

func (s *Service) Stop(ctx context.Context) error {
	if err := s.node.Stop(ctx); err != nil {
		return err
	}
	return s.store.Close()
}
