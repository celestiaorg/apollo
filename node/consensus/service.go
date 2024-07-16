package consensus

import (
	"context"
	"path/filepath"
	"time"

	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/app/encoding"
	"github.com/celestiaorg/celestia-app/pkg/appconsts"
	"github.com/celestiaorg/celestia-app/test/util/testnode"
	"github.com/cmwaters/apollo"
	"github.com/cmwaters/apollo/genesis"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/config"
	tmos "github.com/tendermint/tendermint/libs/os"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/types"
)

const (
	ConsensusServiceName = "consensus-node"
	RPCEndpointLabel     = "comet-rpc"
	GRPCEndpointLabel    = "cosmos-sdk-grpc"
	APIEndpointLabel     = "cosmos-sdk-api"
	APIDocsLabel         = "cosmos-api-docs"
	DocsEndpint          = "https://docs.cosmos.network/api"
)

type Config = testnode.Config

var _ apollo.Service = &Service{}

var (
	cdc = encoding.MakeConfig(app.ModuleEncodingRegisters...)
)

type Service struct {
	testnode.Context
	config  *Config
	chainID string
	closers []func() error
	kr      keyring.Keyring
}

func New(config *Config, kr keyring.Keyring) *Service {
	// override some config values
	config.TmConfig.TxIndex.Indexer = "kv"
	return &Service{
		config: config,
		kr:     kr,
	}
}

func (s *Service) Name() string {
	return ConsensusServiceName
}

func (s *Service) EndpointsNeeded() []string {
	return []string{}
}

func (s *Service) EndpointsProvided() []string {
	return []string{RPCEndpointLabel, GRPCEndpointLabel, APIEndpointLabel, APIDocsLabel}
}

func (s *Service) Setup(ctx context.Context, dir string, pendingGenesis *types.GenesisDoc) (genesis.Modifier, error) {
	record, err := s.kr.Key(ConsensusServiceName)
	if err != nil {
		return nil, err
	}
	pubKey, err := record.GetPubKey()
	if err != nil {
		return nil, err
	}

	s.config.TmConfig.SetRoot(dir)

	pvStateFile := s.config.TmConfig.PrivValidatorStateFile()
	if err := tmos.EnsureDir(filepath.Dir(pvStateFile), 0o777); err != nil {
		return nil, err
	}
	pvKeyFile := s.config.TmConfig.PrivValidatorKeyFile()
	if err := tmos.EnsureDir(filepath.Dir(pvKeyFile), 0o777); err != nil {
		return nil, err
	}

	filePV := privval.LoadOrGenFilePV(pvKeyFile, pvStateFile)
	filePV.Save()

	val := genesis.NewDefaultValidator(ConsensusServiceName)
	val.ConsensusKey = filePV.Key.PrivKey
	genTx, err := val.GenTx(cdc, s.kr, pendingGenesis.ChainID)
	if err != nil {
		return nil, err
	}

	genTxBytes, err := cdc.TxConfig.TxJSONEncoder()(genTx)
	if err != nil {
		return nil, err
	}

	configFilePath := filepath.Join(dir, "config", "config.toml")
	config.WriteConfigFile(configFilePath, s.config.TmConfig)

	appConfigFilePath := filepath.Join(dir, "config", "app.toml")
	serverconfig.WriteConfigFile(appConfigFilePath, s.config.AppConfig)

	genModifier := AddValidator(
		pubKey,
		sdk.NewCoins(sdk.NewInt64Coin(appconsts.BondDenom, val.InitialTokens)),
		genTxBytes,
	)
	return genModifier, nil
}

func (s *Service) Start(ctx context.Context, dir string, genesis *types.GenesisDoc, inputs apollo.Endpoints) (apollo.Endpoints, error) {
	s.config.TmConfig.SetRoot(dir)
	if err := genesis.SaveAs(s.config.TmConfig.GenesisFile()); err != nil {
		return nil, err
	}
	s.chainID = genesis.ChainID
	s.config.TmConfig.PrivValidatorStateFile()
	s.config.TmConfig.Storage.DiscardABCIResponses = false

	tmNode, app, err := NewCometNode(dir, s.config)
	if err != nil {
		return nil, err
	}

	nodeCtx := testnode.NewContext(ctx, s.kr, s.config.TmConfig, s.chainID)

	nodeCtx, _, err = testnode.StartNode(tmNode, nodeCtx)
	if err != nil {
		return nil, err
	}
	stopNode := func() error {
		if err := tmNode.Stop(); err != nil {
			return err
		}
		tmNode.Wait()
		return nil
	}

	nodeCtx, cleanupGRPC, err := testnode.StartGRPCServer(app, s.config.AppConfig, nodeCtx)
	if err != nil {
		return nil, err
	}

	apiServer, err := StartAPIServer(app, *s.config.AppConfig, nodeCtx)
	if err != nil {
		return nil, err
	}
	s.Context = nodeCtx

	if _, err := nodeCtx.WaitForHeightWithTimeout(1, time.Minute); err != nil {
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
		APIDocsLabel:      DocsEndpint,
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
