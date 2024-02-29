package consensus

import (
	"context"

	"github.com/celestiaorg/celestia-app/test/util/genesis"
	"github.com/celestiaorg/celestia-app/test/util/testnode"
	"github.com/cmwaters/apollo"
)

const (
	ConsensusNodeLabel = "consensus-node"
	RPCEndpointLabel   = "comet-rpc"
	GRPCEndpointLabel  = "cosmos-sdk-grpc"
	APIEndpointLabel   = "cosmos-sdk-api"
)

var _ apollo.Service = &Service{}

type Service struct {
	config  *testnode.Config
	closers []func() error
}

func New(config *testnode.Config) *Service {
	return &Service{
		config: config,
	}
}

func (s *Service) Name() string {
	return ConsensusNodeLabel
}

func (s *Service) EndpointsNeeded() []string {
	return []string{}
}

func (s *Service) Endpoints() []string {
	return []string{RPCEndpointLabel, GRPCEndpointLabel, APIEndpointLabel}
}

func (s *Service) Start(ctx context.Context, dir string, inputs apollo.Endpoints) (apollo.Endpoints, error) {
	baseDir, err := genesis.InitFiles(dir, s.config.TmConfig, s.config.Genesis, 0)
	if err != nil {
		return nil, err
	}

	tmNode, app, err := testnode.NewCometNode(baseDir, &s.config.UniversalTestingConfig)
	if err != nil {
		return nil, err
	}

	nodeCtx := testnode.NewContext(ctx, s.config.Genesis.Keyring(), s.config.TmConfig, s.config.Genesis.ChainID, s.config.AppConfig.API.Address)

	nodeCtx, stopNode, err := testnode.StartNode(tmNode, nodeCtx)
	if err != nil {
		return nil, err
	}

	nodeCtx, cleanupGRPC, err := testnode.StartGRPCServer(app, s.config.AppConfig, nodeCtx)
	if err != nil {
		return nil, err
	}

	apiServer, err := testnode.StartAPIServer(app, *s.config.AppConfig, nodeCtx)
	if err != nil {
		return nil, err
	}

	// close these sub services in reverse order
	s.closers = []func() error{
		apiServer.Close, cleanupGRPC, stopNode,
	}

	return apollo.Endpoints{
		RPCEndpointLabel:  s.config.TmConfig.RPC.ListenAddress,
		GRPCEndpointLabel: s.config.AppConfig.GRPC.Address,
		APIEndpointLabel:  s.config.AppConfig.API.Address,
	}, nil
}

func (s *Service) Stop(context.Context) error {
	for _, closer := range s.closers {
		if err := closer(); err != nil {
			return err
		}
	}
	return nil
}
