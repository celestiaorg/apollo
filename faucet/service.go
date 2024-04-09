package faucet

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"log"

	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/app/encoding"
	"github.com/celestiaorg/celestia-app/pkg/appconsts"
	"github.com/celestiaorg/celestia-app/pkg/user"
	"github.com/cmwaters/apollo"
	"github.com/cmwaters/apollo/genesis"
	"github.com/cmwaters/apollo/node/consensus"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	bank "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/tendermint/tendermint/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	_   apollo.Service = &Service{}
	cdc                = encoding.MakeConfig(app.ModuleEncodingRegisters...)

	//go:embed web/*
	web embed.FS
)

const (
	FaucetServiceName = "faucet"
	FaucetAPILabel    = "faucet-api"
)

type Service struct {
	config    *Config
	apiServer http.Server
	store     *Store
	keyring   keyring.Keyring
}

func New(config *Config) *Service {
	return &Service{
		config: config,
	}
}

func (s *Service) Name() string {
	return FaucetServiceName
}

func (s *Service) EndpointsNeeded() []string {
	return []string{consensus.GRPCEndpointLabel}
}

func (s *Service) EndpointsProvided() []string {
	return []string{FaucetAPILabel}
}

func (s *Service) Setup(ctx context.Context, dir string, pendingGenesis *types.GenesisDoc) (genesis.Modifier, error) {
	var err error
	s.keyring, err = keyring.New(app.Name, keyring.BackendTest, dir, nil, cdc.Codec)
	if err != nil {
		return nil, err
	}

	s.store, err = NewStore(dir, s.config)
	if err != nil {
		return nil, err
	}

	record, err := s.keyring.Key(FaucetServiceName)
	if err != nil {
		if errors.Is(err, sdkerrors.ErrKeyNotFound) {
			// if no key exists, create one
			path := hd.CreateHDPath(sdk.CoinType, 0, 0).String()
			record, _, err = s.keyring.NewMnemonic(FaucetServiceName, keyring.English, keyring.DefaultBIP39Passphrase, path, hd.Secp256k1)
			if err != nil {
				return nil, err
			}
			fmt.Println("created new account")
		} else {
			return nil, err
		}
	}

	address, err := record.GetAddress()
	if err != nil {
		return nil, fmt.Errorf("error getting address from keyring: %w", err)
	}

	return genesis.FundAccounts(apollo.Codec().Codec, []sdk.AccAddress{address}, sdk.NewCoin(app.BondDenom, sdk.NewIntFromUint64(s.config.InitialSupply))), nil
}

func (s *Service) Init(ctx context.Context, genesis *types.GenesisDoc) error {
	// No specific initialization required for the faucet service
	return nil
}

func (s *Service) Start(ctx context.Context, _ string, input apollo.Endpoints) (apollo.Endpoints, error) {
	conn, err := grpc.Dial(input[consensus.GRPCEndpointLabel], grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	signer, err := user.SetupSingleSigner(ctx, s.keyring, conn, cdc)
	if err != nil {
		return nil, fmt.Errorf("error setting up signer: %w", err)
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		state := State{
			PerRequestAmount: s.config.Amount,
			Address:          signer.Address().String(),
			PerAccountLimit:  s.config.PerAccountLimit,
			GlobalLimit:      s.config.GlobalLimit,
		}

		stateJSON, err := json.Marshal(state)
		if err != nil {
			panic(err)
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(stateJSON); err != nil {
			log.Printf("error writing state response: %s", err.Error())
		}
	})

	handler.HandleFunc("/fund/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		addrStr := strings.TrimPrefix(r.URL.Path, "/fund/")
		addr, err := sdk.AccAddressFromBech32(addrStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid address %s: %s", addrStr, err.Error()), http.StatusBadRequest)
			log.Printf("invalid address from request %s: %s", addrStr, err.Error())
			return
		}

		err = s.store.RequestFunds(addr)
		if err != nil {
			http.Error(w, fmt.Sprintf("error requesting funds for account %v: %s", addr, err.Error()), http.StatusInternalServerError)
			log.Printf("error requesting funds for account %v: %s", addr, err.Error())
			return
		}

		msgSend := bank.NewMsgSend(signer.Address(), addr, sdk.NewCoins(sdk.NewInt64Coin(app.BondDenom, int64(s.config.Amount))))

		// TODO: the gas estimation could be more dynamic but this should be safe enough for all MsgSend transactions
		resp, err := signer.SubmitTx(r.Context(), []sdk.Msg{msgSend}, user.SetGasLimitAndFee(200_000, appconsts.DefaultMinGasPrice))
		// TODO: if this fails we should ideally revert the request funds changes to the database
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("error submitting send tx to account %s: %s", addr.String(), err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(fmt.Sprintf("Successfully sent %d %s to %s. Transaction hash: %s", s.config.Amount, app.BondDenom, addr.String(), resp.TxHash)))
		if err != nil {
			log.Printf("error writing response: %s", err.Error())
		}
	})

	// serve the static directory
	if s.config.EnableGUI {
		fileSystem, err := fs.Sub(web, "web")
		if err != nil {
			return nil, err
		}
		handler.Handle("/", http.FileServer(http.FS(fileSystem)))
	}

	listener, err := net.Listen("tcp", s.config.APIAddress)
	if err != nil {
		return nil, err
	}
	s.apiServer = http.Server{Handler: handler}
	errCh := make(chan error)
	go func() {
		errCh <- s.apiServer.Serve(listener)
	}()

	select {
	case err := <-errCh:
		return nil, err
	case <-time.After(2 * time.Second):
	}

	return apollo.Endpoints{
		FaucetAPILabel: s.config.APIAddress,
	}, nil
}

func (s *Service) Stop(ctx context.Context) error {
	return s.apiServer.Shutdown(ctx)
}

type State struct {
	PerRequestAmount uint64 `json:"per_request_amount"`
	Address          string `json:"address"`
	PerAccountLimit  Limit  `json:"per_account_limit"`
	GlobalLimit      Limit  `json:"global_limit"`
}
