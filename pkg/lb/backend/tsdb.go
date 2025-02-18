// Package backend implements the backend TSDB server of load balancer app
package backend

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/prometheus/common/config"
)

// tsdbServer implements a given backend TSDB server.
type tsdbServer struct {
	url             *url.URL
	alive           bool
	mux             sync.RWMutex
	connections     int
	retentionPeriod time.Duration
	lastUpdate      time.Time
	updateInterval  time.Duration
	reverseProxy    *httputil.ReverseProxy
	client          *tsdb.Client
	logger          *slog.Logger
}

// NewTSDB returns an instance of backend TSDB server.
func NewTSDB(c base.ServerConfig, logger *slog.Logger) (Server, error) {
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
	rp.ModifyResponse = PromResponseModifier(c.FilterLabels) //nolint:bodyclose
	rp.Transport = httpRoundTripper

	// Create a new API client
	client, err := tsdb.New(webURL.String(), c.Web.HTTPClientConfig, logger.With("subsystem", "tsdb"))
	if err != nil {
		return nil, err
	}

	// Make server struct
	server := &tsdbServer{
		url:            webURL,
		alive:          true,
		reverseProxy:   rp,
		updateInterval: 3 * time.Hour,
		client:         client,
		logger:         logger,
	}

	// Update retention period
	server.RetentionPeriod()

	return server, nil
}

// String returns name/web URL backend TSDB server.
func (b *tsdbServer) String() string {
	if b.url != nil {
		return fmt.Sprintf("url: %s; retention: %s", b.url.Redacted(), b.retentionPeriod)
	}

	return "No backend found"
}

// RetentionPeriod is the retention period of backend TSDB server.
func (b *tsdbServer) RetentionPeriod() time.Duration {
	b.mux.RLock()
	retentionPeriod := b.retentionPeriod
	lastUpdate := b.lastUpdate
	b.mux.RUnlock()

	// Update retention period for every 3 hours
	if retentionPeriod == 0*time.Second || lastUpdate.IsZero() ||
		time.Since(lastUpdate) > b.updateInterval {
		newRetentionPeriod, err := b.fetchRetentionPeriod()
		// If errored, return last retention period
		if err != nil {
			b.logger.Error("Failed to update retention period", "backend", b.String(), "err", err)

			return retentionPeriod
		}

		// If update is successful, update struct and return new retention period
		b.mux.Lock()
		b.retentionPeriod = newRetentionPeriod
		b.lastUpdate = time.Now()
		b.mux.Unlock()

		return newRetentionPeriod
	}

	// If there is no need to update, return last updated value
	return retentionPeriod
}

// ActiveConnections is current number of active connections.
func (b *tsdbServer) ActiveConnections() int {
	b.mux.RLock()
	connections := b.connections
	b.mux.RUnlock()

	return connections
}

// SetAlive sets the backend TSDB server as alive.
func (b *tsdbServer) SetAlive(alive bool) {
	b.mux.Lock()
	b.alive = alive
	b.mux.Unlock()
}

// IsAlive returns if backend TSDB server is alive.
func (b *tsdbServer) IsAlive() bool {
	b.mux.RLock()
	alive := b.alive
	defer b.mux.RUnlock()

	return alive
}

// URL of backend TSDB server.
func (b *tsdbServer) URL() *url.URL {
	return b.url
}

// ReverseProxy is reverse proxy of backend TSDB server.
func (b *tsdbServer) ReverseProxy() *httputil.ReverseProxy {
	return b.reverseProxy
}

// Serve the request by the backend TSDB server.
func (b *tsdbServer) Serve(w http.ResponseWriter, r *http.Request) {
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

// Fetches retention period from backend TSDB server.
func (b *tsdbServer) fetchRetentionPeriod() (time.Duration, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var period time.Duration

	// For first update, get retention period from settings
	if b.retentionPeriod == 0 {
		// Get Prometheus settings
		settings := b.client.Settings(ctx)
		period = settings.RetentionPeriod
	} else {
		period = b.retentionPeriod
	}

	// If both retention storage and retention period are set,
	// depending on whichever hit first, TSDB uses the data based
	// on that retention.
	// So just because retention period is, say 30d, we might not
	// have data spanning 30d if retention size cannot accommodate
	// 30 days of data.
	//
	// Check if the data is actually available on TSDB by making
	// a range query on "up" from now to now - retention_period
	//
	// Seems like default limit of number of points per timeseries
	// is 11k and so we need to choose a step that should keep
	// the number of point below this limit. We choose 5k as limit
	// to be safe. Infact we dont need all the points but just
	// the first one.
	//
	// Make query parameters
	// When period is zero (during very first update) use a sufficiently
	// long range of 10 years to get retention period
	//
	// Use an initial value so that even if we do not find it
	// in runtime config, we return a sane default
	queryPeriod := period
	if queryPeriod == 0 {
		queryPeriod = 10 * 365 * 24 * time.Hour
	}

	// Make a range query
	query := fmt.Sprintf(`up{instance="%s:%s"}`, b.url.Hostname(), b.url.Port())
	if result, err := b.client.RangeQuery(
		ctx,
		query,
		time.Now().Add(-queryPeriod).UTC(),
		time.Now().UTC(),
		queryPeriod/5000,
	); err == nil {
		for metric, values := range result {
			if metric == "up" {
				// We are updating retention period only at a frequency set by
				// updateInterval. This means there is no guarantee that the
				// data until next update is present in the current TSDB.
				// So we need to reduce the actual retention period by the update
				// interval.
				// Here we reduce twice the update interval just to be
				// in a safe land
				t := int64(values[0].Timestamp)
				actualRetentionPeriod := (time.Since(time.UnixMilli(t)) - 2*b.updateInterval).Truncate(time.Hour)

				if actualRetentionPeriod < 0 {
					actualRetentionPeriod = time.Since(time.UnixMilli(t))
				}

				return actualRetentionPeriod, nil
			}
		}
	}

	return period, nil
}
