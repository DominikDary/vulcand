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
	FromBytes ReaderFn
	// This is to provide CRUD for middlewares
	ApiHandlers []*ApiHandler
	// CLI command handlers for crud
	CliHandler cli.Command
}

type Middleware interface {
	// Unique type of the middleware
	GetType() string
	// Unique id of this middleware instance
	GetId() string
	// Returns serialized representation of the middleware
	ToBytes() []byte
	// Returns vulcan library compatible middleware
	NewInstance() middleware.Middleware
}

// Reader constructs the serialized middleware from it's serialized representation
// It's up to middleware to choose the serialization format
type ReaderFn func([]byte) (Middleware, error)

// Api handler defines how middleware is represented by vulcand API interface
// handler is plugged in into vulcand API handlers
type ApiHandler struct {
	Methods string // E.g. GET, PUT
	Path    string // E.g. limits/rates/{id}
	Handler http.HandlerFunc
}

// Registry contains currently registered middlewares and used to support pluggable middlewares
// across all modules of the vulcand
type Registry struct {
	specs []*MiddlewareSpec
}

func NewRegistry() *Registry {
	return &Registry{
		specs: []*Spec{},
	}
}

func (r *Registry) AddSpec(s *Spec) error {
	if s == nil {
		return fmt.Errorf("Spec can not be nil")
	}
	if r.GetSpec(s.Type) != nil {
		return fmt.Errorf("Middleware of type %s already registered")
	}
	r.specs = append(r.specs, s)
	return nil
}

func (r *Registry) GetSpec(middlewareType string) *Spec {
	for _, s := range r.specs {
		if s.Type == middlewareType {
			return s
		}
	}
	return nil
}

func (r *Registry) GetSpecs(middlewareType string) []*Spec {
	return r.specs
}
