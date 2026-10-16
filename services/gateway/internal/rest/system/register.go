package system

import (
	"encoding/json"
	"net/http"

	"lab2/gateway/internal/rest/bookings/api"
	"lab2/platform/observability/metrics"
	"lab2/platform/rest"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/samber/oops"
)

func Register(
	router chi.Router,
	httpConfig rest.Config,
	openAPIConfig rest.OpenAPIConfig,
	telemetry *metrics.Metrics,
	metricsPath string,
) error {
	spec, err := api.GetSpec()
	if err != nil {
		return oops.Wrapf(err, "load OpenAPI schema")
	}
	schema, err := json.Marshal(spec)
	if err != nil {
		return oops.Wrapf(err, "encode OpenAPI schema")
	}
	schemaPath := httpConfig.APIPrefix + openAPIConfig.SchemaPath
	router.Get(schemaPath+".json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(schema)
	})

	docsPath := httpConfig.APIPrefix + openAPIConfig.DocsPath
	docsRouter := chi.NewRouter()
	cfg := huma.DefaultConfig(openAPIConfig.Title, openAPIConfig.Version)
	cfg.CreateHooks = nil
	cfg.DocsPath = docsPath
	cfg.OpenAPIPath = schemaPath
	cfg.DocsRenderer = huma.DocsRendererSwaggerUI
	humachi.New(docsRouter, cfg)
	// Publish only the documentation page; the generated contract owns the schema route.
	router.Get(docsPath, docsRouter.ServeHTTP)
	router.Handle(metricsPath, telemetry.Handler())
	router.Get("/manage/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return nil
}
