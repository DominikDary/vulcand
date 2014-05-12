// Etcd implementation of the backend, where all vulcand properties
// are implemented as directories or keys. This backend watches the changes
// and generates events with sequence of changes.
package etcdbackend

import (
	"fmt"
	"github.com/mailgun/go-etcd/etcd"
	log "github.com/mailgun/gotools-log"
	"github.com/mailgun/vulcan/endpoint"
	. "github.com/mailgun/vulcand/backend"
	"regexp"
	"strings"
)

type EtcdBackend struct {
	nodes       []string
	etcdKey     string
	consistency string
	client      *etcd.Client
	cancelC     chan bool
	stopC       chan bool
}

func NewEtcdBackend(nodes []string, etcdKey, consistency string) (*EtcdBackend, error) {
	client := etcd.NewClient(nodes)
	if err := client.SetConsistency(consistency); err != nil {
		return nil, err
	}
	b := &EtcdBackend{
		nodes:       nodes,
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

func (s *EtcdBackend) AddHost(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("Host name can not be empty")
	}
	_, err := s.client.CreateDir(s.path("hosts", name), 0)
	if isDupe(err) {
		return fmt.Errorf("Host '%s' already exists", name)
	}
	return err
}

func (s *EtcdBackend) DeleteHost(name string) error {
	_, err := s.client.Delete(s.path("hosts", name), true)
	return err
}

func (s *EtcdBackend) AddLocation(id, hostname, path, upstreamId string) error {
	log.Infof("Add Location(id=%s, hosntame=%s, path=%s, upstream=%s)")

	// Make sure upstream actually exists
	_, err := s.readUpstream(upstreamId)
	if err != nil {
		return err
	}
	// Create the location
	if id == "" {
		response, err := s.client.AddChildDir(s.path("hosts", hostname, "locations"), 0)
		if err != nil {
			return formatErr(err)
		}
		id = suffix(response.Node.Key)
	} else {
		_, err := s.client.CreateDir(s.path("hosts", hostname, "locations", id), 0)
		if err != nil {
			return formatErr(err)
		}
	}
	locationKey := s.path("hosts", hostname, "locations", id)
	if _, err := s.client.Create(join(locationKey, "path"), path, 0); err != nil {
		return formatErr(err)
	}
	if _, err := s.client.Create(join(locationKey, "upstream"), upstreamId, 0); err != nil {
		return formatErr(err)
	}
	return nil
}

func (s *EtcdBackend) UpdateLocationUpstream(hostname, id, upstreamId string) error {
	log.Infof("Update Location(id=%s, hostname=%s) set upstream %s", id, hostname, upstreamId)

	// Make sure upstream actually exists
	_, err := s.readUpstream(upstreamId)
	if err != nil {
		return err
	}

	// Make sure location actually exists
	location, err := s.readLocation(hostname, id)
	if err != nil {
		return err
	}

	// Update upstream
	if _, err := s.client.Set(join(location.EtcdKey, "upstream"), upstreamId, 0); err != nil {
		return formatErr(err)
	}

	return nil
}

func (s *EtcdBackend) DeleteLocation(hostname, id string) error {
	locationKey := s.path("hosts", hostname, "locations", id)
	if _, err := s.client.Delete(locationKey, true); err != nil {
		return formatErr(err)
	}
	return nil
}

func (s *EtcdBackend) GetUpstreams() ([]*Upstream, error) {
	return s.readUpstreams()
}

func (s *EtcdBackend) GetUpstream(id string) (*Upstream, error) {
	return s.readUpstream(id)
}

func (s *EtcdBackend) AddUpstream(upstreamId string) error {
	if upstreamId == "" {
		if _, err := s.client.AddChildDir(s.path("upstreams"), 0); err != nil {
			return formatErr(err)
		}
	} else {
		if _, err := s.client.CreateDir(s.path("upstreams", upstreamId), 0); err != nil {
			if isDupe(err) {
				return fmt.Errorf("Upstream '%s' already exists", upstreamId)
			}
			return formatErr(err)
		}
	}
	return nil
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
	return err
}

func (s *EtcdBackend) AddEndpoint(upstreamId, id, url string) error {
	if _, err := endpoint.ParseUrl(url); err != nil {
		return fmt.Errorf("Endpoint url '%s' is not valid")
	}
	if id == "" {
		if _, err := s.client.AddChild(s.path("upstreams", upstreamId, "endpoints"), url, 0); err != nil {
			return formatErr(err)
		}
	} else {
		if _, err := s.client.Create(s.path("upstreams", upstreamId, "endpoints", id), url, 0); err != nil {
			return formatErr(err)
		}
	}
	return nil
}

func (s *EtcdBackend) DeleteEndpoint(upstreamId, id string) error {
	if _, err := s.readUpstream(upstreamId); err != nil {
		if notFound(err) {
			return fmt.Errorf("Upstream '%s' not found", upstreamId)
		}
		return err
	}
	if _, err := s.client.Delete(s.path("upstreams", upstreamId, "endpoints", id), true); err != nil {
		if notFound(err) {
			return fmt.Errorf("Endpoint '%s' not found", id)
		}
	}
	return nil
}

func (s *EtcdBackend) AddLocationRateLimit(hostname, locationId string, id string, rateLimit *RateLimit) error {
	// Make sure location actually exists
	if _, err := s.readLocation(hostname, locationId); err != nil {
		return err
	}
	bytes, err := json.Marshal(rateLimit)
	if err != nil {
		return err
	}
	if id == "" {
		if _, err := s.client.AddChild(s.path("hosts", hostname, "locations", locationId, "limits", "rates"), string(bytes), 0); err != nil {
			return formatErr(err)
		}
	} else {
		if _, err := s.client.Create(s.path("hosts", hostname, "locations", locationId, "limits", "rates", id), string(bytes), 0); err != nil {
			return formatErr(err)
		}
	}
	return nil
}

func (s *EtcdBackend) UpdateLocationRateLimit(hostname, locationId string, id string, rateLimit *RateLimit) error {
	if len(id) == 0 || len(hostname) == 0 || len(locationId) == 0 {
		return fmt.Errorf("Provide hostname, location and rate id to update")
	}
	// Make sure location actually exists
	if _, err := s.readLocation(hostname, locationId); err != nil {
		return err
	}
	bytes, err := json.Marshal(rateLimit)
	if err != nil {
		return err
	}
	if _, err := s.client.Set(s.path("hosts", hostname, "locations", locationId, "limits", "rates", id), string(bytes), 0); err != nil {
		return formatErr(err)
	}
	return nil
}

func (s *EtcdBackend) DeleteLocationRateLimit(hostname, locationId, id string) error {
	if _, err := s.client.Delete(s.path("hosts", hostname, "locations", locationId, "limits", "rates", id), true); err != nil {
		if notFound(err) {
			return fmt.Errorf("Rate limit '%s' not found", id)
		}
	}
	return nil
}

func (s *EtcdBackend) AddLocationConnLimit(hostname, locationId, id string, connLimit *ConnLimit) error {
	// Make sure location actually exists
	if _, err := s.readLocation(hostname, locationId); err != nil {
		return err
	}
	bytes, err := json.Marshal(connLimit)
	if err != nil {
		return err
	}
	if id == "" {
		if _, err := s.client.AddChild(s.path("hosts", hostname, "locations", locationId, "limits", "connections"), string(bytes), 0); err != nil {
			return formatErr(err)
		}
	} else {
		if _, err := s.client.Create(s.path("hosts", hostname, "locations", locationId, "limits", "connections", id), string(bytes), 0); err != nil {
			return formatErr(err)
		}
	}
	return nil
}

func (s *EtcdBackend) UpdateLocationConnLimit(hostname, locationId string, id string, connLimit *ConnLimit) error {
	if len(id) == 0 || len(hostname) == 0 || len(locationId) == 0 {
		return fmt.Errorf("Provide hostname, location and rate id to update")
	}
	// Make sure location actually exists
	if _, err := s.readLocation(hostname, locationId); err != nil {
		return err
	}
	bytes, err := json.Marshal(connLimit)
	if err != nil {
		return err
	}
	if _, err := s.client.Set(s.path("hosts", hostname, "locations", locationId, "limits", "connections", id), string(bytes), 0); err != nil {
		return formatErr(err)
	}
	return nil
}

func (s *EtcdBackend) DeleteLocationConnLimit(hostname, locationId, id string) error {
	if _, err := s.client.Delete(s.path("hosts", hostname, "locations", locationId, "limits", "connections", id), true); err != nil {
		if notFound(err) {
			return fmt.Errorf("Connection limit '%s' not found", id)
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
	upstreams, err := s.readUpstreams()
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
		s.parseRateLimitChange,
		s.parseConnLimitChange,
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
		location, err := s.readLocation(hostname, locationId)
		if err != nil {
			return nil, err
		}
		return &LocationAdded{
			Host:     host,
			Location: location,
		}, nil
	} else if response.Action == "delete" {
		return &LocationDeleted{
			Host:            host,
			LocationId:      locationId,
			LocationEtcdKey: strings.TrimSuffix(response.Node.Key, "/upstream"),
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
	location, err := s.readLocation(hostname, locationId)
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
	location, err := s.readLocation(hostname, locationId)
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
		upstream, err := s.readUpstream(upstreamId)
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
	upstream, err := s.readUpstream(upstreamId)
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

func (s *EtcdBackend) parseRateLimitChange(response *etcd.Response) (interface{}, error) {
	out := regexp.MustCompile("/hosts/([^/]+)/locations/([^/]+)/limits/rates").FindStringSubmatch(response.Node.Key)
	if len(out) != 3 {
		return nil, nil
	}
	hostname, locationId := out[1], out[2]
	rateLimitId := suffix(response.Node.Key)

	host, err := s.readHost(hostname, false)
	if err != nil {
		return nil, err
	}
	location, err := s.readLocation(hostname, locationId)
	if err != nil {
		return nil, err
	}
	if response.Action == "create" {
		rate, err := s.readLocationRateLimit(response.Node.Key)
		if err != nil {
			return nil, err
		}
		return &LocationRateLimitAdded{
			Host:      host,
			Location:  location,
			RateLimit: rate,
		}, nil
	} else if response.Action == "set" {
		rate, err := s.readLocationRateLimit(response.Node.Key)
		if err != nil {
			return nil, err
		}
		return &LocationRateLimitUpdated{
			Host:      host,
			Location:  location,
			RateLimit: rate,
		}, nil
	} else if response.Action == "delete" {
		return &LocationRateLimitDeleted{
			Host:             host,
			Location:         location,
			RateLimitId:      rateLimitId,
			RateLimitEtcdKey: response.Node.Key,
		}, nil
	}
	return nil, fmt.Errorf("Unsupported action on the rate: %s", response.Action)
}

func (s *EtcdBackend) parseConnLimitChange(response *etcd.Response) (interface{}, error) {
	out := regexp.MustCompile("/hosts/([^/]+)/locations/([^/]+)/limits/connections").FindStringSubmatch(response.Node.Key)
	if len(out) != 3 {
		return nil, nil
	}
	hostname, locationId := out[1], out[2]
	connLimitId := suffix(response.Node.Key)

	host, err := s.readHost(hostname, false)
	if err != nil {
		return nil, err
	}
	location, err := s.readLocation(hostname, locationId)
	if err != nil {
		return nil, err
	}
	if response.Action == "create" {
		limit, err := s.readLocationConnLimit(response.Node.Key)
		if err != nil {
			return nil, err
		}
		return &LocationConnLimitAdded{
			Host:      host,
			Location:  location,
			ConnLimit: limit,
		}, nil
	} else if response.Action == "set" {
		limit, err := s.readLocationConnLimit(response.Node.Key)
		if err != nil {
			return nil, err
		}
		return &LocationConnLimitUpdated{
			Host:      host,
			Location:  location,
			ConnLimit: limit,
		}, nil
	} else if response.Action == "delete" {
		return &LocationConnLimitDeleted{
			Host:             host,
			Location:         location,
			ConnLimitId:      connLimitId,
			ConnLimitEtcdKey: response.Node.Key,
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

func (s *EtcdBackend) readHost(hostname string, deep bool) (*Host, error) {
	hostKey := s.path("hosts", hostname)
	_, err := s.client.Get(hostKey, false, false)
	if err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("Host '%s' not found", hostname)
		}
		return nil, err
	}
	host := &Host{
		Name:      hostname,
		EtcdKey:   hostKey,
		Locations: []*Location{},
	}

	if !deep {
		return host, nil
	}

	for _, locationKey := range s.getDirs(hostKey, "locations") {
		location, err := s.readLocation(hostname, suffix(locationKey))
		if err != nil {
			return nil, err
		}
		host.Locations = append(host.Locations, location)
	}
	return host, nil
}

func (s *EtcdBackend) readLocation(hostname, locationId string) (*Location, error) {
	locationKey := s.path("hosts", hostname, "locations", locationId)
	_, err := s.client.Get(locationKey, false, false)
	if err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("Location '%s' not found for Host '%s'", locationId, hostname)
		}
		return nil, err
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
		Hostname:   hostname,
		Id:         suffix(locationKey),
		EtcdKey:    locationKey,
		Path:       path,
		ConnLimits: []*ConnLimit{},
		RateLimits: []*RateLimit{},
	}
	upstream, err := s.readUpstream(upstreamKey)
	if err != nil {
		return nil, err
	}
	for _, cl := range s.getVals(locationKey, "limits", "connections") {
		connLimit, err := s.readLocationConnLimit(cl.Key)
		if err == nil {
			location.ConnLimits = append(location.ConnLimits, connLimit)
		}
	}

	for _, cl := range s.getVals(locationKey, "limits", "rates") {
		rateLimit, err := s.readLocationRateLimit(cl.Key)
		if err == nil {
			location.RateLimits = append(location.RateLimits, rateLimit)
		}
	}

	location.Upstream = upstream
	return location, nil
}

func (s *EtcdBackend) readUpstreams() ([]*Upstream, error) {
	upstreams := []*Upstream{}
	for _, upstreamKey := range s.getDirs(s.etcdKey, "upstreams") {
		upstream, err := s.readUpstream(suffix(upstreamKey))
		if err != nil {
			return nil, err
		}
		upstreams = append(upstreams, upstream)
	}
	return upstreams, nil
}

func (s *EtcdBackend) readLocationRateLimit(rateKey string) (*RateLimit, error) {
	rate, ok := s.getVal(rateKey)
	if !ok {
		return nil, fmt.Errorf("Missing rate limit key: %s", rateKey)
	}
	rl, err := ParseRateLimit(rate)
	if err != nil {
		return nil, err
	}
	rl.EtcdKey = rateKey
	rl.Id = suffix(rl.EtcdKey)
	return rl, nil
}

func (s *EtcdBackend) readLocationConnLimit(connKey string) (*ConnLimit, error) {
	conn, ok := s.getVal(connKey)
	if !ok {
		return nil, fmt.Errorf("Missing connection limit key: %s", connKey)
	}
	cl, err := ParseConnLimit(conn)
	if err != nil {
		return nil, err
	}
	cl.EtcdKey = connKey
	cl.Id = suffix(cl.EtcdKey)
	return cl, nil
}

func (s *EtcdBackend) readUpstream(upstreamId string) (*Upstream, error) {
	upstreamKey := s.path("upstreams", upstreamId)

	_, err := s.client.Get(upstreamKey, false, false)
	if err != nil {
		return nil, err
	}
	upstream := &Upstream{
		Id:        suffix(upstreamKey),
		EtcdKey:   upstreamKey,
		Endpoints: []*Endpoint{},
	}

	endpointPairs := s.getVals(join(upstream.EtcdKey, "endpoints"))
	for _, e := range endpointPairs {
		_, err := endpoint.ParseUrl(e.Val)
		if err != nil {
			fmt.Printf("Ignoring endpoint: failed to parse url: %s", e.Val)
			continue
		}
		e := &Endpoint{
			Url:     e.Val,
			EtcdKey: e.Key,
			Id:      suffix(e.Key),
		}
		upstream.Endpoints = append(upstream.Endpoints, e)
	}
	return upstream, nil
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

func formatErr(e error) error {
	switch err := e.(type) {
	case *etcd.EtcdError:
		return fmt.Errorf("Key error: %s", err.Message)
	}
	return e
}

func notFound(err error) bool {
	if err == nil {
		return false
	}
	eErr, ok := err.(*etcd.EtcdError)
	return ok && eErr.ErrorCode == 100
}

func isDupe(err error) bool {
	if err == nil {
		return false
	}
	eErr, ok := err.(*etcd.EtcdError)
	return ok && eErr.ErrorCode == 105
}

func isDir(n *etcd.Node) bool {
	return n != nil && n.Dir == true
}
