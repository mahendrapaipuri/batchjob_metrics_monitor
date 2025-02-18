package backend

import (
	"io"
	"log/slog"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/prometheus/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPyroBackendWithBasicAuth(t *testing.T) {
	c := base.ServerConfig{
		Web: models.WebConfig{
			URL: "http://localhost:4040",
			HTTPClientConfig: config.HTTPClientConfig{
				BasicAuth: &config.BasicAuth{
					Username: "pyroscope",
					Password: "secret",
				},
			},
		},
	}
	b, err := NewPyroscope(c, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	assert.True(t, b.IsAlive())
	assert.Equal(t, "http://localhost:4040", b.URL().String())
	assert.Equal(t, 0, b.ActiveConnections())
	assert.NotEmpty(t, b.ReverseProxy())
}

func TestPyroBackendAlive(t *testing.T) {
	c := base.ServerConfig{Web: models.WebConfig{URL: "http://localhost:4040"}}
	b, err := NewPyroscope(c, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	b.SetAlive(b.IsAlive())

	assert.True(t, b.IsAlive())
}
