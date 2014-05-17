package ratelimit

import (
	"encoding/json"
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/mailgun/vulcan/limit"
	"github.com/mailgun/vulcan/limit/tokenbucket"
	"github.com/mailgun/vulcan/middleware"
	"github.com/mailgun/vulcand/plugin"
	"time"
)

const Type = "ratelimit"

// Spec is an entry point of a plugin and will be called to register this middleware plugin withing vulcand
func GetSpec() *plugin.MiddlewareSpec {
	return &plugin.MiddlewareSpec{
		Type:     Type,
		FromJson: FromJson,
		FromCli:  FromCli,
		CliFlags: CliFlags(),
	}
}

// Rate controls how many requests per period of time is allowed for a location.
// Existing implementation is based on the token bucket algorightm http://en.wikipedia.org/wiki/Token_bucket
type RateLimit struct {
	Id            string
	BackendKey    string `json:",omitempty"`
	PeriodSeconds int    // Period in seconds, e.g. 3600 to set up hourly rates
	Burst         int    // Burst count, allowes some extra variance for requests exceeding the average rate
	Variable      string // Variable defines how the limiting should be done. e.g. 'client.ip' or 'request.header.X-My-Header'
	Requests      int    // Allowed average requests
}

// Type of the middleware
func (r *RateLimit) GetType() string {
	return Type
}

// Unique id of the rate limit instance
func (r *RateLimit) GetId() string {
	return r.Id
}

// Can be called by the backend to auto generate the id
func (r *RateLimit) SetId(id string) {
	r.Id = id
}

// Returns serialized representation of the middleware
func (r *RateLimit) ToJson() ([]byte, error) {
	return json.Marshal(r)
}

// Returns vulcan library compatible middleware
func (r *RateLimit) NewInstance() (middleware.Middleware, error) {
	mapper, err := limit.VariableToMapper(r.Variable)
	if err != nil {
		return nil, err
	}
	rate := tokenbucket.Rate{Units: int64(r.Requests), Period: time.Second * time.Duration(r.PeriodSeconds)}
	return tokenbucket.NewTokenLimiterWithOptions(mapper, rate, tokenbucket.Options{Burst: r.Burst})
}

func NewRateLimit(id string, requests int, variable string, burst int, periodSeconds int) (*RateLimit, error) {
	if _, err := limit.VariableToMapper(variable); err != nil {
		return nil, err
	}
	if requests <= 0 {
		return nil, fmt.Errorf("Requests should be > 0, got %d", requests)
	}
	if burst < 0 {
		return nil, fmt.Errorf("Burst should be >= 0, got %d", burst)
	}
	if periodSeconds <= 0 {
		return nil, fmt.Errorf("Period seconds should be > 0, got %d", periodSeconds)
	}
	return &RateLimit{
		Id:            id,
		Requests:      requests,
		Variable:      variable,
		Burst:         burst,
		PeriodSeconds: periodSeconds,
	}, nil
}

func (rl *RateLimit) String() string {
	return fmt.Sprintf("RateLimit(id=%s, key=%s, var=%s, reqs/%s=%d, burst=%d)",
		rl.Id, rl.Variable, time.Duration(rl.PeriodSeconds)*time.Second, rl.Requests, rl.Burst)
}

func FromJson(in []byte) (plugin.Middleware, error) {
	var rate *RateLimit
	err := json.Unmarshal(in, &rate)
	if err != nil {
		return nil, err
	}
	return NewRateLimit(rate.Id, rate.Requests, rate.Variable, rate.Burst, rate.PeriodSeconds)
}

// Constructs the middleware from the command line
func FromCli(c *cli.Context) (plugin.Middleware, error) {
	return NewRateLimit(c.String("id"), c.Int("requests"), c.String("var"), c.Int("burst"), c.Int("period"))
}

func CliFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{"id", "", "rate id"},
		cli.StringFlag{"var", "client.ip", "variable to rate against, e.g. client.ip, request.host or request.header.X-Header"},
		cli.IntFlag{"requests", 1, "amount of requests"},
		cli.IntFlag{"period", 1, "rate limit period in seconds"},
		cli.IntFlag{"burst", 1, "allowed burst"},
	}
}
