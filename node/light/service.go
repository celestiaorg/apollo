package light

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
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/tendermint/tendermint/types"
)

var _ apollo.Service = &Service{}

const (
	LightServiceName = "light-node"
	RPCEndpointLabel = "node-rpc"
)

type Service struct {
	node    *nodebuilder.Node
	chainID string
	dir     string
	config  nodebuilder.Config
}

func New(config nodebuilder.Config) *Service {
	return &Service{
		config: config,
	}
}

func (s *Service) Name() string {
	return LightServiceName
}

func (s *Service) EndpointsNeeded() []string {
	return []string{consensus.RPCEndpointLabel, consensus.GRPCEndpointLabel}
}

func (s *Service) EndpointsProvided() []string {
	return []string{RPCEndpointLabel}
}

func (s *Service) Setup(ctx context.Context, dir string, pendingGenesis *types.GenesisDoc) (genesis.Modifier, error) {
	s.dir = dir
	return nil, nodebuilder.Init(s.config, dir, node.Light)
}

func (s *Service) Init(ctx context.Context, genesis *types.GenesisDoc) error {
	s.chainID = genesis.ChainID
	return nil
}

func (s *Service) Start(ctx context.Context, inputs apollo.Endpoints) (apollo.Endpoints, error) {
	encConf := encoding.MakeConfig(app.ModuleEncodingRegisters...)

	keysPath := filepath.Join(s.dir, "keys")
	ring, err := keyring.New(app.Name, s.config.State.KeyringBackend, keysPath, os.Stdin, encConf.Codec)
	if err != nil {
		return nil, err
	}

	store, err := nodebuilder.OpenStore(s.dir, ring)
	if err != nil {
		return nil, err
	}

	s.node, err = nodebuilder.New(node.Light, p2p.Network(s.chainID), store)
	if err != nil {
		return nil, err
	}

	endpoints := map[string]string{
		RPCEndpointLabel: fmt.Sprintf("http://localhost:%s", s.config.RPC.Port),
	}

	return endpoints, s.node.Start(ctx)
}

func (s *Service) Stop(ctx context.Context) error {
	return s.node.Stop(ctx)
}
