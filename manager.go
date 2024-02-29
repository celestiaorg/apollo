package apollo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sync"
)

type Manager struct {
	lock            sync.Mutex
	services        map[string]Service
	activeEndpoints Endpoints
	activeServices  map[string]Service
	rootDir         string
}

func New(dir string, services ...Service) (*Manager, error) {
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

	m := &Manager{
		services:        serviceMap,
		activeEndpoints: make(map[string]string),
		activeServices:  make(map[string]Service),
		rootDir:         dir,
	}
	return m, m.CheckEndpoints()
}

func (m *Manager) CheckEndpoints() error {
	endpointMap := make(map[string]bool)
	for _, service := range m.services {
		for _, endpoint := range service.Endpoints() {
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

func (m *Manager) StartService(ctx context.Context, name string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	service, exists := m.services[name]
	if !exists {
		return fmt.Errorf("service %s does not exist", name)
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

	return nil
}

func (m *Manager) StopService(ctx context.Context, name string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Check if the service exists and is active
	service, exists := m.activeServices[name]
	if !exists {
		return fmt.Errorf("service %s is not active or does not exist", name)
	}

	endpointsProvided := service.Endpoints()

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
	for _, endpoint := range service.Endpoints() {
		delete(m.activeEndpoints, endpoint)
	}

	return nil
}

func (m *Manager) IsServiceRunning(name string) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, exists := m.activeServices[name]
	return exists
}

func (m *Manager) ServiceStatus() map[string]Status {
	m.lock.Lock()
	defer m.lock.Unlock()

	serviceStatus := make(map[string]Status)
	for name, service := range m.services {
		status := Status{
			RequiredEndpoints: service.EndpointsNeeded(),
			ProvidesEndpoints: make(map[string]string),
		}
		if _, ok := m.activeServices[name]; ok {
			status.Running = true
		}
		for _, providedEndpoint := range service.Endpoints() {
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

func (m *Manager) Serve(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := m.ServiceStatus()
		statusJSON, err := json.Marshal(status)
		if err != nil {
			http.Error(w, "Failed to marshal status", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(statusJSON); err != nil {
			log.Printf("failed to write status: %s", err.Error())
		}
	})

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if err := m.StartService(ctx, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if err := m.StopService(ctx, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	return server.ListenAndServe()
}

type Status struct {
	Running           bool      `json:"running"`
	ProvidesEndpoints Endpoints `json:"provides_endpoints"`
	RequiredEndpoints []string  `json:"required_endpoints"`
}
