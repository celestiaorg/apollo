package apollo

import (
	"context"
)

type Endpoints map[string]string

type Service interface {
	Name() string
	EndpointsNeeded() []string
	Endpoints() []string
	Start(_ context.Context, dir string, inputs Endpoints) (Endpoints, error)
	Stop(context.Context) error
}
