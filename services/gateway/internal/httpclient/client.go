package httpclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"lab2/gateway/internal/models/entities"

	"github.com/samber/oops"
)

type Config struct {
	URL                       string        `mapstructure:"url"`
	Timeout                   time.Duration `mapstructure:"timeout"`
	MaxInFlight               int           `mapstructure:"max_in_flight"`
	MaxIdleConnections        int           `mapstructure:"max_idle_connections"`
	MaxIdleConnectionsPerHost int           `mapstructure:"max_idle_connections_per_host"`
	IdleConnectionTimeout     time.Duration `mapstructure:"idle_connection_timeout"`
}

func (c Config) Validate() error {
	u, err := url.Parse(c.URL)
	if err != nil {
		return oops.Wrapf(err, "invalid service URL")
	}
	if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return oops.Errorf("invalid service URL")
	}
	if c.Timeout <= 0 || c.MaxInFlight <= 0 || c.MaxIdleConnections <= 0 ||
		c.MaxIdleConnectionsPerHost <= 0 || c.IdleConnectionTimeout <= 0 {
		return oops.Errorf("client timeout and connection limits must be positive")
	}
	return nil
}

type Client struct {
	http  *http.Client
	slots chan struct{}
}

func New(cfg Config) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = cfg.MaxIdleConnections
	transport.MaxIdleConnsPerHost = cfg.MaxIdleConnectionsPerHost
	transport.IdleConnTimeout = cfg.IdleConnectionTimeout
	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// net/http recognizes this sentinel by identity.
			return http.ErrUseLastResponse
		},
	}
	return &Client{http: client, slots: make(chan struct{}, cfg.MaxInFlight)}
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	select {
	case <-req.Context().Done():
		return nil, oops.Wrap(errors.Join(entities.ErrUpstreamTimeout, req.Context().Err()))
	case c.slots <- struct{}{}:
	}
	res, err := c.http.Do(req) //nolint:gosec // upstream URLs come from validated service configuration.
	if err != nil {
		<-c.slots
		failure := entities.ErrUpstreamUnavailable
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
			failure = entities.ErrUpstreamTimeout
		}
		return nil, oops.With("method", req.Method).Wrap(errors.Join(failure, err))
	}
	res.Body = &slotBody{ReadCloser: res.Body, release: func() { <-c.slots }}
	return res, nil
}

type slotBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *slotBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return oops.Wrapf(err, "close upstream response body")
}

func CheckStatus(response *http.Response, expected int) error {
	if response == nil {
		return oops.Wrapf(entities.ErrUpstreamUnavailable, "missing upstream response")
	}
	if response.StatusCode != expected {
		return oops.With("upstream_status", response.StatusCode).Wrap(ResponseError(response.StatusCode))
	}
	return nil
}

func ResponseError(status int) error {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return entities.ErrInvalidInput
	case http.StatusNotFound:
		return entities.ErrNotFound
	default:
		return entities.ErrUpstreamUnavailable
	}
}

func ResponseReadError(err error) error {
	if errors.Is(err, entities.ErrUpstreamTimeout) || errors.Is(err, entities.ErrUpstreamUnavailable) ||
		errors.Is(err, entities.ErrInvalidInput) || errors.Is(err, entities.ErrNotFound) {
		return err
	}
	return errors.Join(entities.ErrUpstreamUnavailable, err)
}

func (c *Client) CloseIdleConnections() { c.http.CloseIdleConnections() }
