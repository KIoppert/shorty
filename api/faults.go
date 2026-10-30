package main

import (
	"math/rand/v2"
	"net/http"
	"time"
)

func injectFaults(cfg Config, mux *http.ServeMux) http.Handler {
	if cfg.FaultDelay == 0 && cfg.FaultErrorRate == 0 {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			time.Sleep(cfg.FaultDelay)
			if rand.Float64() < cfg.FaultErrorRate {
				_, r.Pattern = mux.Handler(r)
				writeError(w, http.StatusInternalServerError, "injected failure")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
