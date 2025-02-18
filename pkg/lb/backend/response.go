package backend

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
)

// Regex to extract label name in values end point.
var regexLabelValues = regexp.MustCompile("(?:.*)/label/(.*)/values")

// PromResponseModifier modifies the TSDB response before sending to client.
func PromResponseModifier(labelsToFilter []string) func(r *http.Response) error {
	return func(r *http.Response) error {
		// If there are no labels to filter, return
		if len(labelsToFilter) == 0 {
			return nil
		}

		// Read response body
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		defer r.Body.Close()

		var newBody []byte

		// Get correct struct to read body into
		switch {
		case strings.HasSuffix(r.Request.URL.Path, "query") || strings.HasSuffix(r.Request.URL.Path, "query_range"):
			// Read response bytes into TSDB response
			var tsdbResp tsdb.Response[tsdb.Data]
			if err = json.Unmarshal(b, &tsdbResp); err != nil {
				return err
			}

			// Ensure that Data exists
			if tsdbResp.Data.Result == nil {
				newBody = b

				break
			}

			// Remove labelsToFilter from response
			for _, label := range labelsToFilter {
				for iresult := range tsdbResp.Data.Result {
					delete(tsdbResp.Data.Result[iresult].Metric, label)
				}
			}

			// Marshal into newBody
			if newBody, err = json.Marshal(tsdbResp); err != nil {
				return err
			}
		case strings.HasSuffix(r.Request.URL.Path, "series"):
			// Read response bytes into TSDB response
			var tsdbResp tsdb.Response[[]map[string]string]
			if err = json.Unmarshal(b, &tsdbResp); err != nil {
				return err
			}

			// Ensure that Data exists
			if tsdbResp.Data == nil {
				newBody = b

				break
			}

			// Remove labelsToFilter from response
			for _, label := range labelsToFilter {
				for idata := range tsdbResp.Data {
					delete(tsdbResp.Data[idata], label)
				}
			}

			// Marshal into newBody
			if newBody, err = json.Marshal(tsdbResp); err != nil {
				return err
			}
		case strings.HasSuffix(r.Request.URL.Path, "labels"):
			// Read response bytes into TSDB response
			var tsdbResp tsdb.Response[[]string]
			if err = json.Unmarshal(b, &tsdbResp); err != nil {
				return err
			}

			// Ensure that Data exists
			if tsdbResp.Data == nil {
				newBody = b

				break
			}

			var newData []string

			// Remove labelsToFilter from response
			for _, l := range tsdbResp.Data {
				if !slices.Contains(labelsToFilter, l) {
					newData = append(newData, l)
				}
			}

			// Replace Data in the response with newData
			tsdbResp.Data = newData

			// Marshal into newBody
			if newBody, err = json.Marshal(tsdbResp); err != nil {
				return err
			}
		case strings.HasSuffix(r.Request.URL.Path, "values"):
			// Get label name from URL path
			matches := regexLabelValues.FindStringSubmatch(r.Request.URL.Path)
			if len(matches) < 2 {
				newBody = b

				break
			}

			// Check if extracted label is in list of labelsToFilter
			// If it is not in labelsToFilter, nothing to do
			if !slices.Contains(labelsToFilter, matches[1]) {
				newBody = b

				break
			}

			// Read response bytes into TSDB response
			var tsdbResp tsdb.Response[[]string]
			if err = json.Unmarshal(b, &tsdbResp); err != nil {
				return err
			}

			// Replace Data in the response with nil
			tsdbResp.Data = nil

			// Marshal into newBody
			if newBody, err = json.Marshal(tsdbResp); err != nil {
				return err
			}
		}

		// Set it to response body
		r.Body = io.NopCloser(bytes.NewReader(newBody))

		// Set Content Length to newBody
		r.ContentLength = int64(len(newBody))
		r.Header.Set("Content-Length", strconv.Itoa(len(newBody)))

		return nil
	}
}
