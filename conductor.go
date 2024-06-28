package apollo

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/celestiaorg/celestia-node/nodebuilder/p2p"
	"github.com/cmwaters/apollo/genesis"
	"github.com/tendermint/tendermint/types"
)

//go:embed web/*
var web embed.FS

type Conductor struct {
	lock            sync.Mutex
	services        map[string]Service
	activeEndpoints Endpoints
	activeServices  map[string]Service
	startOrder      []string
	rootDir         string
	setup           bool
	genesis         *genesis.Genesis
	genesisDoc      *types.GenesisDoc
	logger          *log.Logger
}

// New creates a conductor for managing the services. If there is
// an existing genesis within the directory that is provided
// then the genesis here will be ignored
func New(dir string, genesis *genesis.Genesis, services ...Service) (*Conductor, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("no services provided")
	}
	serviceMap := make(map[string]Service)
	for _, service := range services {
		name := service.Name()
		if name == "" {
			return nil, fmt.Errorf("service name cannot be empty")
		}
		if _, ok := serviceMap[name]; ok {
			return nil, fmt.Errorf("service %s is registered twice", name)
		}
		serviceMap[name] = service
	}

	c := &Conductor{
		services:        serviceMap,
		activeEndpoints: make(map[string]string),
		activeServices:  make(map[string]Service),
		startOrder:      make([]string, 0),
		genesis:         genesis.WithChainID(string(p2p.Private)),
		rootDir:         dir,
		logger:          log.New(os.Stdout, "", log.LstdFlags),
	}
	err := c.CheckEndpoints()
	if err != nil {
		return nil, err
	}
	return c, nil
}

// CheckEndpoints makes sure that there is at least one provider
// for every endpoint that a service requires.
func (c *Conductor) CheckEndpoints() error {
	endpointMap := make(map[string]bool)
	for _, service := range c.services {
		for _, endpoint := range service.EndpointsProvided() {
			endpointMap[endpoint] = true
		}
	}

	for _, service := range c.services {
		for _, requiredEndpoint := range service.EndpointsNeeded() {
			if !endpointMap[requiredEndpoint] {
				return fmt.Errorf("required endpoint '%s' for service '%s' is not provided by any service", requiredEndpoint, service.Name())
			}
		}
	}

	return nil
}

// Setup initializes all services and generates the genesis
// to be passed to each service upon startup.
func (c *Conductor) Setup(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.logger.Printf("setting up services...")

	configDir := filepath.Join(c.rootDir, "config")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		pendingGenesis, err := c.genesis.Export()
		if err != nil {
			return err
		}
		for name, service := range c.services {
			dir := filepath.Join(c.rootDir, name)
			if err := os.MkdirAll(dir, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory for service %s: %w", name, err)
			}

			// supress output from services
			rescueStdout := os.Stdout
			_, w, _ := os.Pipe()
			os.Stdout = w

			modifier, err := service.Setup(ctx, dir, pendingGenesis)

			// restore stdout
			w.Close()
			os.Stdout = rescueStdout

			if err != nil {
				return fmt.Errorf("failed to setup service %s: %w", name, err)
			}
			if modifier != nil {
				c.genesis = c.genesis.WithModifiers(modifier)
			}
		}
		c.genesisDoc, err = c.genesis.Export()
		if err != nil {
			return err
		}

		if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory for config: %w", err)
		}

		if err := c.genesisDoc.SaveAs(filepath.Join(configDir, "genesis.json")); err != nil {
			return fmt.Errorf("failed to save genesis: %w", err)
		}
	} else {
		// load the existing genesis
		c.logger.Printf("loading existing genesis from %s", configDir)
		c.genesisDoc, err = types.GenesisDocFromFile(filepath.Join(configDir, "genesis.json"))
		if err != nil {
			return err
		}
		for name := range c.services {
			dir := filepath.Join(c.rootDir, name)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				return fmt.Errorf("new service %s added has not been setup. Please clear ~/.apollo directory and restart", dir)
			}
		}
	}

	c.setup = true
	c.logger.Printf("services setup successfully at %s", c.rootDir)
	return nil
}

func (c *Conductor) StartService(ctx context.Context, name string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.startService(ctx, name)
}

func (c *Conductor) startService(ctx context.Context, name string) error {
	c.logger.Printf("starting up service %s", name)
	service, exists := c.services[name]
	if !exists {
		return fmt.Errorf("service %s does not exist", name)
	}
	if !c.setup {
		return fmt.Errorf("Conductor has not setup all services. Call `Setup` first")
	}

	requiredEndpoints := service.EndpointsNeeded()
	for _, endpoint := range requiredEndpoints {
		if _, ok := c.activeEndpoints[endpoint]; !ok {
			return fmt.Errorf("required endpoint '%s' for service '%s' is not active", endpoint, name)
		}
	}

	// supress output from services
	rescueStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	dir := filepath.Join(c.rootDir, name)
	activeEndpoints, err := service.Start(ctx, dir, c.genesisDoc, c.activeEndpoints)

	// restore stdout before error check
	w.Close()
	os.Stdout = rescueStdout

	if err != nil {
		return fmt.Errorf("failed to start service %s: %w", name, err)
	}

	// Update active endpoints after successful service start
	for key, value := range activeEndpoints {
		c.activeEndpoints[key] = value
	}
	c.activeServices[name] = service
	c.startOrder = append(c.startOrder, name)
	c.logger.Printf("service %s started successfully on endpoints: %v", name, activeEndpoints)
	return nil
}

func (c *Conductor) StopService(ctx context.Context, name string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.stopService(ctx, name)
}

func (c *Conductor) stopService(ctx context.Context, name string) error {
	c.logger.Printf("stopping service %s", name)

	// Check if the service exists and is active
	service, exists := c.activeServices[name]
	if !exists {
		return fmt.Errorf("service %s is not active or does not exist", name)
	}

	endpointsProvided := service.EndpointsProvided()

	// Check if any active service requires the endpoints provided by this service
	for _, activeService := range c.activeServices {
		if activeService.Name() == name {
			continue // Skip the service being stopped
		}
		for _, requiredEndpoint := range activeService.EndpointsNeeded() {
			for _, providedEndpoint := range endpointsProvided {
				if requiredEndpoint == providedEndpoint {
					return fmt.Errorf("cannot stop service '%s' as it provides required endpoint '%s' for active service '%s'", name, requiredEndpoint, activeService.Name())
				}
			}
		}
	}

	// Stop the service
	if err := service.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop service %s: %w", name, err)
	}

	// Update active services and endpoints
	delete(c.activeServices, name)
	for _, endpoint := range service.EndpointsProvided() {
		delete(c.activeEndpoints, endpoint)
	}

	c.logger.Printf("service %s stopped successfully", name)

	return nil
}

func (c *Conductor) Stop(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	for i := len(c.startOrder) - 1; i >= 0; i-- {
		if !c.isServiceRunning(c.startOrder[i]) {
			continue
		}
		if err := c.stopService(ctx, c.startOrder[i]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Conductor) Cleanup() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if len(c.activeServices) > 0 {
		return fmt.Errorf("cannot cleanup Conductor with active services")
	}

	c.logger.Printf("cleaning up all services at %s", c.rootDir)
	return os.RemoveAll(c.rootDir)
}

func (c *Conductor) IsServiceRunning(name string) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.isServiceRunning(name)
}

func (c *Conductor) isServiceRunning(name string) bool {
	_, exists := c.activeServices[name]
	return exists
}

func (c *Conductor) ServiceStatus() map[string]Status {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.serviceStatus()
}

func (c *Conductor) serviceStatus() map[string]Status {
	serviceStatus := make(map[string]Status)
	for name, service := range c.services {
		status := Status{
			RequiredEndpoints: service.EndpointsNeeded(),
			ProvidesEndpoints: make(map[string]string),
		}
		if _, ok := c.activeServices[name]; ok {
			status.Running = true
		}
		for _, providedEndpoint := range service.EndpointsProvided() {
			endpoint, ok := c.activeEndpoints[providedEndpoint]
			if !ok {
				continue
			}
			status.ProvidesEndpoints[providedEndpoint] = endpoint
		}
		serviceStatus[name] = status
	}
	return serviceStatus
}

// Serve starts the web server for the conductor, visualising the current
// running services and providing a GUI for basic control of all services.
func (c *Conductor) Serve(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if !c.setup {
		return fmt.Errorf("Conductor has not setup the services. Call `Setup` first")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	mux := http.NewServeMux()

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := c.serviceStatus()
		statusJSON, err := json.Marshal(status)
		if err != nil {
			http.Error(w, "Failed to marshal status", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(statusJSON); err != nil {
			c.logger.Printf("failed to write status: %s", err.Error())
		}
		c.logger.Printf("served status response")
	})

	mux.HandleFunc("/start/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 3 {
			http.Error(w, "Service name is required in the URL path. For example /start/consensus-node", http.StatusBadRequest)
			c.logger.Printf("received bad request to start service")
			return
		}
		serviceName := pathParts[2]
		if err := c.startService(ctx, serviceName); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			c.logger.Printf("failed to start service %s: %s", serviceName, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/stop/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 3 {
			http.Error(w, "Service name is required in the URL path. For example /stop/consensus-node", http.StatusBadRequest)
			c.logger.Printf("received bad request to stop service")
			return
		}
		serviceName := pathParts[2]
		if err := c.stopService(ctx, serviceName); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			c.logger.Printf("failed to stop service %s: %s", serviceName, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// serve front end as a static directory
	fileSystem, err := fs.Sub(web, "web")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(fileSystem)))

	server := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		if err := server.Shutdown(context.Background()); err != nil {
			c.logger.Printf("failed to shutdown server: %s", err.Error())
		}
		c.logger.Printf("control panel server shutdown successfully")
	}()

	c.logger.Printf("starting service control panel on %s", server.Addr)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

type Status struct {
	Running           bool      `json:"running"`
	ProvidesEndpoints Endpoints `json:"provides_endpoints"`
	RequiredEndpoints []string  `json:"required_endpoints"`
}
