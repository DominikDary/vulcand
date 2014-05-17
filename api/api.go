package api

import (
	"fmt"
	"github.com/gorilla/mux"
	api "github.com/mailgun/gotools-api"
	log "github.com/mailgun/gotools-log"
	//	"github.com/mailgun/vulcan/netutils"
	. "github.com/mailgun/vulcand/backend"
	. "github.com/mailgun/vulcand/connwatch"
	//	"github.com/mailgun/vulcand/plugin"
	"net/http"
	//	"net/url"
	//	"strconv"
	//	"time"
)

type ProxyController struct {
	backend     Backend
	connWatcher *ConnectionWatcher
	statsGetter StatsGetter
}

func InitProxyController(backend Backend, statsGetter StatsGetter, connWatcher *ConnectionWatcher, router *mux.Router) {
	controller := &ProxyController{backend: backend, statsGetter: statsGetter, connWatcher: connWatcher}

	router.HandleFunc("/v1/status", api.MakeRawHandler(controller.GetStatus)).Methods("GET")
	router.HandleFunc("/v1/hosts", api.MakeRawHandler(controller.GetHosts)).Methods("GET")
	router.HandleFunc("/v1/hosts", api.MakeRawHandler(controller.AddHost)).Methods("POST")
	router.HandleFunc("/v1/hosts/{hostname}", api.MakeRawHandler(controller.DeleteHost)).Methods("DELETE")

	/*
		router.HandleFunc("/v1/hosts/{hostname}/locations", api.MakeHandler(controller.AddLocation)).Methods("POST")
		router.HandleFunc("/v1/hosts/{hostname}/locations/{id}", api.MakeHandler(controller.DeleteLocation)).Methods("DELETE")
		router.HandleFunc("/v1/hosts/{hostname}/locations/{id}", api.MakeHandler(controller.UpdateLocation)).Methods("PUT")

		router.HandleFunc("/v1/upstreams", api.MakeHandler(controller.AddUpstream)).Methods("POST")
		router.HandleFunc("/v1/upstreams", api.MakeHandler(controller.GetUpstreams)).Methods("GET")
		router.HandleFunc("/v1/upstreams/{id}", api.MakeHandler(controller.DeleteUpstream)).Methods("DELETE")
		router.HandleFunc("/v1/upstreams/{id}", api.MakeHandler(controller.GetUpstream)).Methods("GET")
		router.HandleFunc("/v1/upstreams/{id}/drain", api.MakeHandler(controller.DrainUpstreamConnections)).Methods("GET")

		router.HandleFunc("/v1/upstreams/{upstream}/endpoints", api.MakeHandler(controller.AddEndpoint)).Methods("POST")
		router.HandleFunc("/v1/upstreams/{upstream}/endpoints/{endpoint}", api.MakeHandler(controller.DeleteEndpoint)).Methods("DELETE")

		// Register controllers for middlewares
		if backend.GetRegistry() != nil {
			for _, middlewareSpec := range backend.GetRegistry().GetSpecs() {
				controller.generateHandlers(router, middlewareSpec)
			}
		}
	*/
}

func (c *ProxyController) GetStatus(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	return api.Response{
		"Status": "ok",
	}, nil
}

func (c *ProxyController) GetHosts(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	hosts, err := c.backend.GetHosts()

	// This is to display the realtime stats, looks ugly.
	for _, h := range hosts {
		for _, l := range h.Locations {
			for _, e := range l.Upstream.Endpoints {
				e.Stats = c.statsGetter.GetStats(h.Name, l.Id, e.Id)
			}
		}
	}
	return api.Response{
		"Hosts": hosts,
	}, err
}

func (c *ProxyController) AddHost(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	host, err := HostFromJson(body)
	if err != nil {
		return nil, newError(err)
	}
	log.Infof("Add %s", host)
	return formatResult(c.backend.AddHost(host))
}

func (c *ProxyController) DeleteHost(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	hostname := params["hostname"]
	log.Infof("Delete host: %s", hostname)
	if err := c.backend.DeleteHost(hostname); err != nil {
		return nil, newError(err)
	}
	return api.Response{"message": fmt.Sprintf("Host '%s' deleted", hostname)}, nil
}

func (c *ProxyController) AddUpstream(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	upstream, err := UpstreamFromJson(body)
	if err != nil {
		return nil, api.GenericAPIError{Reason: fmt.Sprintf("%s", err)}
	}
	log.Infof("Add Upstream: %s", upstream)
	return formatResult(c.backend.AddUpstream(upstream))
}

func (c *ProxyController) DeleteUpstream(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	upstreamId := params["upstream"]
	log.Infof("Delete Upstream(id=%s)", upstreamId)
	if err := c.backend.DeleteUpstream(upstreamId); err != nil {
		return nil, newError(err)
	}
	return api.Response{"message": "Upstream deleted"}, nil
}

/*

func (c *ProxyController) AddLocation(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error) {
	hostname := params["hostname"]

	id, err := api.GetStringField(r, "id")
	if err != nil {
		return nil, err
	}

	path, err := api.GetStringField(r, "path")
	if err != nil {
		return nil, err
	}
	upstream, err := api.GetStringField(r, "upstream")
	if err != nil {
		return nil, err
	}

	log.Infof("Add Location: %s %s", hostname, path)
	if err := c.backend.AddLocation(id, hostname, path, upstream); err != nil {
		return nil, api.GenericAPIError{Reason: fmt.Sprintf("%s", err)}
	}

	return api.Response{"message": "Location added"}, nil
}

func (c *ProxyController) UpdateLocation(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error) {
	hostname := params["hostname"]
	locationId := params["id"]

	upstream, err := api.GetStringField(r, "upstream")
	if err != nil {
		return nil, err
	}

	log.Infof("Update Location: %s %s set upstream", hostname, locationId, upstream)
	if err := c.backend.UpdateLocationUpstream(hostname, locationId, upstream); err != nil {
		return nil, api.GenericAPIError{Reason: fmt.Sprintf("%s", err)}
	}

	return api.Response{"message": "Location upstream updated"}, nil
}

func (c *ProxyController) DeleteLocation(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error) {
	log.Infof("Delete Location(id=%s) from Host(name=%s)", params["id"], params["hostname"])
	if err := c.backend.DeleteLocation(params["hostname"], params["id"]); err != nil {
		return nil, api.GenericAPIError{Reason: fmt.Sprintf("%s", err)}
	}
	return api.Response{"message": "Location deleted"}, nil
}

func (c *ProxyController) GetUpstreams(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error) {
	upstreams, err := c.backend.GetUpstreams()
	return api.Response{
		"Upstreams": upstreams,
	}, err
}

func (c *ProxyController) GetUpstream(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error) {
	upstream, err := c.backend.GetUpstream(params["id"])
	if err != nil {
		return nil, err
	}
	if upstream == nil {
		return nil, api.NotFoundError{Description: "Upstream not found"}
	}
	return api.Response{
		"Upstream": upstream,
	}, err
}

func (c *ProxyController) DrainUpstreamConnections(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error) {
	upstream, err := c.backend.GetUpstream(params["id"])
	if err != nil {
		return nil, err
	}
	if upstream == nil {
		return nil, api.NotFoundError{Description: "Upstream not found"}
	}

	timeoutS, err := api.GetStringField(r, "timeout")
	if err != nil {
		return nil, err
	}

	timeout, err := strconv.Atoi(timeoutS)
	if err != nil {
		return nil, err
	}

	endpoints := make([]*url.URL, len(upstream.Endpoints))
	for i, e := range upstream.Endpoints {
		u, err := netutils.ParseUrl(e.Url)
		if err != nil {
			return nil, err
		}
		endpoints[i] = u
	}

	connections, err := c.connWatcher.DrainConnections(time.Duration(timeout)*time.Second, endpoints...)
	if err != nil {
		return nil, err
	}
	return api.Response{
		"Connections": connections,
	}, err
}



func (c *ProxyController) AddEndpoint(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error) {
	url, err := api.GetStringField(r, "url")
	if err != nil {
		return nil, err
	}
	id, err := api.GetStringField(r, "id")
	if err != nil {
		return nil, err
	}

	upstreamId := params["upstream"]
	log.Infof("Add Endpoint %s to %s", url, upstreamId)

	if err := c.backend.AddEndpoint(upstreamId, id, url); err != nil {
		return nil, api.GenericAPIError{Reason: fmt.Sprintf("%s", err)}
	}
	return api.Response{"message": "Endpoint added"}, nil
}

func (c *ProxyController) DeleteEndpoint(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error) {
	upstreamId := params["upstream"]
	id := params["endpoint"]

	log.Infof("Delete Endpoint(url=%s) from Upstream(id=%s)", id, upstreamId)
	if err := c.backend.DeleteEndpoint(upstreamId, id); err != nil {
		return nil, api.GenericAPIError{Reason: fmt.Sprintf("%s", err)}
	}
	return api.Response{"message": "Endpoint deleted"}, nil
}

type Handler func(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error)

func (c *ProxyController) generateHandlers(router *mux.Router, spec *plugin.MiddlewareSpec) error {
	router.HandleFunc(
		fmt.Sprintf("/v1/hosts/{hostname}/locations/{location}/middlewares/%s", spec.Type),
		api.MakeHandler(c.generateAddMiddleware(spec))).Methods("POST")
}

func (c *ProxyController) generateAddMiddleware(spec *plugin.MiddlewareSpec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)

	}
}
*/

func newError(err error) error {
	return api.GenericAPIError{Reason: fmt.Sprintf("%s", err)}
}

func formatResult(in interface{}, err error) (interface{}, error) {
	if err != nil {
		return nil, newError(err)
	}
	return in, nil
}
