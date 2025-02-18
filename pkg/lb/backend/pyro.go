package backend

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/prometheus/common/config"
)

// pyroServer implements a given backend Pyroscope server.
type pyroServer struct {
	url          *url.URL
	alive        bool
	mux          sync.RWMutex
	connections  int
	reverseProxy *httputil.ReverseProxy
	logger       *slog.Logger
}

// NewPyroscope returns an instance of backend Pyroscope server.
func NewPyroscope(c base.ServerConfig, logger *slog.Logger) (Server, error) {
	webURL, err := url.Parse(c.Web.URL)
	if err != nil {
		return nil, err
	}

	// Create a HTTP roundtripper
	httpRoundTripper, err := config.NewRoundTripperFromConfig(c.Web.HTTPClientConfig, "ceems_lb", config.WithUserAgent("ceems_lb/"+webURL.Host))
	if err != nil {
		return nil, err
	}

	// Setup reverse proxy
	rp := httputil.NewSingleHostReverseProxy(webURL)
	rp.Transport = httpRoundTripper

	return &pyroServer{
		url:          webURL,
		alive:        true,
		reverseProxy: rp,
		logger:       logger,
	}, nil
}

// RetentionPeriod is retention period of backend Pyroscope server.
func (b *pyroServer) RetentionPeriod() time.Duration {
	// Return a very long duration so that query will always
	// get proxied to one of the backends
	return 10 * 365 * 24 * time.Hour
}

// String returns name/web URL backend Pyroscope server.
func (b *pyroServer) String() string {
	if b.url != nil {
		return "url: " + b.url.Redacted()
	}

	return "No backend found"
}

// ActiveConnections is current number of active connections.
func (b *pyroServer) ActiveConnections() int {
	b.mux.RLock()
	connections := b.connections
	b.mux.RUnlock()

	return connections
}

// SetAlive sets the backend Pyroscope server as alive.
func (b *pyroServer) SetAlive(alive bool) {
	b.mux.Lock()
	b.alive = alive
	b.mux.Unlock()
}

// IsAlive returns if backend Pyroscope server is alive.
func (b *pyroServer) IsAlive() bool {
	b.mux.RLock()
	alive := b.alive
	defer b.mux.RUnlock()

	return alive
}

// URL of the backend Pyroscope server.
func (b *pyroServer) URL() *url.URL {
	return b.url
}

// ReverseProxy is reverse proxy of backend TSDB server.
func (b *pyroServer) ReverseProxy() *httputil.ReverseProxy {
	return b.reverseProxy
}

// Serves the request by the backend Pyroscope server.
func (b *pyroServer) Serve(w http.ResponseWriter, r *http.Request) {
	defer func() {
		b.mux.Lock()
		b.connections--
		b.mux.Unlock()
	}()

	b.mux.Lock()
	b.connections++
	b.mux.Unlock()
	b.reverseProxy.ServeHTTP(w, r)
}
