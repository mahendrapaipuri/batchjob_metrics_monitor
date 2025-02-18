package backend

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// Custom errors.
var (
	ErrTypeAssertion = errors.New("failed type assertion")
)

// Server is the interface each backend server (TSDB/Pyroscope) needs to implement.
type Server interface {
	SetAlive(alive bool)
	IsAlive() bool
	URL() *url.URL
	String() string
	ActiveConnections() int
	RetentionPeriod() time.Duration
	ReverseProxy() *httputil.ReverseProxy
	Serve(w http.ResponseWriter, r *http.Request)
}
