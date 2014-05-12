package connlimit

import (
	"encoding/json"
	"fmt"
	"github.com/mailgun/vulcan/limit"
	"github.com/mailgun/vulcand/middleware"
)

const ConnLimitType = "connlimit"

func GetSpec() middleware.Spec {
	return middleware.Spec{
		Type:      ConnLimitType,
		FromBytes: ParseConnLimit,
	}
}

// Control simultaneous connections for a location per some variable.
type ConnLimit struct {
	Id          string
	BackendKey  string `json:",omitempty"`
	Connections int
	Variable    string // Variable defines how the limiting should be done. e.g. 'client.ip' or 'request.header.X-My-Header'
}

func ParseConnLimit(in []byte) (middleware.Middleware, error) {
	var conn *ConnLimit
	err := json.Unmarshal(in, &conn)
	if err != nil {
		return nil, err
	}
	return NewConnLimit(conn.Connections, conn.Variable)
}

func NewConnLimit(connections int, variable string) (*ConnLimit, error) {
	if _, err := limit.VariableToMapper(variable); err != nil {
		return nil, err
	}
	if connections < 0 {
		return nil, fmt.Errorf("Connections should be > 0, got %d", connections)
	}
	return &ConnLimit{
		Connections: connections,
		Variable:    variable,
	}, nil
}

func (cl *ConnLimit) String() string {
	return fmt.Sprintf("ConnLimit(id=%s, key=%s, connections=%d, variable=%s)", cl.Id, cl.BackendKey, cl.Connections, cl.Variable)
}
