package consensus

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/celestiaorg/celestia-app/test/util/testnode"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server/api"
	srvconfig "github.com/cosmos/cosmos-sdk/server/config"
	srvtypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	dbm "github.com/tendermint/tm-db"
)

// FIXME: these can be removed once this package upgrades to v2

// NewCometNode creates a ready to use comet node that operates a single
// validator celestia-app network. It expects that all configuration files are
// already initialized and saved to the baseDir.
func NewCometNode(baseDir string, cfg *Config) (*node.Node, srvtypes.Application, error) {
	var logger log.Logger
	if cfg.SupressLogs {
		logger = log.NewNopLogger()
	} else {
		logger = log.NewTMLogger(log.NewSyncWriter(os.Stdout))
		logger = log.NewFilter(logger, log.AllowError())
	}

	dbPath := filepath.Join(cfg.TmConfig.RootDir, "data")
	db, err := dbm.NewGoLevelDB("application", dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create new GoLevelDB: %w", err)
	}

	cfg.AppOptions.Set(flags.FlagHome, baseDir)

	app := cfg.AppCreator(logger, db, nil, cfg.AppOptions)

	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.TmConfig.NodeKeyFile())
	if err != nil {
		return nil, nil, err
	}

	tmNode, err := node.NewNode(
		cfg.TmConfig,
		privval.LoadOrGenFilePV(cfg.TmConfig.PrivValidatorKeyFile(), cfg.TmConfig.PrivValidatorStateFile()),
		nodeKey,
		proxy.NewLocalClientCreator(app),
		node.DefaultGenesisDocProviderFunc(cfg.TmConfig),
		node.DefaultDBProvider,
		node.DefaultMetricsProvider(cfg.TmConfig.Instrumentation),
		logger,
	)

	return tmNode, app, err
}

func StartAPIServer(app srvtypes.Application, appCfg srvconfig.Config, cctx testnode.Context) (*api.Server, error) {
	apiSrv := api.New(cctx.Context, log.NewNopLogger())
	app.RegisterAPIRoutes(apiSrv, appCfg.API)
	errCh := make(chan error)
	go func() {
		if err := apiSrv.Start(appCfg); err != nil {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return nil, err

	case <-time.After(srvtypes.ServerStartTime): // assume server started successfully
	}
	return apiSrv, nil
}
