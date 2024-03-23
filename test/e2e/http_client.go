package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"

	"k8s.io/apimachinery/pkg/util/wait"
)

type HTTPSClient struct {
	host       string
	serverName string
	caCertPool *x509.CertPool
	ctx        context.Context
}

func NewHTTPSClient(host, serverName string, certificate []byte) *HTTPSClient {
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(certificate)
	return &HTTPSClient{
		host:       host,
		serverName: serverName,
		caCertPool: caCertPool,
		ctx:        context.Background(),
	}
}

func (c *HTTPSClient) Get(queryUrl string) (*http.Response, error) {
	var rt http.RoundTripper = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    c.caCertPool,
			ServerName: c.serverName,
			MinVersion: tls.VersionTLS12,
		},
	}
	var resp *http.Response
	u, err := url.Parse(queryUrl)
	if err != nil {
		return nil, err
	}
	u.Host = c.host
	u.Scheme = "https"
	var body []byte = make([]byte, 1024)
	req, err := http.NewRequest(http.MethodGet, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Host = c.host
	resp, err = (&http.Client{Transport: rt}).Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *HTTPSClient) PostJson(queryUrl string, body []byte) (*http.Response, error) {
	var rt http.RoundTripper = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    c.caCertPool,
			ServerName: c.serverName,
			MinVersion: tls.VersionTLS12,
		},
	}
	var resp *http.Response
	u, err := url.Parse(queryUrl)
	if err != nil {
		return nil, err
	}
	u.Host = c.host
	u.Scheme = "https"
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Host = c.host
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err = (&http.Client{Transport: rt}).Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *HTTPSClient) waitForHTTPSGetStatus(queryUrl string, statusCode int) error { // nolint:unused
	var lastErr error
	err := wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, DefaultPollTimeout, true, func(ctx context.Context) (bool, error) {
		var resp *http.Response
		resp, lastErr = c.Get(queryUrl)
		if lastErr != nil {
			return false, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode != statusCode {
			lastErr = fmt.Errorf("unexpected status code %d", resp.StatusCode)
			return false, nil
		}
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("failed to wait for HTTPS response status: %w, lastErr: %w", err, lastErr)
	}

	return nil
}
