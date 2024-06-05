package apollo

import (
	"context"
	"log"

	"github.com/cmwaters/apollo/genesis"
)

// Run is the high level function which will take a list of services and
// a genesis and both initialize and run all services in the order they
// are provided. If there is an error during startup, then the new
// directory will be deleted. Cancelling the context will gracefully
// shutdown all services
func Run(ctx context.Context, dir string, genesis *genesis.Genesis, services ...Service) (err error) {
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
			// If there has been an error during startup, then
			// delete the new directory
			if cleanUpErr := manager.Cleanup(); cleanUpErr != nil {
				log.Printf("error cleaning up: %v", cleanUpErr)
			}
			return err
		}
	}

	if err := manager.Serve(ctx); err != nil {
		return err
	}

	return nil
}
