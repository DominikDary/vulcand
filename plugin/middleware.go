package plugin

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/mailgun/vulcan/middleware"
	"net/http"
)

// Middleware specification, used to construct new middlewares and plug them into CLI API and backends
type MiddlewareSpec struct {
	Type string
	// Reader function that returns a middleware from a serialized format
	FromJson JsonReader
	// Handler function that constructs a middleware from HTTP request
	FromRequest RequestReader
	// Flags for CLI tool to generate interface
	CliFlags []cli.Flag
	// Function that construtcs a middleware from CLI parameters
	FromCli CliReader
}

type Middleware interface {
	// Unique type of the middleware
	GetType() string
	// Returns JSON serialized representation of the middleware
	ToJson() ([]byte, error)
	// Returns vulcan library compatible middleware
	NewInstance() (middleware.Middleware, error)
}

// Reader constructs the middleware from it's serialized JSON representation
// It's up to middleware to choose the serialization format
type JsonReader func([]byte) (Middleware, error)

// Handler constructs the middleware from http request
type RequestReader func(id string, r *http.Request) (Middleware, error)

// Reader constructs the middleware from the CLI interface
type CliReader func(c *cli.Context) (Middleware, error)

// Reads middlewares from the specification given the type
type MiddlewareReader func(middlewareType string, data []byte) (Middleware, error)

// Registry contains currently registered middlewares and used to support pluggable middlewares
// across all modules of the vulcand
type Registry struct {
	specs []*MiddlewareSpec
}

func NewRegistry() *Registry {
	return &Registry{
		specs: []*MiddlewareSpec{},
	}
}

func (r *Registry) AddSpec(s *MiddlewareSpec) error {
	if s == nil {
		return fmt.Errorf("Spec can not be nil")
	}
	if r.GetSpec(s.Type) != nil {
		return fmt.Errorf("Middleware of type %s already registered")
	}
	r.specs = append(r.specs, s)
	return nil
}

func (r *Registry) GetSpec(middlewareType string) *MiddlewareSpec {
	for _, s := range r.specs {
		if s.Type == middlewareType {
			return s
		}
	}
	return nil
}

func (r *Registry) GetSpecs() []*MiddlewareSpec {
	return r.specs
}

func (r *Registry) ReadMiddleware(mType string, data []byte) (Middleware, error) {
	spec := r.GetSpec(mType)
	if spec == nil {
		return nil, fmt.Errorf("Middleware type %s is not supported", mType)
	}
	return spec.FromJson(data)
}
