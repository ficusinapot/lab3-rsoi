package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"lab2/gateway/internal/rest/bookings/api"
	"lab2/platform/observability/metrics"
	"lab2/platform/rest"

	"github.com/go-chi/chi/v5"
)

func TestDocumentation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, prefix, docs, schema string
	}{
		{name: "default paths", prefix: "/api/v1", docs: "/docs", schema: "/openapi"},
		{name: "custom paths", prefix: "/booking/v2", docs: "/reference", schema: "/contract"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			telemetry, err := metrics.New(metrics.Config{
				Service: "gateway", Version: "test", Path: "/metrics", LatencyBuckets: []float64{0.01, 0.1},
			})
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			err = Register(router, rest.Config{APIPrefix: tc.prefix}, rest.OpenAPIConfig{
				Title: "Gateway", Version: "test", DocsPath: tc.docs, SchemaPath: tc.schema,
			}, telemetry, "/metrics")
			if err != nil {
				t.Fatal(err)
			}
			docs := httptest.NewRecorder()
			router.ServeHTTP(docs, httptest.NewRequest(http.MethodGet, tc.prefix+tc.docs, nil))
			if docs.Code != http.StatusOK || !strings.HasPrefix(docs.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("documentation: status=%d headers=%v", docs.Code, docs.Header())
			}
			if !strings.Contains(docs.Body.String(), "SwaggerUIBundle") ||
				!strings.Contains(docs.Body.String(), `data-url="`+tc.prefix+tc.schema+`.json"`) {
				t.Fatalf("documentation does not reference contract: %s", docs.Body.String())
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.prefix+tc.schema+".json", nil))
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("schema: status=%d headers=%v", response.Code, response.Header())
			}
			spec, err := api.GetSpec()
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(spec)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("published schema differs from generated contract")
			}
			for _, path := range []string{tc.prefix + tc.schema + ".yaml", "/docs", "/openapi.json"} {
				missing := httptest.NewRecorder()
				router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, path, nil))
				if missing.Code != http.StatusNotFound {
					t.Fatalf("unconfigured route %s returned %d", path, missing.Code)
				}
			}
		})
	}
}
