package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	. "github.com/mailgun/vulcand/backend"
	. "github.com/mailgun/vulcand/plugin"
	"github.com/mailgun/vulcand/plugin/registry"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

const CurrentVersion = "v1"

type Client struct {
	Addr string
}

func NewClient(addr string) *Client {
	return &Client{Addr: addr}
}

func (c *Client) GetHosts() ([]*Host, error) {
	var hosts []*Host
	err := c.GetJson(c.endpoint("hosts"), url.Values{}, hostsReader(&hosts))
	return hosts, err
}

func (c *Client) AddHost(name string) (*Host, error) {
	host, err := NewHost(name)
	if err != nil {
		return nil, err
	}
	var response *Host
	return response, c.PostJsonInterface(c.endpoint("hosts"), host, hostReader(&host))
}

func (c *Client) DeleteHost(name string) (*StatusResponse, error) {
	response := StatusResponse{}
	return &response, c.Delete(c.endpoint("hosts", name), &response)
}

func (c *Client) AddLocation(hostname, id, path, upstream string) (*Location, error) {
	location, err := NewLocation(hostname, id, path, upstream)
	if err != nil {
		return nil, err
	}
	var response *Location
	return response, c.PostJson(c.endpoint("hosts", hostname, "locations"), location, locationReader(&response))
}

func (c *Client) DeleteLocation(hostname, id string) (*StatusResponse, error) {
	response := StatusResponse{}
	return &response, c.Delete(c.endpoint("hosts", hostname, "locations", id), &response)
}

func (c *Client) UpdateLocationUpstream(hostname, location, upstream string) (*StatusResponse, error) {
	response := StatusResponse{}
	return &response, c.PutForm(c.endpoint("hosts", hostname, "locations", location), url.Values{"upstream": {upstream}}, &response)
}

func (c *Client) AddUpstream(id string) (*Upstream, error) {
	upstream, err := NewUpstream(id)
	if err != nil {
		return nil, err
	}
	var response *Upstream
	return response, c.PostJsonInterface(c.endpoint("upstreams"), upstream, &response)
}

func (c *Client) DeleteUpstream(id string) (*StatusResponse, error) {
	response := StatusResponse{}
	return &response, c.Delete(c.endpoint("upstreams", id), &response)
}

func (c *Client) GetUpstream(id string) (*Upstream, error) {
	response := UpstreamResponse{}
	err := c.Get(c.endpoint("upstreams", id), url.Values{}, &response)
	return response.Upstream, err
}

func (c *Client) DrainUpstreamConnections(upstreamId, timeout string) (int, error) {
	connections := ConnectionsResponse{}
	err := c.Get(c.endpoint("upstreams", upstreamId, "drain"), url.Values{"timeout": {timeout}}, &connections)
	return connections.Connections, err
}

func (c *Client) GetUpstreams() ([]*Upstream, error) {
	upstreams := UpstreamsResponse{}
	err := c.Get(c.endpoint("upstreams"), url.Values{}, &upstreams)
	return upstreams.Upstreams, err
}

func (c *Client) AddEndpoint(upstreamId, id, u string) (*Endpoint, error) {
	e, err := NewEndpoint(upstreamId, id, u)
	if err != nil {
		return nil, err
	}
	var response *Endpoint
	return response, c.PostJsonInterface(c.endpoint("upstreams", upstreamId, "endpoints"), e, &response)
}

func (c *Client) DeleteEndpoint(upstreamId, id string) (*StatusResponse, error) {
	response := StatusResponse{}
	return &response, c.Delete(c.endpoint("upstreams", upstreamId, "endpoints", id), &response)
}

func (c *Client) AddMiddleware(spec *MiddlewareSpec, hostname, locationId string, m Middleware) (Middleware, error) {
	var response Middleware
	return response, c.PostJson(
		c.endpoint("hosts", hostname, "locations", locationId, "middlewares", spec.Type),
		m, middlewareReader(spec, &response))
}

func (c *Client) DeleteMiddleware(spec *MiddlewareSpec, hostname, locationId, mId string) (*StatusResponse, error) {
	response := StatusResponse{}
	return &response, c.Delete(c.endpoint("hosts", hostname, "locations", locationId, "middlewares", spec.Type, mId), &response)
}

func (c *Client) PutForm(endpoint string, values url.Values, in interface{}) error {
	return c.RoundTripJson(func() (*http.Response, error) {
		req, err := http.NewRequest("PUT", endpoint, strings.NewReader(values.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return http.DefaultClient.Do(req)
	}, interfaceReader(in))
}

func (c *Client) PostForm(endpoint string, values url.Values, in interface{}) error {
	return c.RoundTripJson(func() (*http.Response, error) {
		return http.PostForm(endpoint, values)
	}, interfaceReader(in))
}

func (c *Client) PostJsonInterface(endpoint string, in interface{}, out interface{}) error {
	return c.PostJson(endpoint, in, interfaceReader(out))
}

func (c *Client) PostJson(endpoint string, in interface{}, reader JsonReaderFn) error {
	return c.RoundTripJson(func() (*http.Response, error) {
		data, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		return http.Post(endpoint, "application/json", bytes.NewBuffer(data))
	}, reader)
}

func (c *Client) Delete(endpoint string, in interface{}) error {
	return c.RoundTripJson(func() (*http.Response, error) {
		req, err := http.NewRequest("DELETE", endpoint, nil)
		if err != nil {
			return nil, err
		}
		return http.DefaultClient.Do(req)
	}, interfaceReader(in))
}

func (c *Client) Get(u string, params url.Values, in interface{}) error {
	return c.GetJson(u, params, interfaceReader(in))
}

func (c *Client) GetJson(u string, params url.Values, reader JsonReaderFn) error {
	baseUrl, err := url.Parse(u)
	if err != nil {
		return err
	}
	baseUrl.RawQuery = params.Encode()
	return c.RoundTripJson(func() (*http.Response, error) {
		return http.Get(baseUrl.String())
	}, reader)
}

type RoundTripFn func() (*http.Response, error)
type JsonReaderFn func([]byte) error

func interfaceReader(in interface{}) JsonReaderFn {
	return func(body []byte) error {
		if err := json.Unmarshal(body, in); err != nil {
			return fmt.Errorf("Failed to decode response '%s', error: %", body, err)
		}
		return nil
	}
}

func middlewareReader(spec *MiddlewareSpec, in *Middleware) JsonReaderFn {
	return func(body []byte) error {
		m, err := spec.FromJson(body)
		if err != nil {
			return fmt.Errorf("Failed to decode response '%s', error: %", body, err)
		}
		*in = m
		return nil
	}
}

func locationReader(in **Location) JsonReaderFn {
	return func(body []byte) error {
		l, err := LocationFromJson(body, registry.GetRegistry().GetSpec)
		if err != nil {
			return fmt.Errorf("Location reader: failed to decode response '%s', error: %", body, err)
		}
		*in = l
		return nil
	}
}

func hostsReader(in *[]*Host) JsonReaderFn {
	return func(body []byte) error {
		out, err := HostsFromJson(body, registry.GetRegistry().GetSpec)
		if err != nil {
			return fmt.Errorf("Failed to decode response '%s', error: %", body, err)
		}
		*in = out
		return nil
	}
}

func hostReader(in **Host) JsonReaderFn {
	return func(body []byte) error {
		out, err := HostFromJson(body, registry.GetRegistry().GetSpec)
		if err != nil {
			return fmt.Errorf("Failed to decode response '%s', error: %", body, err)
		}
		*in = out
		return nil
	}
}

func (c *Client) RoundTripJson(fn RoundTripFn, reader JsonReaderFn) error {
	response, err := fn()
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		var status *StatusResponse
		if json.Unmarshal(responseBody, &status); err != nil {
			return fmt.Errorf("Failed to decode response '%s', error: %", responseBody, err)
		}
		return status
	}
	return reader(responseBody)
}

func (c *Client) endpoint(params ...string) string {
	return fmt.Sprintf("%s/%s/%s", c.Addr, CurrentVersion, strings.Join(params, "/"))
}

type HostsResponse struct {
	Hosts []*Host
}

type UpstreamsResponse struct {
	Upstreams []*Upstream
}

type UpstreamResponse struct {
	Upstream *Upstream
}

type StatusResponse struct {
	Message string
}

func (e *StatusResponse) Error() string {
	return e.Message
}

type ConnectionsResponse struct {
	Connections int
}
