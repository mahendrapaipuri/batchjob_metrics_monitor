//go:build cgo
// +build cgo

package frontend

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	http_api "github.com/mahendrapaipuri/ceems/pkg/api/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(d string) (*sql.DB, error) {
	dbPath := filepath.Join(d, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create DB: %w", err)
	}

	stmts := `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE units (
	"id" integer not null primary key,
	"cluster_id" text,
	"uuid" text,
	"project" text,
	"usr" text,
	"started_at_ts" int
);
INSERT INTO units VALUES(1, 'rm-0', '1479763', 'prj1', 'usr1', 1735045414000);
INSERT INTO units VALUES(2, 'rm-0', '1481508', 'prj1', 'usr2', 1735045414000);
INSERT INTO units VALUES(3, 'rm-0', '1479765', 'prj2', 'usr2', 1735045414000);
INSERT INTO units VALUES(4, 'rm-0', '1481510', 'prj3', 'usr3', 1735045414000);
INSERT INTO units VALUES(5, 'rm-0', '1481508', 'prj3', 'usr3', 1703419414000);
INSERT INTO units VALUES(6, 'rm-1', '1479763', 'prj1', 'usr1', 1735045414000);
INSERT INTO units VALUES(7, 'rm-1', '1481508', 'prj1', 'usr2', 1735045414000);
INSERT INTO units VALUES(8, 'rm-1', '1479765', 'prj4', 'usr4', 1735045414000);
INSERT INTO units VALUES(9, 'rm-1', '1481510', 'prj5', 'usr5', 1735045414000);
CREATE TABLE usage (
	"id" integer not null primary key,
	"cluster_id" text,
	"project" text,
	"usr" text
);
INSERT INTO usage VALUES(1, 'rm-0', 'prj1', 'usr1');
INSERT INTO usage VALUES(2, 'rm-0', 'prj1', 'usr2');
INSERT INTO usage VALUES(3, 'rm-0', 'prj2', 'usr2');
INSERT INTO usage VALUES(4, 'rm-0', 'prj3', 'usr3');
INSERT INTO usage VALUES(5, 'rm-1', 'prj1', 'usr1');
INSERT INTO usage VALUES(6, 'rm-1', 'prj1', 'usr2');
INSERT INTO usage VALUES(7, 'rm-1', 'prj4', 'usr4');
INSERT INTO usage VALUES(8, 'rm-1', 'prj5', 'usr5');
CREATE TABLE projects (
	"id" integer not null primary key,
	"cluster_id" text,
	"name" text,
	"users" text
);
INSERT INTO projects VALUES(1, 'rm-0', 'prj1', '["usr1","usr2"]');
INSERT INTO projects VALUES(2, 'rm-0', 'prj2', '["usr2"]');
INSERT INTO projects VALUES(3, 'rm-0', 'prj3', '["usr3"]');
INSERT INTO projects VALUES(4, 'rm-1', 'prj1', '["usr1","usr2"]');
INSERT INTO projects VALUES(5, 'rm-1', 'prj4', '["usr4"]');
INSERT INTO projects VALUES(6, 'rm-1', 'prj5', '["usr5"]');
CREATE TABLE users (
	"id" integer not null primary key,
	"cluster_id" text,
	"name" text,
	"projects" text
);
INSERT INTO users VALUES(1, 'rm-0', 'usr1', '["prj1"]');
INSERT INTO users VALUES(2, 'rm-0', 'usr2', '["prj1","prj2"]');
INSERT INTO users VALUES(3, 'rm-0', 'usr3', '["prj3"]');
INSERT INTO users VALUES(4, 'rm-1', 'usr1', '["prj1"]');
INSERT INTO users VALUES(5, 'rm-1', 'usr2', '["prj1"]');
INSERT INTO users VALUES(6, 'rm-1', 'usr4', '["prj4"]');
INSERT INTO users VALUES(7, 'rm-1', 'usr5', '["prj5"]');
CREATE TABLE admin_users (
	"id" integer not null primary key,
	"source" text,
	"users" text
);
INSERT INTO admin_users VALUES(1, 'ceems', '["adm1","adm2","adm3"]');
INSERT INTO admin_users VALUES(2, 'grafana', '["adm4","adm5","adm6"]');
COMMIT;`

	_, err = db.Exec(stmts)
	if err != nil {
		return nil, fmt.Errorf("failed to insert mock data into DB: %w", err)
	}

	return db, nil
}

func setupMiddlewareWithDB(tmpDir string) (http.Handler, error) {
	// Setup test DB
	db, err := setupTestDB(tmpDir)
	if err != nil {
		return nil, err
	}

	// Create an instance of middleware
	amw := authenticationMiddleware{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		clusterIDs:    []string{"rm-0", "rm-1"},
		ceems:         ceems{db: db},
		parseRequest:  parseTSDBRequest,
		pathsACLRegex: regexpAllowedTSDBResources,
	}

	// create a handler to use as "next" which will verify the request
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// create the handler to test, using our custom "next" handler
	return amw.Middleware(nextHandler), nil
}

func setupMiddlewareWithAPI(tmpDir string) (http.Handler, error) {
	// Setup test DB
	db, err := setupTestDB(tmpDir)
	if err != nil {
		return nil, err
	}

	// Setup test CEEMS API server
	ceemsServer := setupCEEMSAPI(db)
	ceemsURL, _ := url.Parse(ceemsServer.URL)

	// Create an instance of middleware
	amw := authenticationMiddleware{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		clusterIDs:    []string{"rm-0", "rm-1"},
		ceems:         ceems{webURL: ceemsURL, client: http.DefaultClient},
		parseRequest:  parseTSDBRequest,
		pathsACLRegex: regexpAllowedTSDBResources,
	}

	// create a handler to use as "next" which will verify the request
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// create the handler to test, using our custom "next" handler
	return amw.Middleware(nextHandler), nil
}

func setupCEEMSAPI(db *sql.DB) *httptest.Server {
	// We copy the logic from CEEMS API server here for testing
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set headers
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Get current logged user and dashboard user from headers
		user := r.Header.Get(grafanaUserHeader)

		// Get list of queried uuids and cluster IDs
		uuids := r.URL.Query()["uuid"]
		rmIDs := r.URL.Query()["cluster_id"]

		var starts []int64

		for _, s := range r.URL.Query()["start"] {
			if is, err := strconv.ParseInt(s, 10, 64); err == nil {
				starts = append(starts, is)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Check if user is owner of the queries uuids
		if http_api.VerifyOwnership(ctx, user, rmIDs, uuids, starts, db, slog.New(slog.NewTextHandler(io.Discard, nil))) { //nolint:contextcheck
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("fail"))
		}
	}))

	return server
}

func TestMiddlewareWithDB(t *testing.T) {
	// Setup middleware handlers
	handlerToTestDB, err := setupMiddlewareWithDB(t.TempDir())
	require.NoError(t, err, "failed to setup middleware with DB")
	handlerToTestAPI, err := setupMiddlewareWithAPI(t.TempDir())
	require.NoError(t, err, "failed to setup middleware with API")

	tests := []struct {
		name   string
		req    string
		id     string
		user   string
		header bool
		code   int
	}{
		{
			name:   "forbid due to mismatch uuid",
			req:    "/query?query=foo{uuid=~\"1479765|1481510\"}&time=1735045414",
			id:     "rm-0",
			user:   "usr1",
			header: true,
			code:   403,
		},
		{
			name:   "forbid due to missing cluster_id",
			req:    "/query?query=foo{uuid=~\"1481508|1479765\"}&time=1735045414",
			user:   "usr2",
			header: true,
			code:   400,
		},
		{
			name:   "forbid due to incorrect start",
			req:    "/query?query=foo{uuid=~\"1481508\"}&time=1703419414",
			user:   "usr2",
			header: true,
			code:   400,
		},
		{
			name:   "allow query for admins",
			req:    "/query_range?query=foo{uuid=~\"1479765|1481510\"}&start=1735045414",
			id:     "rm-0",
			user:   "adm1",
			header: true,
			code:   200,
		},
		{
			name:   "forbid due to missing project",
			req:    "/query_range?query=foo{uuid=~\"123|345\"}&start=1735045414",
			id:     "rm-1",
			user:   "usr1",
			header: true,
			code:   403,
		},
		{
			name:   "forbid due to missing header",
			req:    "/query?query=foo{uuid=~\"123|345\"}&time=1735045414",
			id:     "rm-0",
			header: false,
			code:   401,
		},
		{
			name:   "pass due to correct uuid",
			req:    "/query_range?query=foo{uuid=\"1479763\"}&start=1735045414",
			id:     "rm-0",
			user:   "usr1",
			header: true,
			code:   200,
		},
		{
			name:   "pass due to correct uuid with gpuuuid in query",
			req:    "/query?query=foo{uuid=\"1479763\",gpuuuid=\"GPU-01234\"}&time=1735045414",
			id:     "rm-1",
			user:   "usr1",
			header: true,
			code:   200,
		},
		{
			name:   "pass due to uuid from same project",
			req:    "/query?query=foo{uuid=\"1481508\"}&time=1735045414",
			id:     "rm-0",
			user:   "usr1",
			header: true,
			code:   200,
		},
		{
			name:   "forbid due to no uuid",
			req:    "/query_range?query=foo{uuid=\"\"}",
			id:     "rm-0",
			header: true,
			user:   "usr3",
			code:   403,
		},
		{
			name:   "forbid due to no uuid and non-empty gpuuuid",
			req:    "/query?query=foo{uuid=\"\",gpuuuid=\"GPU-01234\"}",
			id:     "rm-0",
			header: true,
			user:   "usr2",
			code:   403,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, test.req, nil)
		if test.header {
			request.Header.Set(grafanaUserHeader, test.user)
		}

		if test.id != "" {
			request.Header.Set(ceemsClusterIDHeader, test.id)
		}

		// Tests with CEEMS DB
		dbRequest := request.Clone(request.Context())
		responseRecorderDB := httptest.NewRecorder()
		handlerToTestDB.ServeHTTP(responseRecorderDB, dbRequest)

		resDB := responseRecorderDB.Result()
		defer resDB.Body.Close()
		assert.Equal(t, test.code, resDB.StatusCode, "%s with DB", test.name)

		// Tests with CEEMS API
		apiRequest := request.Clone(request.Context())
		responseRecorderAPI := httptest.NewRecorder()
		handlerToTestAPI.ServeHTTP(responseRecorderAPI, apiRequest)

		resAPI := responseRecorderAPI.Result()
		defer resAPI.Body.Close()
		assert.Equal(t, test.code, resAPI.StatusCode, "%s with API", test.name)
	}
}
