// Etcd implementation of the backend, where all vulcand properties are implemented as directories or keys.
// This backend watches the changes and generates events with sequence of changes.
package etcdbackend

import (
	"fmt"
	"github.com/mailgun/go-etcd/etcd"
	log "github.com/mailgun/gotools-log"
	"github.com/mailgun/vulcan/endpoint"
	. "github.com/mailgun/vulcand/backend"
	"github.com/mailgun/vulcand/plugin"
	. "github.com/mailgun/vulcand/plugin"
	"regexp"
	"strings"
)

type EtcdBackend struct {
	nodes       []string
	registry    *plugin.Registry
	etcdKey     string
	consistency string
	client      *etcd.Client
	cancelC     chan bool
	stopC       chan bool
}

func NewEtcdBackend(registry *plugin.Registry, nodes []string, etcdKey, consistency string) (*EtcdBackend, error) {
	client := etcd.NewClient(nodes)
	if err := client.SetConsistency(consistency); err != nil {
		return nil, err
	}
	b := &EtcdBackend{
		nodes:       nodes,
		registry:    registry,
		etcdKey:     etcdKey,
		consistency: consistency,
		client:      client,
		cancelC:     make(chan bool, 1),
		stopC:       make(chan bool, 1),
	}
	return b, nil
}

func (s *EtcdBackend) StopWatching() {
	s.cancelC <- true
	s.stopC <- true
}

func (s *EtcdBackend) GetHosts() ([]*Host, error) {
	return s.readHosts(true)
}

func (s *EtcdBackend) AddHost(h *Host) (*Host, error) {
	if _, err := s.client.CreateDir(s.path("hosts", h.Name), 0); err != nil {
		return nil, objectError(err, h)
	}
	return h, nil
}

func (s *EtcdBackend) GetHost(hostname string) (*Host, error) {
	return s.readHost(hostname, false)
}

func (s *EtcdBackend) readHost(hostname string, deep bool) (*Host, error) {
	hostKey := s.path("hosts", hostname)
	_, err := s.client.Get(hostKey, false, false)
	if err != nil {
		return nil, objectError(err, &Host{Name: hostname})
	}
	host := &Host{
		Name:       hostname,
		BackendKey: hostKey,
		Locations:  []*Location{},
	}

	if !deep {
		return host, nil
	}

	for _, locationKey := range s.getDirs(hostKey, "locations") {
		location, err := s.GetLocation(hostname, suffix(locationKey))
		if err != nil {
			return nil, err
		}
		host.Locations = append(host.Locations, location)
	}
	return host, nil
}

func (s *EtcdBackend) DeleteHost(name string) error {
	_, err := s.client.Delete(s.path("hosts", name), true)
	return objectError(err, &Host{Name: name})
}

func (s *EtcdBackend) AddLocation(l *Location) (*Location, error) {
	if _, err := s.GetUpstream(l.Upstream.Id); err != nil {
		return nil, err
	}

	// Auto generate id if not set by user, very handy feature
	if l.Id == "" {
		response, err := s.client.AddChildDir(s.path("hosts", l.Hostname, "locations"), 0)
		if err != nil {
			return nil, objectError(err, l)
		}
		l.Id = suffix(response.Node.Key)
	} else {
		_, err := s.client.CreateDir(s.path("hosts", l.Hostname, "locations", l.Id), 0)
		if err != nil {
			return nil, objectError(err, l)
		}
	}
	locationKey := s.path("hosts", l.Hostname, "locations", l.Id)
	if _, err := s.client.Create(join(locationKey, "path"), l.Path, 0); err != nil {
		return nil, err
	}
	if _, err := s.client.Create(join(locationKey, "upstream"), l.Upstream.Id, 0); err != nil {
		return nil, err
	}
	return l, nil
}

func (s *EtcdBackend) GetLocation(hostname, locationId string) (*Location, error) {
	locationKey := s.path("hosts", hostname, "locations", locationId)
	_, err := s.client.Get(locationKey, false, false)
	if err != nil {
		return nil, objectError(err, &Location{Id: locationId})
	}
	path, ok := s.getVal(locationKey, "path")
	if !ok {
		return nil, fmt.Errorf("Missing location path: %s", locationKey)
	}
	upstreamKey, ok := s.getVal(locationKey, "upstream")
	if !ok {
		return nil, fmt.Errorf("Missing location upstream: %s", locationKey)
	}
	location := &Location{
		Hostname:    hostname,
		Id:          locationId,
		BackendKey:  locationKey,
		Path:        path,
		Middlewares: []Middleware{},
	}
	upstream, err := s.GetUpstream(upstreamKey)
	if err != nil {
		return nil, err
	}

	for _, spec := range s.registry.GetSpecs() {
		for _, cl := range s.getVals(locationKey, "middlewares", spec.Type) {
			m, err := s.GetLocationMiddleware(hostname, locationId, spec.Type, cl.Key)
			if err != nil {
				log.Errorf("Failed to read middleware %s(%s), error: %s", spec.Type, cl.Key, err)
			} else {
				location.Middlewares = append(location.Middlewares, m)
			}
		}
	}

	location.Upstream = upstream
	return location, nil
}

func (s *EtcdBackend) UpdateLocationUpstream(hostname, id, upstreamId string) error {
	log.Infof("Update Location(id=%s, hostname=%s) set upstream %s", id, hostname, upstreamId)

	// Make sure upstream exists
	if _, err := s.GetUpstream(upstreamId); err != nil {
		return err
	}

	location, err := s.GetLocation(hostname, id)
	if err != nil {
		return err
	}

	_, err = s.client.Set(join(location.BackendKey, "upstream"), upstreamId, 0)
	return err
}

func (s *EtcdBackend) DeleteLocation(hostname, id string) error {
	locationKey := s.path("hosts", hostname, "locations", id)
	_, err := s.client.Delete(locationKey, true)
	if err != nil {
		return objectError(err, &Location{Id: id})
	}
	return nil
}

func (s *EtcdBackend) AddUpstream(u *Upstream) (*Upstream, error) {
	if u.Id == "" {
		response, err := s.client.AddChildDir(s.path("upstreams"), 0)
		if err != nil {
			return nil, objectError(err, u)
		}
		u.Id = suffix(response.Node.Key)
	} else {
		_, err := s.client.CreateDir(s.path("upstreams", u.Id), 0)
		if err != nil {
			return nil, objectError(err, u)
		}
	}
	return u, nil
}

func (s *EtcdBackend) GetUpstream(upstreamId string) (*Upstream, error) {
	upstreamKey := s.path("upstreams", upstreamId)

	_, err := s.client.Get(upstreamKey, false, false)
	if err != nil {
		return nil, objectError(err, &Upstream{Id: upstreamId})
	}
	upstream := &Upstream{
		Id:         suffix(upstreamKey),
		BackendKey: upstreamKey,
		Endpoints:  []*Endpoint{},
	}

	endpointPairs := s.getVals(join(upstream.BackendKey, "endpoints"))
	for _, e := range endpointPairs {
		_, err := endpoint.ParseUrl(e.Val)
		if err != nil {
			fmt.Printf("Ignoring endpoint: failed to parse url: %s", e.Val)
			continue
		}
		e := &Endpoint{
			Url:        e.Val,
			BackendKey: e.Key,
			Id:         suffix(e.Key),
		}
		upstream.Endpoints = append(upstream.Endpoints, e)
	}
	return upstream, nil
}

func (s *EtcdBackend) GetUpstreams() ([]*Upstream, error) {
	upstreams := []*Upstream{}
	for _, upstreamKey := range s.getDirs(s.etcdKey, "upstreams") {
		upstream, err := s.GetUpstream(suffix(upstreamKey))
		if err != nil {
			return nil, err
		}
		upstreams = append(upstreams, upstream)
	}
	return upstreams, nil
}

func (s *EtcdBackend) DeleteUpstream(upstreamId string) error {
	locations, err := s.upstreamUsedBy(upstreamId)
	if err != nil {
		return err
	}
	if len(locations) != 0 {
		return fmt.Errorf("Can't delete upstream '%s', it's in use by %s", locations)
	}
	_, err = s.client.Delete(s.path("upstreams", upstreamId), true)
	return objectError(err, &Upstream{Id: upstreamId})
}

func (s *EtcdBackend) AddEndpoint(e *Endpoint) (*Endpoint, error) {
	if e.Id == "" {
		response, err := s.client.AddChild(s.path("upstreams", e.UpstreamId, "endpoints"), e.Url, 0)
		if err != nil {
			return nil, objectError(err, e)
		}
		e.Id = suffix(response.Node.Key)
	} else {
		if _, err := s.client.Create(s.path("upstreams", e.UpstreamId, "endpoints", e.Id), e.Url, 0); err != nil {
			return nil, objectError(err, e)
		}
	}
	return e, nil
}

func (s *EtcdBackend) GetEndpoint(upstreamId, id string) (*Endpoint, error) {
	if _, err := s.GetUpstream(upstreamId); err != nil {
		return nil, err
	}

	response, err := s.client.Get(s.path("upstreams", upstreamId, "endpoints", id), false, false)
	if err != nil {
		return nil, objectError(err, &Endpoint{Id: id})
	}

	return &Endpoint{
		Url:        response.Node.Value,
		BackendKey: response.Node.Key,
		Id:         suffix(response.Node.Key),
	}, nil
}

func (s *EtcdBackend) DeleteEndpoint(upstreamId, id string) error {
	if _, err := s.GetUpstream(upstreamId); err != nil {
		return err
	}
	if _, err := s.client.Delete(s.path("upstreams", upstreamId, "endpoints", id), true); err != nil {
		return objectError(err, &Endpoint{Id: id})
	}
	return nil
}

func (s *EtcdBackend) AddLocationMiddleware(hostname, locationId string, m Middleware) (Middleware, error) {
	// Make sure location actually exists
	if _, err := s.GetLocation(hostname, locationId); err != nil {
		return nil, err
	}
	bytes, err := m.ToJson()
	if err != nil {
		return nil, err
	}
	if m.GetId() == "" {
		response, err := s.client.AddChild(s.path("hosts", hostname, "locations", locationId, "middlewares", m.GetType()), string(bytes), 0)
		if err != nil {
			return nil, err
		}
		m.SetId(suffix(response.Node.Key))
	} else {
		if _, err := s.client.Create(s.path("hosts", hostname, "locations", locationId, "middlewares", m.GetType(), m.GetId()), string(bytes), 0); err != nil {
			return nil, objectError(err, m)
		}
	}
	return m, nil
}

func (s *EtcdBackend) GetLocationMiddleware(hostname, locationId, mType, id string) (Middleware, error) {
	spec := s.registry.GetSpec(mType)
	if spec == nil {
		return nil, fmt.Errorf("Middleware type %s is not registered", mType)
	}
	// Make sure location exists
	if _, err := s.GetLocation(hostname, locationId); err != nil {
		return nil, err
	}

	bytes, ok := s.getVal(s.path("hosts", hostname, "locations", locationId, "middlewares", mType))
	if !ok {
		return nil, fmt.Errorf("Middleware %s(%s) not found", mType, id)
	}
	m, err := spec.FromJson([]byte(bytes))
	if err != nil {
		return nil, fmt.Errorf("Failed to read Middleware %s(%s)", mType, id, err)
	}
	return m, nil
}

func (s *EtcdBackend) UpdateLocationMiddleware(hostname, locationId string, id string, m Middleware) error {
	if len(id) == 0 || len(hostname) == 0 || len(locationId) == 0 {
		return fmt.Errorf("Provide hostname, location and rate id to update")
	}
	// Make sure location actually exists
	if _, err := s.GetLocation(hostname, locationId); err != nil {
		return err
	}
	bytes, err := m.ToJson()
	if err != nil {
		return err
	}
	if _, err := s.client.Set(s.path("hosts", hostname, "locations", locationId, "limits", "rates", m.GetId()), string(bytes), 0); err != nil {
		return objectError(err, m)
	}
	return nil
}

func (s *EtcdBackend) DeleteLocationMiddleware(hostname, locationId, mType, id string) error {
	if _, err := s.client.Delete(s.path("hosts", hostname, "locations", locationId, "middlewares", mType, id), true); err != nil {
		if notFound(err) {
			return fmt.Errorf("Middleware %s('%s') not found", mType, id)
		}
	}
	return nil
}

// Watches etcd changes and generates structured events telling vulcand to add or delete locations, hosts etc.
// if initialSetup is true, reads the existing configuration and generates events for inital configuration of the proxy.
func (s *EtcdBackend) WatchChanges(changes chan interface{}, initialSetup bool) error {
	if initialSetup == true {
		log.Infof("Etcd backend reading initial configuration")
		if err := s.generateChanges(changes); err != nil {
			log.Errorf("Failed to generate changes: %s, stopping watch.", err)
			return err
		}
	}
	// This index helps us to get changes in sequence, as they were performed by clients.
	waitIndex := uint64(0)
	for {
		response, err := s.client.Watch(s.etcdKey, waitIndex, true, nil, s.cancelC)
		if err != nil {
			if err == etcd.ErrWatchStoppedByUser {
				log.Infof("Stop watching: graceful shutdown")
				return nil
			}
			log.Errorf("Stop watching: Etcd client error: %v", err)
			return err
		}

		waitIndex = response.Node.ModifiedIndex + 1
		log.Infof("Got response: %s %s %d %v",
			response.Action, response.Node.Key, response.EtcdIndex, err)
		change, err := s.parseChange(response)
		if err != nil {
			log.Errorf("Failed to process change: %s, ignoring", err)
			continue
		}
		if change != nil {
			log.Infof("Sending change: %s", change)
			select {
			case changes <- change:
			case <-s.stopC:
				return nil
			}
		}
	}
	return nil
}

func (s EtcdBackend) path(keys ...string) string {
	return "/" + strings.Join(append([]string{s.etcdKey}, keys...), "/")
}

// Reads the configuration of the vulcand and generates a sequence of events
// just like as someone was creating locations and hosts in sequence.
func (s *EtcdBackend) generateChanges(changes chan interface{}) error {
	upstreams, err := s.GetUpstreams()
	if err != nil {
		return err
	}
	for _, u := range upstreams {
		changes <- &UpstreamAdded{
			Upstream: u,
		}
		for _, e := range u.Endpoints {
			changes <- &EndpointAdded{
				Upstream: u,
				Endpoint: e,
			}
		}
	}

	hosts, err := s.readHosts(true)
	if err != nil {
		return err
	}

	for _, h := range hosts {
		changes <- &HostAdded{
			Host: h,
		}
		for _, l := range h.Locations {
			changes <- &LocationAdded{
				Host:     h,
				Location: l,
			}
		}
	}
	return nil
}

type MatcherFn func(*etcd.Response) (interface{}, error)

// Dispatches etcd key changes changes to the etcd to the matching functions
func (s *EtcdBackend) parseChange(response *etcd.Response) (interface{}, error) {
	matchers := []MatcherFn{
		s.parseHostChange,
		s.parseLocationChange,
		s.parseLocationUpstreamChange,
		s.parseLocationPathChange,
		s.parseUpstreamChange,
		s.parseUpstreamEndpointChange,
		s.parseUpstreamChange,
		s.parseMiddlewareChange,
	}
	for _, matcher := range matchers {
		a, err := matcher(response)
		if a != nil || err != nil {
			return a, err
		}
	}
	return nil, nil
}

func (s *EtcdBackend) parseHostChange(response *etcd.Response) (interface{}, error) {
	out := regexp.MustCompile("/hosts/([^/]+)$").FindStringSubmatch(response.Node.Key)
	if len(out) != 2 {
		return nil, nil
	}

	hostname := out[1]

	if response.Action == "create" {
		host, err := s.readHost(hostname, false)
		if err != nil {
			return nil, err
		}
		return &HostAdded{
			Host: host,
		}, nil
	} else if response.Action == "delete" {
		return &HostDeleted{
			Name:        hostname,
			HostEtcdKey: response.Node.Key,
		}, nil
	}
	return nil, fmt.Errorf("Unsupported action on the location: %s", response.Action)
}

func (s *EtcdBackend) parseLocationChange(response *etcd.Response) (interface{}, error) {
	out := regexp.MustCompile("/hosts/([^/]+)/locations/([^/]+)$").FindStringSubmatch(response.Node.Key)
	if len(out) != 3 {
		return nil, nil
	}
	hostname, locationId := out[1], out[2]
	host, err := s.readHost(hostname, false)
	if err != nil {
		return nil, err
	}
	if response.Action == "create" {
		location, err := s.GetLocation(hostname, locationId)
		if err != nil {
			return nil, err
		}
		return &LocationAdded{
			Host:     host,
			Location: location,
		}, nil
	} else if response.Action == "delete" {
		return &LocationDeleted{
			Host:               host,
			LocationId:         locationId,
			LocationBackendKey: strings.TrimSuffix(response.Node.Key, "/upstream"),
		}, nil
	}
	return nil, fmt.Errorf("Unsupported action on the location: %s", response.Action)
}

func (s *EtcdBackend) parseLocationUpstreamChange(response *etcd.Response) (interface{}, error) {
	out := regexp.MustCompile("/hosts/([^/]+)/locations/([^/]+)/upstream").FindStringSubmatch(response.Node.Key)
	if len(out) != 3 {
		return nil, nil
	}

	if response.Action != "create" && response.Action != "set" {
		return nil, fmt.Errorf("Unsupported action on the location upstream: %s", response.Action)
	}

	hostname, locationId := out[1], out[2]
	host, err := s.readHost(hostname, false)
	if err != nil {
		return nil, err
	}
	location, err := s.GetLocation(hostname, locationId)
	if err != nil {
		return nil, err
	}
	return &LocationUpstreamUpdated{
		Host:     host,
		Location: location,
	}, nil
}

func (s *EtcdBackend) parseLocationPathChange(response *etcd.Response) (interface{}, error) {
	out := regexp.MustCompile("/hosts/([^/]+)/locations/([^/]+)/path").FindStringSubmatch(response.Node.Key)
	if len(out) != 3 {
		return nil, nil
	}

	if response.Action != "create" && response.Action != "set" {
		return nil, fmt.Errorf("Unsupported action on the location path: %s", response.Action)
	}

	hostname, locationId := out[1], out[2]
	host, err := s.readHost(hostname, false)
	if err != nil {
		return nil, err
	}
	location, err := s.GetLocation(hostname, locationId)
	if err != nil {
		return nil, err
	}

	return &LocationPathUpdated{
		Host:     host,
		Location: location,
		Path:     response.Node.Value,
	}, nil
}

func (s *EtcdBackend) parseUpstreamChange(response *etcd.Response) (interface{}, error) {
	out := regexp.MustCompile("/upstreams/([^/]+)$").FindStringSubmatch(response.Node.Key)
	if len(out) != 2 {
		return nil, nil
	}
	upstreamId := out[1]
	if response.Action == "create" {
		upstream, err := s.GetUpstream(upstreamId)
		if err != nil {
			return nil, err
		}
		return &UpstreamAdded{
			Upstream: upstream,
		}, nil
	} else if response.Action == "delete" {
		return &UpstreamDeleted{
			UpstreamId:      upstreamId,
			UpstreamEtcdKey: response.Node.Key,
		}, nil
	}
	return nil, fmt.Errorf("Unsupported node action: %s", response)
}

func (s *EtcdBackend) parseUpstreamEndpointChange(response *etcd.Response) (interface{}, error) {
	out := regexp.MustCompile("/upstreams/([^/]+)/endpoints/([^/]+)").FindStringSubmatch(response.Node.Key)
	if len(out) != 3 {
		return nil, nil
	}
	upstreamId, endpointId := out[1], out[2]
	upstream, err := s.GetUpstream(upstreamId)
	if err != nil {
		return nil, err
	}

	affectedLocations, err := s.upstreamUsedBy(upstreamId)
	if err != nil {
		return nil, err
	}

	if response.Action == "create" {
		for _, e := range upstream.Endpoints {
			if e.Id == endpointId {
				return &EndpointAdded{
					Upstream:          upstream,
					Endpoint:          e,
					AffectedLocations: affectedLocations,
				}, nil
			}
		}
		return nil, fmt.Errorf("Endpoint %s not found", endpointId)
	} else if response.Action == "set" {
		for _, e := range upstream.Endpoints {
			if e.Id == endpointId {
				return &EndpointUpdated{
					Upstream:          upstream,
					Endpoint:          e,
					AffectedLocations: affectedLocations,
				}, nil
			}
		}
		return nil, fmt.Errorf("Endpoint %s not found", endpointId)
	} else if response.Action == "delete" {
		return &EndpointDeleted{
			Upstream:          upstream,
			EndpointId:        endpointId,
			EndpointEtcdKey:   response.Node.Key,
			AffectedLocations: affectedLocations,
		}, nil
	}
	return nil, fmt.Errorf("Unsupported action on the endpoint: %s", response.Action)
}

func (s *EtcdBackend) parseMiddlewareChange(response *etcd.Response) (interface{}, error) {
	out := regexp.MustCompile("/hosts/([^/]+)/locations/([^/]+)/middlewares/([^/]+)").FindStringSubmatch(response.Node.Key)
	if len(out) != 4 {
		return nil, nil
	}
	hostname, locationId, mType := out[1], out[2], out[3]
	mId := suffix(response.Node.Key)

	spec := s.registry.GetSpec(mType)
	if spec == nil {
		return nil, fmt.Errorf("Unregistered middleware type %s", mType)
	}
	host, err := s.GetHost(hostname)
	if err != nil {
		return nil, err
	}
	location, err := s.GetLocation(hostname, locationId)
	if err != nil {
		return nil, err
	}
	if response.Action == "create" {
		m, err := s.GetLocationMiddleware(hostname, locationId, mType, mId)
		if err != nil {
			return nil, err
		}
		return &LocationMiddlewareAdded{
			Host:                 host,
			Location:             location,
			Middleware:           m,
			MiddlewareBackendKey: response.Node.Key,
		}, nil
	} else if response.Action == "set" {
		m, err := s.GetLocationMiddleware(hostname, locationId, mType, mId)
		if err != nil {
			return nil, err
		}
		return &LocationMiddlewareUpdated{
			Host:                 host,
			Location:             location,
			Middleware:           m,
			MiddlewareBackendKey: response.Node.Key,
		}, nil
	} else if response.Action == "delete" {
		return &LocationMiddlewareDeleted{
			Host:                 host,
			Location:             location,
			MiddlewareId:         mId,
			MiddlewareType:       mType,
			MiddlewareBackendKey: response.Node.Key,
		}, nil
	}
	return nil, fmt.Errorf("Unsupported action on the rate: %s", response.Action)
}

func (s *EtcdBackend) readHosts(deep bool) ([]*Host, error) {
	hosts := []*Host{}
	for _, hostKey := range s.getDirs(s.etcdKey, "hosts") {
		host, err := s.readHost(suffix(hostKey), deep)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func (s *EtcdBackend) upstreamUsedBy(upstreamId string) ([]*Location, error) {
	locations := []*Location{}
	hosts, err := s.readHosts(true)
	if err != nil {
		return nil, err
	}
	for _, h := range hosts {
		for _, l := range h.Locations {
			if l.Upstream.Id == upstreamId {
				locations = append(locations, l)
			}
		}
	}
	return locations, nil
}

func (s *EtcdBackend) getVal(keys ...string) (string, bool) {
	response, err := s.client.Get(strings.Join(keys, "/"), false, false)
	if notFound(err) {
		return "", false
	}
	if isDir(response.Node) {
		return "", false
	}
	return response.Node.Value, true
}

func (s *EtcdBackend) getDirs(keys ...string) []string {
	var out []string
	response, err := s.client.Get(strings.Join(keys, "/"), true, true)
	if notFound(err) {
		return out
	}

	if response == nil || !isDir(response.Node) {
		return out
	}

	for _, srvNode := range response.Node.Nodes {
		if isDir(&srvNode) {
			out = append(out, srvNode.Key)
		}
	}
	return out
}

func (s *EtcdBackend) getVals(keys ...string) []Pair {
	var out []Pair
	response, err := s.client.Get(strings.Join(keys, "/"), true, true)
	if notFound(err) {
		return out
	}

	if !isDir(response.Node) {
		return out
	}

	for _, srvNode := range response.Node.Nodes {
		if !isDir(&srvNode) {
			out = append(out, Pair{srvNode.Key, srvNode.Value})
		}
	}
	return out
}

type Pair struct {
	Key string
	Val string
}

func suffix(key string) string {
	vals := strings.Split(key, "/")
	return vals[len(vals)-1]
}

func join(keys ...string) string {
	return strings.Join(keys, "/")
}

func notFound(e error) bool {
	err, ok := e.(*etcd.EtcdError)
	return ok && err.ErrorCode == 100
}

func objectError(e error, o IdProvider) error {
	if e == nil {
		return nil
	}
	switch err := e.(type) {
	case *etcd.EtcdError:
		if err.ErrorCode == 100 {
			return &NotFoundError{Obj: o}
		}
		if err.ErrorCode == 105 {
			return &AlreadyExistsError{Obj: o}
		}
	}
	return e
}

func isDir(n *etcd.Node) bool {
	return n != nil && n.Dir == true
}

type NotFoundError struct {
	Obj IdProvider
}

func (n *NotFoundError) Error() string {
	return fmt.Sprintf("%T '%s' not found", n.Obj, n.Obj.GetId())
}

type AlreadyExistsError struct {
	Obj IdProvider
}

func (n *AlreadyExistsError) Error() string {
	return fmt.Sprintf("%T '%s' already exists", n.Obj, n.Obj.GetId())
}

type IdProvider interface {
	GetId() string
}
