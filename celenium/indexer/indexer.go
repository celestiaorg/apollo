package indexer

import (
	"context"
	"fmt"

	"github.com/docker/compose/v2/pkg/api"
	tc "github.com/testcontainers/testcontainers-go/modules/compose"
	"go.uber.org/multierr"

	"github.com/cmwaters/apollo"
	"github.com/cmwaters/apollo/genesis"
	"github.com/cmwaters/apollo/node/bridge"
	"github.com/cmwaters/apollo/node/consensus"
	"github.com/tendermint/tendermint/types"
)

const (
	IndexerServiceName = "indexer"
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
	return []string{}
}

func (s *Service) Setup(ctx context.Context, dir string, pendingGenesis *types.GenesisDoc) (genesis.Modifier, error) {
	return nil, nil
}

func (s *Service) Start(ctx context.Context, dir string, genesis *types.GenesisDoc, inputs apollo.Endpoints) (endpoints apollo.Endpoints, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

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

	return nil, err
}

func (s *Service) Stop(ctx context.Context) error {
	s.docker.Down(ctx, tc.RemoveOrphans(true), tc.RemoveVolumes(true))
	s.cancel()
	return nil
}

func (s *Service) Name() string {
	return IndexerServiceName
}
