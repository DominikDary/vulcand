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
	FromBytes ByteReader

	// Handler function that constructs a middleware from HTTP request
	FromRequest RequestReader

	// CLI command handlers for crud
	CliHandler cli.Command
}

type Middleware interface {
	// Unique type of the middleware
	GetType() string
	// Unique id of this middleware instance
	GetId() string
	// Returns serialized representation of the middleware
	ToBytes() ([]byte, error)
	// Returns vulcan library compatible middleware
	NewInstance() (middleware.Middleware, error)
}

// Reader constructs the middleware from it's serialized representation
// It's up to middleware to choose the serialization format
type ByteReader func([]byte) (Middleware, error)

// Handler constructs the middleware from http request
type RequestReader func(r *http.Request, params map[string]string) (Middleware, error)

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

func (r *Registry) GetSpecs(middlewareType string) []*MiddlewareSpec {
	return r.specs
}
