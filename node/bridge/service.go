package bridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/tendermint/tendermint/types"
)

var _ apollo.Service = &Service{}

const (
	BridgeServiceName = "bridge-node"
	RPCEndpointLabel  = "bridge-rpc"
	P2PEndpointLabel  = "bridge-p2p"
)

type Service struct {
	node    *nodebuilder.Node
	store   nodebuilder.Store
	chainID string
	dir     string
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
	s.dir = dir
	return nil, nodebuilder.Init(*s.config, dir, node.Bridge)
}

func (s *Service) Init(ctx context.Context, genesis *types.GenesisDoc) error {
	s.chainID = genesis.ChainID
	return nil
}

func (s *Service) Start(ctx context.Context, inputs apollo.Endpoints) (apollo.Endpoints, error) {
	rpcEndpoint, ok := inputs[consensus.RPCEndpointLabel]
	if !ok {
		return nil, fmt.Errorf("RPC endpoint not provided")
	}

	headerHash, err := util.GetTrustedHash(ctx, rpcEndpoint)
	if err != nil {
		return nil, err
	}
	s.config.Header.TrustedHash = headerHash

	// TODO: we don't take the consensus nodes endpoints here and inject them into the config, 
	// instead we assume they are the same as the defaults

	encConf := encoding.MakeConfig(app.ModuleEncodingRegisters...)

	keysPath := filepath.Join(s.dir, "keys")
	ring, err := keyring.New(app.Name, s.config.State.KeyringBackend, keysPath, os.Stdin, encConf.Codec)
	if err != nil {
		return nil, err
	}

	s.store, err = nodebuilder.OpenStore(s.dir, ring)
	if err != nil {
		return nil, err
	}

	s.node, err = nodebuilder.NewWithConfig(node.Bridge, p2p.Network(s.chainID), s.store, s.config)
	if err != nil {
		return nil, err
	}

	fmt.Println("input", host.InfoFromHost(s.node.Host).String())

	bz, err := host.InfoFromHost(s.node.Host).MarshalJSON()
	if err != nil {
		return nil, err
	}

	var output peer.AddrInfo
	if err := output.UnmarshalJSON([]byte(string(bz))); err != nil {
		return nil, err
	}

	fmt.Println("output", output.String())

	endpoints := map[string]string{
		RPCEndpointLabel: fmt.Sprintf("http://localhost:%s", s.config.RPC.Port),
		P2PEndpointLabel: string(bz),
	}

	return endpoints, s.node.Start(ctx)
}

func (s *Service) Stop(ctx context.Context) error {
	if err := s.node.Stop(ctx); err != nil {
		return err
	}
	return s.store.Close()
}

func getTCPAddress(addrInfo *peer.AddrInfo) (string, error) {
	for _, addr := range addrInfo.Addrs {
		for _, protocol := range addr.Protocols() {
			if protocol.Name == "tcp" {
				return fmt.Sprintf("%v/%v", addr.String(), addrInfo.ID), nil
			}
		}
	}
	return "", errors.New("no tcp address found")
}

func ParseStringIntoAddrInfo(addrInfo string) (*peer.AddrInfo, error) {
	parts := strings.SplitAfterN(addrInfo, ": [", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid address info: %s", addrInfo)
	}
	id := strings.TrimPrefix(parts[0], "{")
	addrsStr := strings.Split(strings.TrimSuffix(parts[1], "]}"), " ")
	addrs := make([]ma.Multiaddr, len(addrsStr))
	var err error
	for idx, addr := range addrsStr {
		addrs[idx], err = ma.NewMultiaddr(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address %s: %w", addr, err)
		}
	}
	return &peer.AddrInfo{
		ID:    peer.ID(id),
		Addrs: addrs,
	}, nil
}
