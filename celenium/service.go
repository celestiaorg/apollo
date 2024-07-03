package celenium

import (
	"context"
	"fmt"

	tc "github.com/testcontainers/testcontainers-go/modules/compose"
	"go.uber.org/multierr"

	"github.com/cmwaters/apollo"
	"github.com/cmwaters/apollo/genesis"
	"github.com/cmwaters/apollo/node/bridge"
	"github.com/cmwaters/apollo/node/consensus"
	"github.com/tendermint/tendermint/types"
)

const (
	CeleniumServiceName = "celenium"
	DatabaseLabel       = "celenium-database"
	FrontendLabel       = "celenium-frontend"
	BackendAPILabel     = "celenium-backend"
)

type Service struct {
	cancel context.CancelFunc
	docker DockerService
}

func New() *Service {
	return &Service{}
}

type DockerService interface {
	Down(ctx context.Context, opts ...tc.StackDownOption) error
}

func (s *Service) EndpointsNeeded() []string {
	return []string{consensus.RPCEndpointLabel, bridge.RPCEndpointLabel}
}

func (s *Service) EndpointsProvided() []string {
	return []string{DatabaseLabel, FrontendLabel, BackendAPILabel}
}

func (s *Service) Setup(ctx context.Context, dir string, pendingGenesis *types.GenesisDoc) (genesis.Modifier, error) {
	return nil, nil
}

func (s *Service) Start(ctx context.Context, dir string, genesis *types.GenesisDoc, inputs apollo.Endpoints) (endpoints apollo.Endpoints, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// TODO: this is a temporary solution to get the docker-compose file,
	// but the command needs to be able to be run from anywhere on fs
	compose, err := tc.NewDockerCompose("celenium/docker-compose.yml")
	if err != nil {
		return nil, fmt.Errorf("failed to create docker-compose: %w", err)
	}

	s.docker = compose
	err = compose.Up(ctx)
	if err != nil {
		stopErr := s.Stop(ctx)
		return nil, multierr.Combine(err, stopErr)
	}

	endpoints = apollo.Endpoints{
		DatabaseLabel:   "http://127.0.0.1:5432",
		FrontendLabel:   "http://127.0.0.1:3030",
		BackendAPILabel: "http://127.0.0.1:9876",
	}

	return nil, err
}

func (s *Service) Stop(ctx context.Context) error {
	s.docker.Down(ctx, tc.RemoveOrphans(true), tc.RemoveVolumes(true))
	s.cancel()
	return nil
}

func (s *Service) Name() string {
	return CeleniumServiceName
}
