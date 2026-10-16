package rest

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/samber/oops"
)

type Config struct {
	Address           string        `mapstructure:"address"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	RequestTimeout    time.Duration `mapstructure:"request_timeout"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout"`
	MaxBodyBytes      int64         `mapstructure:"max_body_bytes"`
	APIPrefix         string        `mapstructure:"api_prefix"`
}

func (c Config) Validate() error {
	if _, _, err := net.SplitHostPort(c.Address); err != nil {
		return oops.Wrapf(err, "invalid HTTP address")
	}
	invalidRead := c.ReadHeaderTimeout <= 0 || c.ReadTimeout <= 0
	invalidWrite := c.WriteTimeout <= 0 || c.IdleTimeout <= 0
	invalidLifecycle := c.RequestTimeout <= 0 || c.ShutdownTimeout <= 0
	if invalidRead || invalidWrite || invalidLifecycle || c.MaxBodyBytes <= 0 {
		return oops.Errorf("HTTP timeouts and max_body_bytes must be positive")
	}
	if c.WriteTimeout <= c.RequestTimeout {
		return oops.Errorf("write_timeout must exceed request_timeout")
	}
	if err := ValidatePath(c.APIPrefix); err != nil {
		return oops.Wrapf(err, "invalid API prefix")
	}
	return nil
}

type OpenAPIConfig struct {
	Title      string `mapstructure:"title"`
	Version    string `mapstructure:"version"`
	DocsPath   string `mapstructure:"docs_path"`
	SchemaPath string `mapstructure:"schema_path"`
}

func (c OpenAPIConfig) Validate() error {
	missingInfo := c.Title == "" || c.Version == ""
	if missingInfo || c.DocsPath == c.SchemaPath {
		return oops.Errorf("invalid OpenAPI configuration")
	}
	return oops.Wrapf(errors.Join(ValidatePath(c.DocsPath), ValidatePath(c.SchemaPath)), "invalid OpenAPI paths")
}

func ValidatePath(value string) error {
	invalid := len(value) < 2 || !strings.HasPrefix(value, "/")
	unclean := path.Clean(value) != value || strings.ContainsAny(value, "{}*?#\\")
	if invalid || unclean {
		return oops.Errorf("path must be an absolute, clean, non-root URL path")
	}
	return nil
}

func New(c Config, docs OpenAPIConfig, middlewares ...func(http.Handler) http.Handler) (*chi.Mux, huma.API) {
	router := chi.NewRouter()
	router.Use(middlewares...)
	cfg := huma.DefaultConfig(docs.Title, docs.Version)
	// Keep response bodies identical to the service contract, without Huma's $schema field.
	cfg.CreateHooks = nil
	cfg.DocsPath = docs.DocsPath
	cfg.OpenAPIPath = docs.SchemaPath
	cfg.DocsRenderer = huma.DocsRendererSwaggerUI
	cfg.Servers = []*huma.Server{{URL: c.APIPrefix}}
	apiRouter := chi.NewRouter()
	apiRouter.Use(middleware.Timeout(c.RequestTimeout))
	router.Mount(c.APIPrefix, apiRouter)
	return router, humachi.New(apiRouter, cfg)
}

func Serve(ctx context.Context, cfg Config, handler http.Handler) error {
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return oops.Wrapf(err, "listen")
	}
	return serve(ctx, cfg, listener, handler)
}

func serve(ctx context.Context, cfg Config, listener net.Listener, handler http.Handler) error {
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		return oops.Wrapf(err, "serve HTTP")
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			closeErr := server.Close()
			<-done
			return oops.Wrapf(errors.Join(err, closeErr), "shutdown HTTP")
		}
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			return oops.Wrapf(err, "serve HTTP")
		}
		return nil
	}
}
