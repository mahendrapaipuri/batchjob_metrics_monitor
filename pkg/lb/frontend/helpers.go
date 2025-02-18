//go:build cgo
// +build cgo

package frontend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"google.golang.org/protobuf/proto"
)

// These functions are nicked from https://github.com/prometheus/prometheus/blob/main/web/api/v1/api.go
var (
	// MinTime is the default timestamp used for the begin of optional time ranges.
	// Exposed to let downstream projects to reference it.
	MinTime = time.Unix(math.MinInt64/1000+62135596801, 0).UTC()

	// MaxTime is the default timestamp used for the end of optional time ranges.
	// Exposed to let downstream projects to reference it.
	MaxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999).UTC()

	minTimeFormatted = MinTime.Format(time.RFC3339Nano)
	maxTimeFormatted = MaxTime.Format(time.RFC3339Nano)
)

// Set query params into request's context and return new request.
func setQueryParams(r *http.Request, queryParams *ReqParams) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ReqParamsContextKey{}, queryParams))
}

// parseTSDBRequest parses TSDB query in the request after cloning it and reads them into request params.
func parseTSDBRequest(p *ReqParams, r *http.Request) error {
	var body []byte

	var err error

	// Make a new request and add newReader to that request body
	clonedReq := r.Clone(r.Context())

	// If request has no body go to proxy directly
	if r.Body == nil {
		return errors.New("no body found in the request")
	}

	// If failed to read body, skip verification and go to request proxy
	if body, err = io.ReadAll(r.Body); err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// clone body to existing request and new request
	r.Body = io.NopCloser(bytes.NewReader(body))
	clonedReq.Body = io.NopCloser(bytes.NewReader(body))

	// Get form values
	if err = clonedReq.ParseForm(); err != nil {
		return fmt.Errorf("failed to parse request form data: %w", err)
	}

	// Except for query API, rest of the load balanced API endpoint have start query param
	var targetTimeParam, targetQueryParam string

	switch {
	case strings.HasSuffix(clonedReq.URL.Path, "query"):
		targetQueryParam = "query"
		targetTimeParam = "time"
	case strings.HasSuffix(clonedReq.URL.Path, "query_range"):
		targetQueryParam = "query"
		targetTimeParam = "start"
	default:
		targetQueryParam = "match[]"
		targetTimeParam = "start"
	}

	// Parse TSDB's query in request query params
	if vals, ok := clonedReq.Form[targetQueryParam]; ok {
		for _, val := range vals {
			parseReqParams(p, val)
		}
	}

	// Parse TSDB's start query in request query params
	if startTime, err := parseTimeParam(clonedReq, targetTimeParam, time.Now().Local()); err != nil {
		p.queryPeriod = 0 * time.Second
		p.time = time.Now().Local().UnixMilli()
	} else {
		p.queryPeriod = time.Now().Local().Sub(startTime)
		p.time = startTime.Local().UnixMilli()
	}

	return nil
}

// parsePyroRequest parses Pyroscope query in the request after cloning it and reads them into request params.
func parsePyroRequest(p *ReqParams, r *http.Request) error {
	var body []byte

	var err error

	// If request has no body go to proxy directly
	if r.Body == nil {
		return errors.New("no body found in the request")
	}

	// If failed to read body, skip verification and go to request proxy
	if body, err = io.ReadAll(r.Body); err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// clone body to existing request
	r.Body = io.NopCloser(bytes.NewReader(body))

	var start int64

	// Read body into request data based on resource
	switch {
	case strings.HasSuffix(r.URL.Path, "SelectMergeStacktraces"):
		// Read body into request data
		data := querierv1.SelectMergeStacktracesRequest{}
		if err := proto.Unmarshal(body, &data); err != nil {
			return fmt.Errorf("failed to umarshall request body: %w", err)
		}

		// Parse Pyroscope's LabelSelector in request data
		if val := data.GetLabelSelector(); val != "" {
			parseReqParams(p, val)
		}

		// Get start time of query
		start = data.GetStart()
	case strings.HasSuffix(r.URL.Path, "LabelNames"):
		// Read body into request data
		data := typesv1.LabelNamesRequest{}
		if err := proto.Unmarshal(body, &data); err != nil {
			return fmt.Errorf("failed to umarshall request body: %w", err)
		}

		// Parse Pyroscope's LabelSelector in request data
		if vals := data.GetMatchers(); vals != nil {
			for _, val := range vals {
				parseReqParams(p, val)
			}
		}

		// Get start time of query
		start = data.GetStart()
	case strings.HasSuffix(r.URL.Path, "LabelValues"):
		// Read body into request data
		data := typesv1.LabelValuesRequest{}
		if err := proto.Unmarshal(body, &data); err != nil {
			return fmt.Errorf("failed to umarshall request body: %w", err)
		}

		// Parse Pyroscope's LabelSelector in request data
		if vals := data.GetMatchers(); vals != nil {
			for _, val := range vals {
				parseReqParams(p, val)
			}
		}

		// Get start time of query
		start = data.GetStart()
	}

	// Parse Pyroscope's start query in request query params
	// The times are already in milliseconds and so we need to
	// convert it to seconds before setting it to struct.
	if start == 0 {
		p.queryPeriod = 0 * time.Second
		p.time = time.Now().UTC().UnixMilli()
	} else {
		startTime := time.Unix(start/1000, 0).UTC()
		p.queryPeriod = time.Now().UTC().Sub(startTime)
		p.time = startTime.UnixMilli()
	}

	return nil
}

// parseRequestParams parses request parameters from `req` and reads them into `p`.
func parseReqParams(p *ReqParams, req string) {
	// Extract UUIDs from query
	for _, match := range regexpUUID.FindAllStringSubmatch(req, -1) {
		if len(match) > 1 {
			for _, uuid := range strings.Split(match[1], "|") {
				// Ignore empty strings
				if strings.TrimSpace(uuid) != "" && !slices.Contains(p.uuids, uuid) {
					p.uuids = append(p.uuids, uuid)
				}
			}
		}
	}

	// Extract ceems_id from query. If multiple values are provided, always
	// get the last and most recent one
	idMatches := regexID.FindAllStringSubmatch(req, -1)
	for _, match := range idMatches {
		if len(match) > 1 {
			for _, idMatch := range strings.Split(match[1], "|") {
				// Ignore empty strings
				if strings.TrimSpace(idMatch) != "" {
					p.clusterID = strings.TrimSpace(idMatch)
				}
			}
		}
	}
}

// Parse time parameter in request.
func parseTimeParam(r *http.Request, paramName string, defaultValue time.Time) (time.Time, error) {
	val := r.FormValue(paramName)
	if val == "" {
		return defaultValue, nil
	}

	result, err := parseTime(val)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time value for '%s': %w", paramName, err)
	}

	return result, nil
}

// Convert time parameter string into time.Time.
func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		ns = math.Round(ns*1000) / 1000

		return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC(), nil
	}

	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}

	// Stdlib's time parser can only handle 4 digit years. As a workaround until
	// that is fixed we want to at least support our own boundary times.
	// Context: https://github.com/prometheus/client_golang/issues/614
	// Upstream issue: https://github.com/golang/go/issues/20555
	switch s {
	case minTimeFormatted:
		return MinTime, nil
	case maxTimeFormatted:
		return MaxTime, nil
	}

	return time.Time{}, fmt.Errorf("cannot parse %q to a valid timestamp", s)
}
