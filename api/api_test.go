package api

import (
	. "launchpad.net/gocheck"
	"testing"
)

func TestBackend(t *testing.T) { TestingT(t) }

type BackendSuite struct {
}

var _ = Suite(&BackendSuite{})

func (s *BackendSuite) TestVariableToMapper(c *C) {
}
