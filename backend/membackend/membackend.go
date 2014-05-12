// Provides in memory backend implementation, mostly used for test purposes
package membackend

import (
	"fmt"
	. "github.com/mailgun/vulcand/backend"
	"sync/atomic"
)

type MemBackend struct {
	counter   int32
	Hosts     []*Host
	Upstreams []*Upstream
}

func NewMemBackend() *MemBackend {
	return &MemBackend{
		Hosts: []*Host{},
	}
}

func (m *MemBackend) autoId() string {
	return fmt.Sprintf("%d", atomic.AddInt32(&m.counter, 1))
}

func (m *MemBackend) AddHost(h *Host) error {
	if m.GetHost(h.Name) != nil {
		return fmt.Errorf("Host %s already exists", h.Name)
	}
	m.Hosts = append(m.Hosts, h)
	return nil
}

func (m *MemBackend) GetHost(name string) *Host {
	for _, h := range m.Hosts {
		if h.Name == name {
			return h
		}
	}
	return nil
}

func (m *MemBackend) DeleteHost(name string) error {
	for i, h := range m.Hosts {
		if h.Name == name {
			m.Hosts = append(m.Hosts[:i], m.Hosts[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("Host %s does not exist", name)
}

func (m *MemBackend) GetLocation(hostname, id string) *Location {
	host := m.GetHost(hostname)
	if host == nil {
		return nil
	}
	for _, l := range host.Locations {
		if l.Id == id {
			return l
		}
	}
	return nil
}

func (m *MemBackend) AddLocation(loc *Location) (*Location, error) {
	host := m.GetHost(loc.Hostname)
	if host == nil {
		return nil, fmt.Errorf("Host not found")
	}
	if m.GetLocation(loc.Hostname, loc.Id) != nil {
		return nil, fmt.Errorf("Location %s already exists", loc.Id)
	}
	if loc.Id == "" {
		loc.Id = m.autoId()
	}
	host.Locations = append(host.Locations, loc)
	return loc, nil
}

func (m *MemBackend) DeleteLocation(hostname, id string) error {
	host := m.GetHost(hostname)
	if host == nil {
		return fmt.Errorf("Location %s not found")
	}
}

func (m *MemBackend) UpdateLocationUpstream(hostname, id string, upstreamId string) error {
	loc := m.GetLocation(hostname, id)
	if loc == nil {
		return fmt.Errorf("Location %s %s not found", hostname, id)
	}
	up := m.GetUpstream(upstreamId)
	if up == nil {
		return fmt.Errorf("Upstream not found")
	}
	loc.Upstream = up
	return nil
}

func (m *MemBackend) GetLocationRateLimit(hostname, locationId string, id string) *RateLimit {
	loc := m.GetLocation(hostname, locationId)
	if loc == nil {
		return nil
	}
	for _, rl := range loc.RateLimits {
		if rl.Id == id {
			return rl
		}
	}
	return nil
}

func (m *MemBackend) AddLocationRateLimit(hostname, locationId string, id string, rateLimit *RateLimit) (*RateLimit, error) {
	loc := m.GetLocation(hostname, locationId)
	if loc == nil {
		return nil, fmt.Errorf("Location %s %s not found", hostname, locationId)
	}
	if m.GetLocationRateLimit(hostname, locationId, id) != nil {
		return nil, fmt.Errorf("RateLimit with id %s already exists")
	}
	if rateLimit.Id == "" {
		rateLimit.Id = m.autoId()
	}
	loc.RateLimits = append(loc.RateLimits, rateLimit)
	return rateLimit, nil
}

func (m *MemBackend) DeleteLocationRateLimit(hostname, locationId, id string) error {
	loc := m.GetLocation(hostname, locationId)
	if loc == nil {
		return fmt.Errorf("Location %s %s not found", hostname, locationId)
	}
	for i, rl := range loc.RateLimits {
		if rl.Id == id {
			loc.RateLimits = append(loc.RateLimits[:i], loc.RateLimits[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("Rate limit %s not found")
}

func (m *MemBackend) UpdateLocationRateLimit(hostname, locationId string, id string, rateLimit *RateLimit) error {
}

func (m *MemBackend) AddLocationConnLimit(hostname, locationId, id string, connLimit *ConnLimit) error {
}

func (m *MemBackend) DeleteLocationConnLimit(hostname, locationId, id string) error {
}

func (m *MemBackend) UpdateLocationConnLimit(hostname, locationId string, id string, connLimit *ConnLimit) error {
}

func (m *MemBackend) GetUpstreams() ([]*Upstream, error) {
	return m.Upstreams, nil
}

func (m *MemBackend) GetUpstream(id string) *Upstream {
	for _, up := range m.Upstreams {
		if up.Id == id {
			return up
		}
	}
	return nil
}

func (m *MemBackend) AddUpstream(id string) error {
	if m.GetUpstream(id) != nil {
		return fmt.Errorf("Upstream %s already exists", id)
	}
	if id == "" {
		id = m.autoId()
	}
	m.Upstreams = append(m.Upstreams, &Upstream{Id: id, Endpoints: []*Endpoint{}})
	return nil
}

func (m *MemBackend) DeleteUpstream(id string) error {
	for i, u := range m.Upstreams {
		if u.Id == id {
			m.Upstreams = append(m.Upstreams[:i], m.Upstreams[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("Upstream %s does not exist", id)
}

func (m *MemBackend) AddEndpoint(upstreamId, id, url string) error {
	if m.GetEndpoint(upstreamId, id) != nil {
		return fmt.Errorf("Endpoint %s already exists", id)
	}
	u := m.GetUpstream(upstreamId)
	if u == nil {
		return fmt.Errorf("Upstream %s not found", upstreamId)
	}
	if id == "" {
		id = m.autoId()
	}
	e := &Endpoint{Id: id}
	u.Endpoints = append(u.Endpoints, e)
}

func (m *MemBackend) GetEndpoint(upstreamId, id string) *Endpoint {
	u := m.GetUpstream(upstreamId)
	if u == nil {
		return nil
	}
	for _, e := range u.Endpoints {
		if e.Id == id {
			return e
		}
	}
	return nil
}

func (m *MemBackend) DeleteEndpoint(upstreamId, id string) error {
}

func (m *MemBackend) WatchChanges(changes chan interface{}, initialSetup bool) error {
}
