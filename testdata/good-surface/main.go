package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

// Pure stateless surface — proxies a few /api/v1/* core reads. No DB, no
// upstream provider client, no auth library.

func main() {
	coreBaseURL := envOrDefault("YGGDRASIL_CORE_HTTP_URL", "http://yggdrasil-core:9080")
	httpClient := &http.Client{Timeout: 5 * time.Second}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/proxy/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(coreBaseURL, "/")+"/readyz", nil)
		resp, err := httpClient.Do(req)
		if err != nil {
			http.Error(w, "core_unreachable", http.StatusServiceUnavailable)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": resp.Status})
	})

	_ = http.ListenAndServe(":9090", mux)
}

func envOrDefault(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
