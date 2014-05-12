// This file will be generated to include all customer specific middlewares
package registry

import (
	. "github.com/mailgun/vulcand/middleware"
	"github.com/mailgun/vulcand/middleware/limit/conn"
	"github.com/mailgun/vulcand/middleware/limit/rate"
)

func GetRegistry(middlewareType string) *Registry {
	r := NewRegistry()

	if err := r.AddSpec(conn.GetSpec()); err != nil {
		panic(err)
	}
	if err := r.AddSpec(rate.GetSpec()); err != nil {
		panic(err)
	}
	return r
}
