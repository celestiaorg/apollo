package faucet

import (
	"context"
	"fmt"
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
	"github.com/cmwaters/apollo/node/consensus"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bank "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var _ apollo.Service = &Service{}

var (
	cdc = encoding.MakeConfig(app.ModuleEncodingRegisters...)
)

const (
	FaucetServiceName = "faucet"
	FaucetAPILabel    = "faucet-api"
	FaucetGUILabel    = "faucet-gui"
)

type Service struct {
	config Config
	server http.Server
}

func New(config Config) *Service {
	return &Service{
		config: config,
	}
}

func (s *Service) Name() string {
	return FaucetServiceName
}

func (s *Service) EndpointsNeeded() []string {
	return []string{consensus.RPCEndpointLabel, consensus.GRPCEndpointLabel}
}

func (s *Service) Endpoints() []string {
	return []string{FaucetAPILabel, FaucetGUILabel}
}

func (s *Service) Start(ctx context.Context, dir string, input apollo.Endpoints) (apollo.Endpoints, error) {
	store, err := NewStore(dir, s.config)
	if err != nil {
		return nil, err
	}

	kr, err := keyring.New(app.Name, keyring.BackendTest, dir, nil, cdc.Codec)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.Dial(input[consensus.GRPCEndpointLabel], grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	signer, err := user.SetupSingleSigner(ctx, kr, conn, cdc)
	if err != nil {
		return nil, err
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		addrStr := strings.TrimPrefix("/", r.URL.Path)
		addr, err := sdk.AccAddressFromBech32(addrStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid address: %s", err.Error()), http.StatusBadRequest)
			return
		}

		err = store.RequestFunds(addr)
		if err != nil {
			http.Error(w, fmt.Sprintf("error requesting funds for account %v: %s", addr, err.Error()), http.StatusInternalServerError)
			log.Printf("error requesting funds for account %v: %s", addr, err.Error())
			return
		}

		msgSend := bank.NewMsgSend(signer.Address(), addr, sdk.NewCoins(sdk.NewInt64Coin(app.BondDenom, int64(s.config.Amount))))

		// TODO: the gas estimation could be more dynamic but this should be safe enough for all MsgSend transactions
		resp, err := signer.SubmitTx(r.Context(), []sdk.Msg{msgSend}, user.SetGasLimitAndFee(100000, appconsts.DefaultMinGasPrice))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("error submitting send tx to account %v: %s", addr, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(fmt.Sprintf("Successfully sent %d %s to %s. Transaction hash: %s", s.config.Amount, app.BondDenom, addr.String(), resp.TxHash)))
		if err != nil {
			log.Printf("error writing response: %s", err.Error())
		}
	})
	listener, err := net.Listen("tcp", s.config.APIAddress)
	if err != nil {
		return nil, err
	}
	s.server = http.Server{Handler: handler}
	errCh := make(chan error)
	go func() {
		errCh <- s.server.Serve(listener)
	}()

	select {
	case err := <-errCh:
		return nil, err
	case <-time.After(5 * time.Second):
	}

	return apollo.Endpoints{
		FaucetAPILabel: s.config.APIAddress,
		// FaucetGUILabel: s.config.GUIAddress,
	}, nil
}

func (s *Service) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
