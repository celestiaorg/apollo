package apollo

import (
	"context"
	"embed"
	"encoding/json"
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
	logger          *log.Logger
}

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

	m := &Conductor{
		services:        serviceMap,
		activeEndpoints: make(map[string]string),
		activeServices:  make(map[string]Service),
		startOrder:      make([]string, 0),
		genesis:         genesis.WithChainID(string(p2p.Private)),
		rootDir:         dir,
		logger:          log.New(os.Stdout, "", log.LstdFlags),
	}
	err := m.CheckEndpoints()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Conductor) CheckEndpoints() error {
	endpointMap := make(map[string]bool)
	for _, service := range m.services {
		for _, endpoint := range service.EndpointsProvided() {
			endpointMap[endpoint] = true
		}
	}

	for _, service := range m.services {
		for _, requiredEndpoint := range service.EndpointsNeeded() {
			if !endpointMap[requiredEndpoint] {
				return fmt.Errorf("required endpoint '%s' for service '%s' is not provided by any service", requiredEndpoint, service.Name())
			}
		}
	}

	return nil
}

func (m *Conductor) Setup(ctx context.Context) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.logger.Printf("setting up services...")

	configDir := filepath.Join(m.rootDir, "config")
	var genesis *types.GenesisDoc
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		pendingGenesis, err := m.genesis.Export()
		if err != nil {
			return err
		}
		for name, service := range m.services {
			dir := filepath.Join(m.rootDir, name)
			if err := os.MkdirAll(dir, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory for service %s: %w", name, err)
			}
			modifier, err := service.Setup(ctx, dir, pendingGenesis)
			if err != nil {
				return fmt.Errorf("failed to setup service %s: %w", name, err)
			}
			if modifier != nil {
				m.genesis = m.genesis.WithModifiers(modifier)
			}
		}
		genesis, err = m.genesis.Export()
		if err != nil {
			return err
		}

		if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory for config: %w", err)
		}

		if err := genesis.SaveAs(filepath.Join(configDir, "genesis.json")); err != nil {
			return fmt.Errorf("failed to save genesis: %w", err)
		}
	} else {
		// load the existing genesis
		m.logger.Printf("loading existing genesis from %s", configDir)
		genesis, err = types.GenesisDocFromFile(filepath.Join(configDir, "genesis.json"))
		if err != nil {
			return err
		}
	}

	for name, service := range m.services {
		dir := filepath.Join(m.rootDir, name)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("new service %s added has not been setup. Please clear ~/.apollo directory and restart", dir)
		}
		if err := service.Init(ctx, genesis); err != nil {
			return fmt.Errorf("failed to init service %s: %w", name, err)
		}
	}
	m.setup = true
	m.logger.Printf("services setup successfully at %s", m.rootDir)
	return nil
}

func (m *Conductor) StartService(ctx context.Context, name string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.startService(ctx, name)
}

func (m *Conductor) startService(ctx context.Context, name string) error {
	m.logger.Printf("starting up service %s", name)
	service, exists := m.services[name]
	if !exists {
		return fmt.Errorf("service %s does not exist", name)
	}
	if !m.setup {
		return fmt.Errorf("Conductor has not setup all services. Call `Setup` first")
	}

	requiredEndpoints := service.EndpointsNeeded()
	for _, endpoint := range requiredEndpoints {
		if _, ok := m.activeEndpoints[endpoint]; !ok {
			return fmt.Errorf("required endpoint '%s' for service '%s' is not active", endpoint, name)
		}
	}

	dir := filepath.Join(m.rootDir, name)
	activeEndpoints, err := service.Start(ctx, dir, m.activeEndpoints)
	if err != nil {
		return fmt.Errorf("failed to start service %s: %w", name, err)
	}

	// Update active endpoints after successful service start
	for key, value := range activeEndpoints {
		m.activeEndpoints[key] = value
	}
	m.activeServices[name] = service
	m.startOrder = append(m.startOrder, name)
	m.logger.Printf("service %s started successfully on endpoints: %v", name, activeEndpoints)
	return nil
}

func (m *Conductor) StopService(ctx context.Context, name string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.stopService(ctx, name)
}

func (m *Conductor) stopService(ctx context.Context, name string) error {
	m.logger.Printf("stopping service %s", name)

	// Check if the service exists and is active
	service, exists := m.activeServices[name]
	if !exists {
		return fmt.Errorf("service %s is not active or does not exist", name)
	}

	endpointsProvided := service.EndpointsProvided()

	// Check if any active service requires the endpoints provided by this service
	for _, activeService := range m.activeServices {
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
	delete(m.activeServices, name)
	for _, endpoint := range service.EndpointsProvided() {
		delete(m.activeEndpoints, endpoint)
	}

	m.logger.Printf("service %s stopped successfully", name)

	return nil
}

func (m *Conductor) Stop(ctx context.Context) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	for i := len(m.startOrder) - 1; i >= 0; i-- {
		if !m.isServiceRunning(m.startOrder[i]) {
			continue
		}
		if err := m.stopService(ctx, m.startOrder[i]); err != nil {
			return err
		}
	}
	return nil
}

func (m *Conductor) Cleanup() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if len(m.activeServices) > 0 {
		return fmt.Errorf("cannot cleanup Conductor with active services")
	}

	m.logger.Printf("cleaning up all services at %s", m.rootDir)
	return os.RemoveAll(m.rootDir)
}

func (m *Conductor) IsServiceRunning(name string) bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.isServiceRunning(name)
}

func (m *Conductor) isServiceRunning(name string) bool {
	_, exists := m.activeServices[name]
	return exists
}

func (m *Conductor) ServiceStatus() map[string]Status {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.serviceStatus()
}

func (m *Conductor) serviceStatus() map[string]Status {
	serviceStatus := make(map[string]Status)
	for name, service := range m.services {
		status := Status{
			RequiredEndpoints: service.EndpointsNeeded(),
			ProvidesEndpoints: make(map[string]string),
		}
		if _, ok := m.activeServices[name]; ok {
			status.Running = true
		}
		for _, providedEndpoint := range service.EndpointsProvided() {
			endpoint, ok := m.activeEndpoints[providedEndpoint]
			if !ok {
				continue
			}
			status.ProvidesEndpoints[providedEndpoint] = endpoint
		}
		serviceStatus[name] = status
	}
	return serviceStatus
}

func (m *Conductor) Serve(ctx context.Context) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if !m.setup {
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
		status := m.serviceStatus()
		statusJSON, err := json.Marshal(status)
		if err != nil {
			http.Error(w, "Failed to marshal status", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(statusJSON); err != nil {
			m.logger.Printf("failed to write status: %s", err.Error())
		}
		m.logger.Printf("served status response")
	})

	mux.HandleFunc("/start/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 3 {
			http.Error(w, "Service name is required in the URL path. For example /start/consensus-node", http.StatusBadRequest)
			m.logger.Printf("received bad request to start service")
			return
		}
		serviceName := pathParts[2]
		if err := m.startService(ctx, serviceName); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			m.logger.Printf("failed to start service %s: %s", serviceName, err.Error())
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
			m.logger.Printf("received bad request to stop service")
			return
		}
		serviceName := pathParts[2]
		if err := m.stopService(ctx, serviceName); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			m.logger.Printf("failed to stop service %s: %s", serviceName, err.Error())
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
			m.logger.Printf("failed to shutdown server: %s", err.Error())
		}
		m.logger.Printf("control panel server shutdown successfully")
	}()

	m.logger.Printf("starting service control panel on %s", server.Addr)
	return server.ListenAndServe()
}

type Status struct {
	Running           bool      `json:"running"`
	ProvidesEndpoints Endpoints `json:"provides_endpoints"`
	RequiredEndpoints []string  `json:"required_endpoints"`
}
