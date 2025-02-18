package backend

import (
	"io"
	"log/slog"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	// TSDB
	_, err := New(base.PromLB, base.ServerConfig{Web: models.WebConfig{URL: "http://localhost:9090"}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// Pyro
	_, err = New(base.PyroLB, base.ServerConfig{Web: models.WebConfig{URL: "http://localhost:9090"}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// Unknown
	_, err = New(base.LBType(4), base.ServerConfig{Web: models.WebConfig{URL: "http://localhost:9090"}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Error(t, err)
}
