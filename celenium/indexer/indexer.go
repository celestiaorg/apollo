package indexer

import (
	"context"
	"fmt"

	goLibConfig "github.com/dipdup-net/go-lib/config"

	"github.com/celenium-io/celestia-indexer/pkg/indexer"
	"github.com/celenium-io/celestia-indexer/pkg/indexer/config"
	"github.com/cmwaters/apollo"
	"github.com/cmwaters/apollo/genesis"
	"github.com/cmwaters/apollo/node/bridge"
	"github.com/cmwaters/apollo/node/consensus"
	"github.com/dipdup-net/indexer-sdk/pkg/modules/stopper"
	"github.com/tendermint/tendermint/types"
)

const (
	IndexerServiceName = "indexer"
)

type Service struct {
	idx     indexer.Indexer
	stopper stopper.Module
	cfg     *config.Config
}

func New() *Service {
	return &Service{}
}

func (s *Service) EndpointsNeeded() []string {
	return []string{consensus.RPCEndpointLabel, bridge.RPCEndpointLabel}
}

func (s *Service) EndpointsProvided() []string {
	return []string{}
}

func (s *Service) Setup(ctx context.Context, dir string, pendingGenesis *types.GenesisDoc) (genesis.Modifier, error) {
	var cfg config.Config
	if err := goLibConfig.Parse("dipdup.yml", &cfg); err != nil {
		return nil, err
	}
	s.cfg = &cfg
	return nil, nil
}

func (s *Service) Start(ctx context.Context, dir string, genesis *types.GenesisDoc, inputs apollo.Endpoints) (endpoints apollo.Endpoints, err error) {
	ctx, cancel := context.WithCancel(context.Background())

	consensusDatasource := s.cfg.DataSources["node_rpc"]
	consensusDatasource.URL = "http" + inputs[consensus.RPCEndpointLabel][3:]
	fmt.Println(consensusDatasource.URL)

	bridgeDatasource := s.cfg.DataSources["dal_api"]
	bridgeDatasource.URL = inputs[bridge.RPCEndpointLabel]

	s.cfg.DataSources["node_rpc"] = consensusDatasource
	s.cfg.DataSources["dal_api"] = bridgeDatasource
	s.stopper = stopper.NewModule(cancel)
	s.idx, err = indexer.New(ctx, *s.cfg, &s.stopper)
	if err != nil {
		return nil, err
	}

	s.stopper.Start(ctx)
	s.idx.Start(ctx)
	return
}

// TODO(@distractedm1nd): or should I call something on this stopper thingy?
func (s *Service) Stop(ctx context.Context) error {
	if err := s.idx.Close(); err != nil {
		return err
	}
	return nil
}

func (s *Service) Name() string {
	return IndexerServiceName
}
