package apollo

import (
	"context"
	"log"

	"github.com/cmwaters/apollo/genesis"
)

func Run(ctx context.Context, dir string, genesis *genesis.Genesis, services ...Service) error {
	manager, err := New(dir, genesis, services...)
	if err != nil {
		return err
	}

	if err := manager.Setup(ctx); err != nil {
		return err
	}
	defer func() {
		if err := manager.Stop(context.Background()); err != nil {
			log.Printf("error stopping manager: %v", err)
		}
	}()

	for _, service := range services {
		if err := manager.StartService(ctx, service.Name()); err != nil {
			return err
		}
	}

	if err := manager.Serve(ctx); err != nil {
		return err
	}

	return nil
}
