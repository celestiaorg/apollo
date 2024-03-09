package apollo

import (
	"context"
	"fmt"

	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/app/encoding"
	"github.com/cmwaters/apollo/genesis"
	"github.com/tendermint/tendermint/types"
)

var (
	cdc = encoding.MakeConfig(app.ModuleEncodingRegisters...)
)

func Codec() encoding.Config {
	return cdc
}

type Service interface {
	Name() string
	EndpointsNeeded() []string
	EndpointsProvided() []string
	Setup(_ context.Context, dir string, pendingGenesis *types.GenesisDoc) (genesis.Modifier, error)
	Init(_ context.Context, genesis *types.GenesisDoc) error
	Start(_ context.Context, inputs Endpoints) (Endpoints, error)
	Stop(context.Context) error
}

type Endpoints map[string]string

func (e Endpoints) String() string {
	var output string
	for name, endpoint := range e {
		output += fmt.Sprintf("%s: %s\t", name, endpoint)
	}
	return output
}
