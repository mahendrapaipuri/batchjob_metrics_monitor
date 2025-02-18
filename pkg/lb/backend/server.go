package backend

import (
	"errors"
	"log/slog"

	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
)

// New returns a backend server of type `t`.
func New(t base.LBType, c base.ServerConfig, logger *slog.Logger) (Server, error) {
	switch t {
	case base.PromLB:
		return NewTSDB(c, logger)
	case base.PyroLB:
		return NewPyroscope(c, logger)
	}

	return nil, errors.New("unknown load balancer type. Only tsdb and pyroscope types supported")
}
