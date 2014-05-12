// Defines interfaces and structures controlling the proxy configuration and changes
package backend

import (
	"fmt"

	"github.com/mailgun/vulcan/netutils"
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

	AddLocationMiddleware(hostname, locationId string, rateLimit *RateLimit) (*RateLimit, error)
	//GetLocationMiddleware(hostname, locationId string,
	DeleteLocationMiddleware(hostname, locationId, id string) error
	UpdateLocationMiddleware(hostname, locationId string, id string, rateLimit *RateLimit) error

	AddLocationConnLimit(hostname, locationId, connLimit *ConnLimit) (*ConnLimit, error)
	DeleteLocationConnLimit(hostname, locationId, id string) error
	UpdateLocationConnLimit(hostname, locationId string, connLimit *ConnLimit) (*ConnLimit, error)

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
		"Location(hostname=%s, id=%s, path=%s, upstream=%s, connlimits=%s, ratelimits=%s)",
		l.Id, l.Path, l.Upstream, l.ConnLimits, l.RateLimits)
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
