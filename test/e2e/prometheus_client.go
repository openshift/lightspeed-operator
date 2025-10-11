package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Jeffail/gabs/v2"
	routev1 "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// PrometheusClient provides access to the Prometheus, Thanos & Alertmanager API.
type PrometheusClient struct {
	// Host address of the endpoint.
	host string
	// Bearer token to use for authentication.
	token string
	// RoundTripper to use for HTTP transactions.
	rt http.RoundTripper
}

// DefaultPollInterval is the default interval for polling Prometheus metrics.
// Prometheus metrics are typically scraped every 30 seconds. This is enough for 3 scrapes.
const DefaultPrometheusQueryTimeout = 120 * time.Second

// NewPrometheusClientFromRoute creates and returns a new PrometheusClient from the given OpenShift route.
func NewPrometheusClientFromRoute(
	ctx context.Context,
	routeClient routev1.RouteV1Interface,
	namespace, name string,
	token string,
) (*PrometheusClient, error) {
	route, err := routeClient.Routes(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {

		logf.Log.Error(err, "Error fetching Prometheus route")
		return nil, err
	}

	return NewPrometheusClient(route.Spec.Host, token), nil
}

// WrapTransporter wraps an http.RoundTripper with another.
type WrapTransporter interface {
	WrapTransport(rt http.RoundTripper) http.RoundTripper
}

// NewPrometheusClient creates and returns a new PrometheusClient.
func NewPrometheusClient(host, token string, wts ...WrapTransporter) *PrometheusClient {
	var rt http.RoundTripper = &http.Transport{
		// #nosec G402
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	rt = (&HeaderInjector{Name: "Authorization", Value: "Bearer " + token}).WrapTransport(rt)
	rt = (&HeaderInjector{Name: "Content-Type", Value: "application/json"}).WrapTransport(rt)
	for i := range wts {
		rt = wts[i].WrapTransport(rt)
	}
	return &PrometheusClient{
		host:  host,
		rt:    rt,
		token: token,
	}
}

// MaxLength is the maximum string length returned by ClampMax().
const MaxLength = 1000

// ClampMax converts a slice of bytes to a string truncated to MaxLength.
func ClampMax(b []byte) string {
	s := string(b)
	if len(s) <= MaxLength {
		return s
	}
	return s[0:MaxLength-3] + "..."
}

// Do sends an HTTP request to the remote endpoint and returns the response.
func (c *PrometheusClient) Do(method string, path string, body []byte) (*http.Response, error) {
	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	u.Host = c.host
	u.Scheme = "https"

	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	return (&http.Client{Transport: c.rt}).Do(req)
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// HeaderInjector injects a fixed HTTP header into the inbound request.
type HeaderInjector struct {
	Name  string
	Value string
}

// WrapTransport implements the WrapTransporter interface.
func (h *HeaderInjector) WrapTransport(rt http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(
		func(req *http.Request) (*http.Response, error) {
			req.Header.Add(h.Name, h.Value)
			return rt.RoundTrip(req)
		},
	)
}

// QueryParameterInjector injects a fixed query parameter into the inbound request.
// It is typically used when querying kube-rbac-proxy.
type QueryParameterInjector struct {
	Name  string
	Value string
}

// WrapTransport implements the WrapTransporter interface.
func (qp *QueryParameterInjector) WrapTransport(rt http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(
		func(req *http.Request) (*http.Response, error) {
			q := req.URL.Query()
			q.Add(qp.Name, qp.Value)
			req.URL.RawQuery = q.Encode()
			return rt.RoundTrip(req)
		},
	)
}

// PrometheusQuery runs an HTTP GET request against the Prometheus query API and returns
// the response body.
func (c *PrometheusClient) PrometheusQuery(query string) ([]byte, error) {
	return c.PrometheusQueryWithStatus(query, http.StatusOK)
}

func (c *PrometheusClient) PrometheusQueryWithStatus(query string, status int) ([]byte, error) {
	resp, err := c.Do("GET", fmt.Sprintf("/api/v1/query?query=%s", url.QueryEscape(query)), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != status {
		return nil, fmt.Errorf("unexpected status code response, want %d, got %d (%q)", status, resp.StatusCode, ClampMax(body))
	}

	return body, nil
}

// PrometheusTargets runs an HTTP GET request against the Prometheus targets API and returns
// the response body.
func (c *PrometheusClient) PrometheusTargets() ([]byte, error) {
	resp, err := c.Do("GET", "/api/v1/targets", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code response, want %d, got %d (%q)", http.StatusOK, resp.StatusCode, ClampMax(body))
	}

	return body, nil
}

// PrometheusLabel runs an HTTP GET request against the Prometheus label API and returns
// the response body.
func (c *PrometheusClient) PrometheusLabel(label string) ([]byte, error) {
	resp, err := c.Do("GET", fmt.Sprintf("/api/v1/label/%s/values", url.QueryEscape(label)), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code response, want %d, got %d (%q)", http.StatusOK, resp.StatusCode, ClampMax(body))
	}

	return body, nil
}

// GetFirstValueFromPromQuery takes a query api response body and returns the
// value of the first timeseries. If body contains multiple timeseries
// GetFirstValueFromPromQuery errors.
func GetFirstValueFromPromQuery(body []byte) (float64, error) {
	res, err := gabs.ParseJSON(body)
	if err != nil {
		return 0, err
	}

	count, err := res.ArrayCountP("data.result")
	if err != nil {
		return 0, err
	}

	if count != 1 {
		return 0, fmt.Errorf("expected body to contain single timeseries but got %v", count)
	}

	timeseries, err := res.ArrayElementP(0, "data.result")
	if err != nil {
		return 0, err
	}

	value, err := timeseries.ArrayElementP(1, "value")
	if err != nil {
		return 0, err
	}

	v, err := strconv.ParseFloat(value.Data().(string), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse query value: %w", err)
	}

	return v, nil
}

// GetResultSizeFromPromQuery takes a query api response body and returns the
// size of the result vector.
func GetResultSizeFromPromQuery(body []byte) (int, error) {
	res, err := gabs.ParseJSON(body)
	if err != nil {
		return 0, err
	}

	count, err := res.ArrayCountP("data.result")
	if err != nil {
		return 0, err
	}

	return count, nil
}

// WaitForQueryReturnGreaterEqualOne see WaitForQueryReturn.
func (c *PrometheusClient) WaitForQueryReturnGreaterEqualOne(query string, timeout time.Duration) error {
	return c.WaitForQueryReturn(query, timeout, func(v float64) error {
		if v >= 1 {
			return nil
		}

		return fmt.Errorf("expected value to equal or greater than 1 but got %v", v)
	})
}

// WaitForQueryReturnOne see WaitForQueryReturn.
func (c *PrometheusClient) WaitForQueryReturnOne(query string, timeout time.Duration) error {
	return c.WaitForQueryReturn(query, timeout, func(v float64) error {
		if v == 1 {
			return nil
		}

		return fmt.Errorf("expected value to equal 1 but got %v", v)
	})
}

// WaitForQueryReturn waits for a given PromQL query for a given time interval
// and validates the **first and only** result with the given validate function.
func (c *PrometheusClient) WaitForQueryReturn(query string, timeout time.Duration, validate func(float64) error) error {

	var lastErr error

	err := wait.PollUntilContextTimeout(context.Background(), DefaultPollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		body, err := c.PrometheusQuery(query)
		if err != nil {
			lastErr = fmt.Errorf("error getting response for query %q: %w", query, err)
			return false, nil
		}
		v, err := GetFirstValueFromPromQuery(body)
		if err != nil {
			lastErr = fmt.Errorf("error getting first value from response body %q for query %q: %w", string(body), query, err)
			return false, nil
		}
		if err := validate(v); err != nil {
			lastErr = fmt.Errorf("error validating response body %q for query %q: %w", string(body), query, err)
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("WaitForQueryReturn - query: %s ; poll error: %w ; last error: %w", query, err, lastErr)
	}
	return nil

}

// WaitForQueryReturnEmpty waits for a given PromQL query return an empty response for a given time interval
func (c *PrometheusClient) WaitForQueryReturnEmpty(query string, timeout time.Duration) error {
	var lastErr error

	err := wait.PollUntilContextTimeout(context.Background(), DefaultPollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		body, err := c.PrometheusQuery(query)
		if err != nil {
			lastErr = fmt.Errorf("error getting response for query %q: %w", query, err)
			return false, nil
		}

		size, err := GetResultSizeFromPromQuery(body)
		if err != nil {
			lastErr = fmt.Errorf("error getting first value from response body %q for query %q: %w", string(body), query, err)
			return false, nil
		}

		if size > 0 {
			lastErr = fmt.Errorf("error validating response body %q for query %q: %w", string(body), query, err)
			return false, nil
		}

		return true, nil
	})

	if err != nil {
		return fmt.Errorf("WaitForQueryReturnEmpty - query: %s ; poll error: %w ; last error: %w", query, err, lastErr)
	}
	return nil

}
