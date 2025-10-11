package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/apimachinery/pkg/util/wait"
)

type HTTPSClient struct {
	host       string
	serverName string
	caCertPool *x509.CertPool
	ctx        context.Context
	clientCert *tls.Certificate
}

func NewHTTPSClient(host, serverName string, caCertificate, clientCert, clientKey []byte) *HTTPSClient {
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCertificate)
	httpsClient := &HTTPSClient{
		host:       host,
		serverName: serverName,
		caCertPool: caCertPool,
		ctx:        context.Background(),
	}

	if len(clientCert) > 0 && len(clientKey) > 0 {
		cert, err := tls.X509KeyPair(clientCert, clientKey)
		if err != nil {
			panic(err)
		}
		httpsClient.clientCert = &cert
	}
	return httpsClient

}

func (c *HTTPSClient) Get(queryUrl string, headers ...map[string]string) (*http.Response, error) {
	var rt http.RoundTripper = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    c.caCertPool,
			ServerName: c.serverName,
			MinVersion: tls.VersionTLS12,
		},
	}
	if c.clientCert != nil {
		rt.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{*c.clientCert}
	}
	var resp *http.Response
	u, err := url.Parse(queryUrl)
	if err != nil {
		return nil, err
	}
	u.Host = c.host
	u.Scheme = "https"
	var body = make([]byte, 1024)
	req, err := http.NewRequest(http.MethodGet, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Host = c.host
	if len(headers) > 0 {
		for key, value := range headers[0] {
			req.Header.Set(key, value)
		}
	}
	resp, err = (&http.Client{Transport: rt}).Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *HTTPSClient) PostJson(queryUrl string, body []byte, headers ...map[string]string) (*http.Response, error) {
	var rt http.RoundTripper = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    c.caCertPool,
			ServerName: c.serverName,
			MinVersion: tls.VersionTLS12,
		},
	}
	if c.clientCert != nil {
		rt.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{*c.clientCert}
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
	if len(headers) > 0 {
		for key, value := range headers[0] {
			req.Header.Set(key, value)
		}
	}
	resp, err = (&http.Client{Transport: rt}).Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *HTTPSClient) waitForHTTPSGetStatus(queryUrl string, statusCode int, headers ...map[string]string) error { // nolint:unused
	var lastErr error
	err := wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, DefaultPollTimeout, true, func(ctx context.Context) (bool, error) {
		var resp *http.Response
		resp, lastErr = c.Get(queryUrl, headers...)
		if lastErr != nil {
			// return EOF error to trigger port forwarding restart
			// the pipe is already closed, so we need to restart the port forwarding
			if strings.Contains(lastErr.Error(), "EOF") {
				return false, lastErr
			}
			return false, nil
		}
		defer func() { _ = resp.Body.Close() }()
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

func (c *HTTPSClient) waitForHTTPSPostStatus(queryUrl string, body []byte, statusCode int, headers ...map[string]string) error { // nolint:unused
	var lastErr error
	err := wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, DefaultPollTimeout, true, func(ctx context.Context) (bool, error) {
		var resp *http.Response
		resp, lastErr = c.PostJson(queryUrl, body, headers...)
		if lastErr != nil {
			return false, nil
		}
		defer func() { _ = resp.Body.Close() }()
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
