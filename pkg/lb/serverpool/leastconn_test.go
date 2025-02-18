package serverpool

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var lcIDs = []string{"lc0", "lc1"}

func TestUnAvailableBackends(t *testing.T) {
	d := 0 * time.Second

	// Start manager
	manager, err := New("least-connection", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// Make dummy backend servers
	backendURLs := make(map[string][]*url.URL, 2)
	backends := make(map[string][]backend.Server, 2)

	for i := range 2 {
		for j, id := range lcIDs {
			backendURL, err := url.Parse(fmt.Sprintf("http://localhost:%d", 3333*(i+1)+j))
			require.NoError(t, err)

			if _, ok := backendURLs[id]; !ok {
				backendURLs[id] = make([]*url.URL, 2)
				backends[id] = make([]backend.Server, 2)
			}

			backendURLs[id][i] = backendURL

			backend, err := backend.NewTSDB(base.ServerConfig{Web: models.WebConfig{URL: backendURL.String()}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			require.NoError(t, err)

			backends[id][i] = backend
			manager.Add(id, backend)
		}
	}

	// Check manager size
	for _, id := range lcIDs {
		assert.Equal(t, 2, manager.Size(id))
	}

	// Set one backend to dead
	backends[lcIDs[0]][1].SetAlive(false)
	backends[lcIDs[1]][0].SetAlive(false)

	// Get target and it should be backend2
	for i, id := range lcIDs {
		target := manager.Target(id, d)
		assert.Equal(t, target.URL(), backendURLs[id][i])
	}

	// Set other backend to dead as well
	backends[lcIDs[0]][0].SetAlive(false)
	backends[lcIDs[1]][1].SetAlive(false)

	// Get target and it should be nil
	for _, id := range lcIDs {
		assert.Empty(t, manager.Target(id, d))
	}
}

func TestLeastConnectionLB(t *testing.T) {
	d := 0 * time.Second

	// Start manager
	manager, err := New("least-connection", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	backendURLs := make(map[string][]*url.URL, 2)
	backends := make(map[string][]backend.Server, 2)

	for i := range 2 {
		for _, id := range lcIDs {
			dummyServer := httptest.NewServer(h)
			defer dummyServer.Close()
			backendURL, err := url.Parse(dummyServer.URL)
			require.NoError(t, err)

			if _, ok := backendURLs[id]; !ok {
				backendURLs[id] = make([]*url.URL, 2)
				backends[id] = make([]backend.Server, 2)
			}

			backendURLs[id][i] = backendURL

			backend, err := backend.NewTSDB(base.ServerConfig{Web: models.WebConfig{URL: backendURL.String()}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			require.NoError(t, err)

			backends[id][i] = backend
			manager.Add(id, backend)
		}
	}

	// Check manager size
	for _, id := range lcIDs {
		assert.Equal(t, 2, manager.Size(id))
	}

	// Start wait group
	var wg sync.WaitGroup

	wg.Add(len(lcIDs))

	// Check if we get non nil target
	target := make(map[string]backend.Server)

	for _, id := range lcIDs {
		assert.NotEmpty(t, manager.Target(id, d))
	}

	// Serve a long request
	for _, id := range lcIDs {
		go func(i string) {
			defer wg.Done()

			r := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			if target := manager.Target(i, d); target != nil {
				target.Serve(w, r)
			}
		}(id)
	}

	// Let the server serve request
	time.Sleep(1 * time.Second)

	// Check new target is not nil
	for _, id := range lcIDs {
		newTarget := manager.Target(id, d)
		require.NotEmpty(t, newTarget)
		assert.Equal(t, 0, newTarget.ActiveConnections())
		assert.NotEqual(t, target[id], newTarget)
	}

	// For unknown ID expect nil
	assert.Empty(t, manager.Target("unknown", d))

	// Wait for go routines
	wg.Wait()
}
