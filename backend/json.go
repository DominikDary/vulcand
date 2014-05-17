package backend

import (
	"encoding/json"
	"github.com/mailgun/vulcand/plugin"
)

type rawHosts struct {
	Hosts []json.RawMessage
}

type rawHost struct {
	Name       string
	BackendKey string
	Locations  []json.RawMessage
}

type rawLocation struct {
	Hostname    string
	BackendKey  string
	Path        string
	Id          string
	Upstream    *Upstream
	Middlewares []*rawMiddleware
}

type rawMiddleware struct {
	Id         string
	BackendKey string
	Type       string
	Middleware json.RawMessage
}

func HostsFromJson(in []byte, reader plugin.MiddlewareReader) ([]*Host, error) {
	var hs *rawHosts
	err := json.Unmarshal(in, &hs)
	if err != nil {
		return nil, err
	}
	out := []*Host{}
	if hs.Hosts != nil {
		for _, raw := range hs.Hosts {
			h, err := HostFromJson(raw, reader)
			if err != nil {
				return nil, err
			}
			out = append(out, h)
		}
	}
	return out, nil
}

func HostFromJson(in []byte, reader plugin.MiddlewareReader) (*Host, error) {
	var h *rawHost
	err := json.Unmarshal(in, &h)
	if err != nil {
		return nil, err
	}
	out, err := NewHost(h.Name)
	if err != nil {
		return nil, err
	}
	if h.Locations != nil {
		for _, raw := range h.Locations {
			l, err := LocationFromJson(raw, reader)
			if err != nil {
				return nil, err
			}
			out.Locations = append(out.Locations, l)
		}
	}
	return out, nil
}

func LocationFromJson(in []byte, reader plugin.MiddlewareReader) (*Location, error) {
	var l *rawLocation
	err := json.Unmarshal(in, &l)
	if err != nil {
		return nil, err
	}
	loc, err := NewLocation(l.Hostname, l.Id, l.Path, l.Upstream.Id)
	if err != nil {
		return nil, err
	}
	for _, el := range l.Middlewares {
		m, err := reader(el.Type, el.Middleware)
		if err != nil {
			return nil, err
		}
		loc.Middlewares = append(loc.Middlewares, &MiddlewareInstance{Id: el.Id, Type: el.Type, BackendKey: el.BackendKey, Middleware: m})
	}
	return loc, nil
}

func UpstreamFromJson(in []byte) (*Upstream, error) {
	var u *Upstream
	err := json.Unmarshal(in, &u)
	if err != nil {
		return nil, err
	}
	return NewUpstream(u.Id)
}

func EndpointFromJson(in []byte) (*Endpoint, error) {
	var e *Endpoint
	err := json.Unmarshal(in, &e)
	if err != nil {
		return nil, err
	}
	return NewEndpoint(e.UpstreamId, e.Id, e.Url)
}
