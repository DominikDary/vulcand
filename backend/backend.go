// Defines interfaces and structures controlling the proxy configuration and changes
package backend

import (
	"encoding/json"
	"fmt"
	"github.com/mailgun/vulcan/netutils"
	. "github.com/mailgun/vulcand/plugin"
	"regexp"
)

type Backend interface {
	GetHosts() ([]*Host, error)
	AddHost(*Host) (*Host, error)
	GetHost(name string) *Host
	DeleteHost(name string) error

	AddLocation(*Location) (*Location, error)
	GetLocation(hostname, id string) *Location
	DeleteLocation(hostname, id string) error
	UpdateLocationUpstream(hostname, id string, upstream string) error

	AddLocationMiddleware(hostname, locationId string, m Middleware) (Middleware, error)
	GetLocationMiddleware(hostname, locationId string, mType string) (Middleware, error)
	DeleteLocationMiddleware(hostname, locationId, mType, id string) error
	UpdateLocationMiddleware(hostname, locationId string, m Middleware) error

	GetUpstreams() ([]*Upstream, error)
	AddUpstream(*Upstream) (*Upstream, error)
	GetUpstream(id string) *Upstream
	DeleteUpstream(id string) error

	AddEndpoint(*Endpoint) (*Endpoint, error)
	GetEndpoint(upstreamId, id string) *Endpoint
	DeleteEndpoint(upstreamId, id string) error

	// Watch changes is an entry point for getting the configuration changes
	// as well as the initial configuration. It should be have in the following way:
	// * This should be a blocking function generating events from change.go to the changes channel
	// If the initalSetup is true, it should read the existing configuration and generate the events to the channel
	// just as someone was creating the elements one by one.
	WatchChanges(changes chan interface{}, initialSetup bool) error
}

// Provides realtime stats about endpoint specific to a particular location.
type StatsGetter interface {
	GetStats(hostname string, locationId string, endpointId string) *EndpointStats
}

// Incoming requests are matched by their hostname first.
// Hostname is defined by incoming 'Host' header.
// E.g. curl http://example.com/alice will be matched by the host example.com first.
type Host struct {
	Name       string
	BackendKey string `json:",omitempty"`
	Locations  []*Location
}

func HostFromJson(in []byte) (*Host, error) {
	var h *Host
	err := json.Unmarshal(in, &h)
	if err != nil {
		return nil, err
	}
	return NewHost(h.Name)
}

func NewHost(name string) (*Host, error) {
	if name == "" {
		return nil, fmt.Errorf("Hostname can not be empty")
	}
	return &Host{
		Name:      name,
		Locations: []*Location{},
	}, nil
}

func (h *Host) String() string {
	return fmt.Sprintf("Host(name=%s, backendKey=%s, locations=%s)", h.Name, h.BackendKey, h.Locations)
}

func (h *Host) GetId() string {
	return h.Name
}

// Hosts contain one or several locations. Each location defines a path - simply a regular expression that will be matched against request's url.
// Location contains link to an upstream and vulcand will use the endpoints from this upstream to serve the request.
// E.g. location loc1 will serve the request curl http://example.com/alice because it matches the path /alice:
type Location struct {
	Hostname    string
	BackendKey  string `json:",omitempty"`
	Path        string
	Id          string
	Upstream    *Upstream
	Middlewares []interface{}
}

func LocationFromJson(in []byte) (*Location, error) {
	var l *Location
	err := json.Unmarshal(in, &l)
	if err != nil {
		return nil, err
	}
	return NewLocation(l.Hostname, l.Id, l.Path, l.Upstream.Id)
}

func NewLocation(hostname, id, path, upstreamId string) (*Location, error) {
	if len(path) == 0 || len(hostname) == 0 || len(upstreamId) == 0 {
		return nil, fmt.Errorf("Supply valid hostname, path and upstream id")
	}

	// Make sure location path is a valid regular expression
	if _, err := regexp.Compile(path); err != nil {
		return nil, fmt.Errorf("Path should be a valid Golang regular expression")
	}

	return &Location{
		Hostname:    hostname,
		Path:        path,
		Id:          id,
		Upstream:    &Upstream{Id: upstreamId, Endpoints: []*Endpoint{}},
		Middlewares: []interface{}{},
	}, nil
}

func (l *Location) String() string {
	return fmt.Sprintf(
		"Location(hostname=%s, id=%s, path=%s, upstream=%s, middlewares=%s)",
		l.Id, l.Path, l.Upstream, l.Middlewares)
}

func (l *Location) GetId() string {
	return l.Id
}

// Upstream is a collection of endpoints. Each location is assigned an upstream. Changing assigned upstream
// of the location gracefully redirects the traffic to the new endpoints of the upstream.
type Upstream struct {
	Id         string
	BackendKey string `json:",omitempty"`
	Endpoints  []*Endpoint
}

func NewUpstream(id string) (*Upstream, error) {
	return &Upstream{
		Id:        id,
		Endpoints: []*Endpoint{},
	}, nil
}

func (u *Upstream) String() string {
	return fmt.Sprintf("Upstream(id=%s, key=%s, endpoints=%s)", u.Id, u.BackendKey, u.Endpoints)
}

func (u *Upstream) GetId() string {
	return u.Id
}

func UpstreamFromJson(in []byte) (*Upstream, error) {
	var u *Upstream
	err := json.Unmarshal(in, &u)
	if err != nil {
		return nil, err
	}
	return NewUpstream(u.Id)
}

// Endpoint is a final destination of the request
type Endpoint struct {
	Id         string
	Url        string
	BackendKey string `json:",omitempty"`
	UpstreamId string
	Stats      *EndpointStats
}

func NewEndpoint(upstreamId, id, url string) (*Endpoint, error) {
	if upstreamId == "" {
		return nil, fmt.Errorf("Upstream id '%s' can not be empty")
	}
	if _, err := netutils.ParseUrl(url); err != nil {
		return nil, fmt.Errorf("Endpoint url '%s' is not valid", url)
	}
	return &Endpoint{
		UpstreamId: upstreamId,
		Id:         id,
		Url:        url,
	}, nil
}

func (e *Endpoint) String() string {
	return fmt.Sprintf("Endpoint(id=%s, key=%s, url=%s, stats=%s)", e.Id, e.BackendKey, e.Url, e.Stats)
}

func (e *Endpoint) GetId() string {
	return e.Id
}

func EndpointFromJson(in []byte) (*Endpoint, error) {
	var e *Endpoint
	err := json.Unmarshal(in, &e)
	if err != nil {
		return nil, err
	}
	return NewEndpoint(e.UpstreamId, e.Id, e.Url)
}

// Endpoint's realtime stats about endpoint
type EndpointStats struct {
	Successes     int64
	Failures      int64
	FailRate      float64
	PeriodSeconds int
}

func (e *EndpointStats) String() string {
	reqsSec := (e.Failures + e.Successes) / int64(e.PeriodSeconds)
	return fmt.Sprintf("%d requests/sec, %.2f failures/sec", reqsSec, e.FailRate)
}
