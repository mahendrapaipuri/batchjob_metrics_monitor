package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/db"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	name    string
	req     string
	user    string
	admin   bool
	handler func(http.ResponseWriter, *http.Request)
	code    int
}

var (
	mockServerUnits = []models.Unit{
		{UUID: "1000", ClusterID: "slurm-0", ResourceManager: "slurm", User: "foousr"},
		{UUID: "10001", ClusterID: "os-0", ResourceManager: "openstack", User: "barusr"},
	}
	mockServerUsage = []models.Usage{
		{Project: "foo", ClusterID: "slurm-0", ResourceManager: "slurm"},
		{Project: "bar", ClusterID: "os-0", ResourceManager: "openstack"},
	}
	mockServerProjects = []models.Project{
		{Name: "foo", ClusterID: "slurm-0", ResourceManager: "slurm", Users: models.List{"foousr"}},
		{Name: "bar", ClusterID: "os-0", ResourceManager: "openstack", Users: models.List{"barusr"}},
	}
	mockServerUsers = []models.User{
		{Name: "foousr", ClusterID: "slurm-0", ResourceManager: "slurm", Projects: models.List{"foo"}},
		{Name: "bar", ClusterID: "os-0", ResourceManager: "openstack", Projects: models.List{"bar"}},
	}
	mockServerClusters = []models.Cluster{
		{ID: "slurm-0", Manager: "slurm"},
		{ID: "os-0", Manager: "openstack"},
	}
	mockStats = []models.Stat{
		{ClusterID: "slurm-0", ResourceManager: "slurm", NumUnits: 10, NumInActiveUnits: 2, NumActiveUnits: 8},
		{ClusterID: "os-0", ResourceManager: "openstack", NumUnits: 10, NumInActiveUnits: 8, NumActiveUnits: 2},
	}
	errTest = fmt.Errorf("failed to query 10 rows")
)

func setupServer(d string) *CEEMSServer {
	logger := log.NewNopLogger()
	server, _, _ := NewCEEMSServer(
		&Config{Logger: logger, DB: db.Config{Data: db.DataConfig{Path: d}}, Web: WebConfig{RequestsLimit: 10}},
	)
	server.maxQueryPeriod = time.Duration(time.Hour * 168)
	server.queriers = queriers{
		unit:    unitQuerier,
		usage:   usageQuerier,
		project: projectQuerier,
		user:    userQuerier,
		cluster: clusterQuerier,
		stat:    statQuerier,
	}
	return server
}

func unitQuerier(ctx context.Context, db *sql.DB, q Query, logger log.Logger) ([]models.Unit, error) {
	return mockServerUnits, nil
}

func usageQuerier(ctx context.Context, db *sql.DB, q Query, logger log.Logger) ([]models.Usage, error) {
	return mockServerUsage, nil
}

func projectQuerier(ctx context.Context, db *sql.DB, q Query, logger log.Logger) ([]models.Project, error) {
	return mockServerProjects, errTest
}

func userQuerier(ctx context.Context, db *sql.DB, q Query, logger log.Logger) ([]models.User, error) {
	return mockServerUsers, nil
}

func clusterQuerier(ctx context.Context, db *sql.DB, q Query, logger log.Logger) ([]models.Cluster, error) {
	return mockServerClusters, nil
}

func statQuerier(ctx context.Context, db *sql.DB, q Query, logger log.Logger) ([]models.Stat, error) {
	return mockStats, nil
}

func getMockUnits(
	_ Query,
	_ log.Logger,
) ([]models.Unit, error) {
	return mockServerUnits, nil
}

// Test users and users admin handlers
func TestUsersHandlers(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "users",
			req:     "/api/" + base.APIVersion + "/users?field=uuid&field=project",
			user:    "foousr",
			admin:   false,
			handler: server.users,
			code:    200,
		},
		{
			name:    "users admin",
			req:     "/api/" + base.APIVersion + "/users/admin?project=foo",
			user:    "foousr",
			admin:   true,
			handler: server.usersAdmin,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if test.admin {
			q := url.Values{}
			q.Add("user", "foousr")
			request.URL.RawQuery = q.Encode()
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		// Get body
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		// Unmarshal byte into structs.
		var response Response[models.User]
		json.Unmarshal(data, &response)
		assert.Equal(t, w.Code, test.code)
		assert.Equal(t, response.Status, "success")
		assert.Equal(t, response.Data, mockServerUsers)
	}
}

// Test projects and projects admin handlers
func TestProjectsHandler(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "projects",
			req:     "/api/" + base.APIVersion + "/projects",
			user:    "foousr",
			admin:   false,
			handler: server.projects,
			code:    200,
		},
		{
			name:    "projects admin",
			req:     "/api/" + base.APIVersion + "/projects/admin",
			user:    "foousr",
			admin:   true,
			handler: server.projectsAdmin,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if test.admin {
			q := url.Values{}
			q.Add("project", "foo")
			request.URL.RawQuery = q.Encode()
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		// Get body
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		// Unmarshal byte into structs.
		var response Response[models.Project]
		json.Unmarshal(data, &response)
		assert.Equal(t, w.Code, test.code)
		assert.Equal(t, response.Status, "success")
		assert.Equal(t, response.Data, mockServerProjects)
		assert.Equal(t, response.Warnings, []string{errTest.Error()})
	}
}

// Test units and units admin handlers
func TestUnitsHandler(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "units",
			req:     "/api/" + base.APIVersion + "/units",
			user:    "foousr",
			admin:   false,
			handler: server.units,
			code:    200,
		},
		{
			name:    "units admin",
			req:     "/api/" + base.APIVersion + "/units/admin",
			user:    "foousr",
			admin:   true,
			handler: server.unitsAdmin,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if test.admin {
			q := url.Values{}
			q.Add("user", "foousr")
			request.URL.RawQuery = q.Encode()
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		// Get body
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		// Unmarshal byte into structs.
		var response Response[models.Unit]
		json.Unmarshal(data, &response)
		assert.Equal(t, w.Code, test.code)
		assert.Equal(t, response.Status, "success")
		assert.Equal(t, response.Data, mockServerUnits)
	}
}

// Test usage and usage admin handlers
func TestUsageHandlers(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "current usage",
			req:     "/api/" + base.APIVersion + "/usage/current",
			user:    "foousr",
			admin:   false,
			handler: server.usage,
			code:    200,
		},
		{
			name:    "current usage cached",
			req:     "/api/" + base.APIVersion + "/usage/current",
			user:    "foousr",
			admin:   false,
			handler: server.usage,
			code:    200,
		},
		{
			name:    "global usage",
			req:     "/api/" + base.APIVersion + "/usage/global",
			user:    "foousr",
			admin:   false,
			handler: server.usage,
			code:    200,
		},
		{
			name:    "current usage admin",
			req:     "/api/" + base.APIVersion + "/usage/current/admin",
			user:    "adm1",
			admin:   true,
			handler: server.usageAdmin,
			code:    200,
		},
		{
			name:    "current usage admin cached",
			req:     "/api/" + base.APIVersion + "/usage/current/admin",
			user:    "adm1",
			admin:   true,
			handler: server.usageAdmin,
			code:    200,
		},
		{
			name:    "global usage admin",
			req:     "/api/" + base.APIVersion + "/usage/global/admin",
			user:    "adm1",
			admin:   true,
			handler: server.usageAdmin,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if test.admin {
			q := url.Values{}
			q.Add("user", "foousr")
			request.URL.RawQuery = q.Encode()
		}
		if strings.Contains(test.name, "current") {
			request = mux.SetURLVars(request, map[string]string{"mode": "current"})
		} else {
			request = mux.SetURLVars(request, map[string]string{"mode": "global"})
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		// Get body
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		// Unmarshal byte into structs.
		var response Response[models.Usage]
		json.Unmarshal(data, &response)
		assert.Equal(t, w.Code, test.code)
		assert.Equal(t, response.Status, "success")
		assert.Equal(t, response.Data, mockServerUsage)
		if strings.Contains(test.name, "cached") {
			assert.NotEmpty(t, res.Header["Expires"])
		} else {
			assert.Empty(t, res.Header["Expires"])
		}
	}
}

// Test stats admin handlers
func TestStatsHandlers(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "current stats",
			req:     "/api/" + base.APIVersion + "/stats/current",
			user:    "adm1",
			admin:   true,
			handler: server.statsAdmin,
			code:    200,
		},
		{
			name:    "global stats",
			req:     "/api/" + base.APIVersion + "/stats/global",
			user:    "adm1",
			admin:   true,
			handler: server.statsAdmin,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if test.admin {
			q := url.Values{}
			q.Add("user", "foousr")
			request.URL.RawQuery = q.Encode()
		}
		if strings.Contains(test.name, "current") {
			request = mux.SetURLVars(request, map[string]string{"mode": "current"})
		} else {
			request = mux.SetURLVars(request, map[string]string{"mode": "global"})
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		// Get body
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		// Unmarshal byte into structs.
		var response Response[models.Stat]
		json.Unmarshal(data, &response)
		assert.Equal(t, w.Code, test.code)
		assert.Equal(t, response.Status, "success")
		assert.Equal(t, response.Data, mockStats)
	}
}

// Test verify handler
func TestVerifyHandler(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	tests := []testCase{
		{
			name:    "verify bad data",
			req:     "/api/" + base.APIVersion + "/units/verify",
			user:    "foousr",
			admin:   false,
			handler: server.verifyUnitsOwnership,
			code:    400,
		},
		{
			name:    "verify forbidden",
			req:     "/api/" + base.APIVersion + "/units/verify?uuid=1234",
			user:    "foousr",
			admin:   false,
			handler: server.verifyUnitsOwnership,
			code:    403,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		assert.Equal(t, w.Code, test.code)
	}
}

// Test demo handlers
func TestDemoHandlers(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "units demo",
			req:     "/api/" + base.APIVersion + "/demo/units",
			user:    "foousr",
			admin:   false,
			handler: server.demo,
			code:    200,
		},
		{
			name:    "usage demo",
			req:     "/api/" + base.APIVersion + "/demo/usage",
			user:    "foousr",
			admin:   false,
			handler: server.demo,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if strings.Contains(test.name, "units") {
			request = mux.SetURLVars(request, map[string]string{"resource": "units"})
		} else {
			request = mux.SetURLVars(request, map[string]string{"resource": "usage"})
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		assert.Equal(t, w.Code, test.code)
	}
}

// Test clusters handlers
func TestClustersHandler(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/"+base.APIVersion+"/clusters/admin", nil)
	// Add user header
	currentUser := "foo"
	req.Header.Set("X-Grafana-User", currentUser)

	// Start recorder
	w := httptest.NewRecorder()
	server.clustersAdmin(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	// Expected result
	expectedClusters, _ := clusterQuerier(context.Background(), server.db, Query{}, server.logger)

	// Unmarshal byte into structs
	var response Response[models.Cluster]
	json.Unmarshal(data, &response)

	assert.Equal(t, response.Status, "success")
	assert.Equal(t, expectedClusters, response.Data)
}

// Test /units when from/to query parameters are malformed
func TestUnitsHandlerWithMalformedQueryParams(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("from", "10-12-2023")
	req.URL.RawQuery = q.Encode()

	// Start recorder
	w := httptest.NewRecorder()
	server.units(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	// Unmarshal byte into structs.
	var response Response[any]
	json.Unmarshal(data, &response)

	assert.Equal(t, response.Status, "error")
	assert.Equal(t, response.ErrorType, errorType("bad_data"))
	assert.Empty(t, response.Data)
}

// Test /units when from/to query parameters exceed max time window
func TestUnitsHandlerWithQueryWindowExceeded(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("from", "1672527600")
	q.Add("to", "1685570400")
	req.URL.RawQuery = q.Encode()

	// Start recorder
	w := httptest.NewRecorder()
	server.units(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	// Unmarshal byte into structs.
	var response Response[any]
	json.Unmarshal(data, &response)

	assert.Equal(t, response.Status, "error")
	assert.Equal(t, response.Error, "maximum query window exceeded")
	assert.Empty(t, response.Data)
}

// Test /units when from/to query parameters exceed max time window but when unit uuids
// are present
func TestUnitsHandlerWithUnituuidsQueryParams(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}
	defer f.Close()
	server := setupServer(tmpDir)
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("from", "1672527600")
	q.Add("to", "1685570400")
	q.Add("uuid", "foo-bar")
	req.URL.RawQuery = q.Encode()

	// Start recorder
	w := httptest.NewRecorder()
	server.units(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	// Expected result
	expectedUnits, _ := getMockUnits(Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response[models.Unit]
	json.Unmarshal(data, &response)

	assert.Equal(t, response.Status, "success")
	assert.Equal(t, expectedUnits, response.Data)
}

// // Test /usage
// func TestUsageHandler(t *testing.T) {
// 	server := setupServer()
// 	defer server.Shutdown(context.Background())

// 	// Create request
// 	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/current", nil)
// 	// Need to set path variables here
// 	req = mux.SetURLVars(req, map[string]string{"mode": "current"})

// 	// Add user header
// 	currentUser := "foo"
// 	req.Header.Set("X-Grafana-User", currentUser)

// 	// Start recorder
// 	w := httptest.NewRecorder()
// 	server.usage(w, req)
// 	res := w.Result()
// 	defer res.Body.Close()

// 	// Get body
// 	data, err := io.ReadAll(res.Body)
// 	if err != nil {
// 		t.Errorf("expected error to be nil got %v", err)
// 	}

// 	// Expected result
// 	expectedUsage, _ := usageQuerier(server.db, Query{}, server.logger)

// 	// Unmarshal byte into structs.
// 	var response Response[models.Usage]
// 	json.Unmarshal(data, &response)

// 	if response.Status != "success" {
// 		t.Errorf("expected success status got %#v", response)
// 	}

// 	if !reflect.DeepEqual(expectedUsage, response.Data) {
// 		t.Errorf("expected usage %#v usage, got %#v", expectedUsage, response.Data)
// 	}
// }
