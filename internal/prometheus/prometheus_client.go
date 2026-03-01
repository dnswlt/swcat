package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ClientOptions holds optional configuration for the Client.
// Both Prometheus and Thanos are supported — the API surface is compatible.
type ClientOptions struct {
	// Username and Password enable HTTP Basic Auth.
	Username string
	Password string
	// BearerToken enables Bearer token authentication (common with Thanos).
	BearerToken string
	// Timeout for HTTP requests. Defaults to 30s.
	Timeout time.Duration
}

// Client is a minimal HTTP client for the Prometheus (and Thanos) query API.
// It supports only instant queries — a single point-in-time evaluation of a
// PromQL expression, returning the current value of each matching time series.
type Client struct {
	baseURL    string
	opts       ClientOptions
	httpClient *http.Client
}

// NewClient creates a new Client targeting baseURL
// (e.g. "https://prometheus.example.com" or "https://thanos.example.com").
func NewClient(baseURL string, opts ClientOptions) *Client {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		opts:    opts,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Querier is the interface satisfied by *Client.
type Querier interface {
	Query(ctx context.Context, query string) ([]Sample, error)
}

// Sample represents a single measurement from an instant query result.
// Each sample corresponds to one time series that matched the PromQL expression.
type Sample struct {
	// Labels contains all metric labels, including __name__ when present.
	Labels    map[string]string
	Timestamp time.Time
	Value     float64
}

// Query executes an instant PromQL query and returns the individual samples
// from the result vector. The query is evaluated at the server's current time.
//
// This is suitable for "what is the current state of X?" questions, e.g.
//
//	kube_pod_container_status_ready{namespace="prod", ready="true"}
//
// API: GET /api/v1/query?query=<PromQL>
func (c *Client) Query(ctx context.Context, query string) ([]Sample, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("prometheus: building query URL: %w", err)
	}
	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("prometheus: creating request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus: executing query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("prometheus: reading response: %w", err)
	}

	var envelope struct {
		Status    string          `json:"status"`
		Data      json.RawMessage `json:"data"`
		ErrorType string          `json:"errorType"`
		Error     string          `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("prometheus: decoding response: %w", err)
	}
	if envelope.Status != "success" {
		return nil, &APIError{ErrorType: envelope.ErrorType, Message: envelope.Error}
	}

	var data struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string  `json:"metric"`
			Value  [2]json.RawMessage `json:"value"` // [unixTimestamp, "valueString"]
		} `json:"result"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return nil, fmt.Errorf("prometheus: decoding query data: %w", err)
	}
	if data.ResultType != "vector" {
		return nil, fmt.Errorf("prometheus: unexpected result type %q (only instant vector queries are supported)", data.ResultType)
	}

	samples := make([]Sample, 0, len(data.Result))
	for _, r := range data.Result {
		ts, val, err := parseSampleValue(r.Value)
		if err != nil {
			return nil, fmt.Errorf("prometheus: parsing sample: %w", err)
		}
		samples = append(samples, Sample{
			Labels:    r.Metric,
			Timestamp: ts,
			Value:     val,
		})
	}
	return samples, nil
}

// parseSampleValue parses the [timestamp, "value"] pair from a Prometheus result.
// The timestamp is a Unix epoch float; the value is a quoted float string that
// may be "NaN", "+Inf", or "-Inf".
func parseSampleValue(raw [2]json.RawMessage) (time.Time, float64, error) {
	var tsSec float64
	if err := json.Unmarshal(raw[0], &tsSec); err != nil {
		return time.Time{}, 0, fmt.Errorf("parsing timestamp: %w", err)
	}

	var valStr string
	if err := json.Unmarshal(raw[1], &valStr); err != nil {
		return time.Time{}, 0, fmt.Errorf("parsing value: %w", err)
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("parsing value %q as float: %w", valStr, err)
	}

	sec := int64(tsSec)
	nsec := int64((tsSec - float64(sec)) * 1e9)
	return time.Unix(sec, nsec), val, nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.opts.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.opts.BearerToken)
	} else if c.opts.Username != "" {
		req.SetBasicAuth(c.opts.Username, c.opts.Password)
	}
}

// APIError is returned when the Prometheus API responds with status "error".
type APIError struct {
	ErrorType string
	Message   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("prometheus API error (%s): %s", e.ErrorType, e.Message)
}
