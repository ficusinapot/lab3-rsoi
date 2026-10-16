//go:build integration

package hotels_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/autometrics-dev/autometrics-go/prometheus/autometrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMain(m *testing.M) {
	stop, err := autometrics.Init(autometrics.WithRegistry(prometheus.NewRegistry()))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	stop(nil)
	os.Exit(code)
}
