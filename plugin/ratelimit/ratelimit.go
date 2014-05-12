package ratelimit

import (
	"encoding/json"
	"fmt"
	"github.com/mailgun/vulcan/limit"
	"github.com/mailgun/vulcan/middleware"
	vdmiddleware "github.com/mailgun/vulcand/middleware"
	"time"
)

const Type = "ratelimit"

func GetSpec() vdmiddleware.Spec {
	return middleware.Spec{
		Type:      Type,
		FromBytes: ParseRateLimit,
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

func (r *RateLimit) GetType() string {
	return Type
}

func (r *RateLimit) GetId() string {
	return r.Id
}

// Returns serialized representation of the middleware
func (r *RateLimit) ToBytes() []byte {
}

// Returns vulcan library compatible middleware
func (r *RateLimit) NewInstance() middleware.Middleware {

}

func NewRateLimit(requests int, variable string, burst int, periodSeconds int) (*RateLimit, error) {
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

func ParseRateLimit(in string) (middleware.Middleware, error) {
	var rate *RateLimit
	err := json.Unmarshal([]byte(in), &rate)
	if err != nil {
		return nil, err
	}
	return NewRateLimit(rate.Requests, rate.Variable, rate.Burst, rate.PeriodSeconds)
}
