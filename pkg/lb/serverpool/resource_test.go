package serverpool

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var rbIDs = []string{"rb0", "rb1"}

func dummyServer(retention string) *httptest.Server {
	// Start test server
	expectedConfig := tsdb.Response[any]{
		Status: "success",
		Data: map[string]string{
			"yaml": "global:\n  scrape_interval: 15s\n  scrape_timeout: 10s",
		},
	}

	expectedFlags := tsdb.Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"query.lookback-delta": "5m",
			"query.max-samples":    "50000000",
			"query.timeout":        "2m",
		},
	}

	expectedRuntimeInfo := tsdb.Response[any]{
		Status: "success",
		Data: map[string]string{
			"storageRetention": retention,
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "config") {
			if err := json.NewEncoder(w).Encode(&expectedConfig); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "flags") {
			if err := json.NewEncoder(w).Encode(&expectedFlags); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expectedRuntimeInfo); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			time.Sleep(3 * time.Second)
		}
	}))

	return server
}

func TestResourceBasedLB(t *testing.T) {
	// Create a manager
	manager, err := New("resource-based", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// Retention periods
	periods := []string{"30d", "180d or 100GiB", "180d or 100GiB"}
	backendURLs := make(map[string][]*url.URL, len(rbIDs))
	backends := make(map[string][]backend.Server, len(rbIDs))

	// Make backends
	for i, p := range periods {
		for _, id := range rbIDs {
			dummyServer := dummyServer(p)
			defer dummyServer.Close()
			backendURL, err := url.Parse(dummyServer.URL)
			require.NoError(t, err)

			if _, ok := backendURLs[id]; !ok {
				backendURLs[id] = make([]*url.URL, len(periods))
				backends[id] = make([]backend.Server, len(periods))
			}

			backendURLs[id][i] = backendURL

			backend, err := backend.NewTSDB(base.ServerConfig{Web: models.WebConfig{URL: backendURL.String()}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			require.NoError(t, err)

			manager.Add(id, backend)
			backends[id][i] = backend
		}
	}

	// Manager size should be three
	for _, id := range rbIDs {
		assert.Equal(t, len(periods), manager.Size(id))
	}

	// Start wait group
	var wg sync.WaitGroup

	// Target should be backend[0]
	for _, id := range rbIDs {
		target := manager.Target(id, 10*time.Hour)
		assert.Equal(t, target.URL(), backendURLs[id][0])
	}

	// Backend[1] is serving a long request
	for _, id := range rbIDs {
		wg.Add(1)

		go func(i string) {
			r := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			defer wg.Done()

			t := backends[i][1]
			t.Serve(w, r)
		}(id)
	}

	// Let the server serve request
	time.Sleep(1 * time.Second)

	// This request should be proxied to backend[2] as backend[1] is busy with request
	for _, id := range rbIDs {
		target := manager.Target(id, 100*24*time.Hour)
		assert.Equal(t, target.URL().String(), backendURLs[id][2].String())
	}

	// Check if backend[1] has one active connection
	for _, id := range rbIDs {
		connTwo := backends[id][1].ActiveConnections()
		assert.Equal(t, 1, connTwo)
	}

	// Just to check for any race conditions
	for _, id := range rbIDs {
		wg.Add(1)

		go func(iid string) {
			defer wg.Done()

			for range 3 {
				manager.Target(iid, 10*time.Hour)
			}
		}(id)
	}

	// Wait for all go routines
	wg.Wait()

	// Make all backends offline
	for _, id := range rbIDs {
		for _, backend := range backends[id] {
			backend.SetAlive(false)
		}
	}

	// Should return nil target
	for _, id := range rbIDs {
		assert.Empty(t, manager.Target(id, 100*24*time.Hour))
	}
}
