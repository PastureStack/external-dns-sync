package main

// Modified by PastureStack contributors for independent maintenance and rebranding.

import (
	"net/http"
	"time"

	"github.com/PastureStack/external-dns-sync/config"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func startHealthCheck() {
	router := mux.NewRouter()
	router.HandleFunc("/", healthCheck).Methods("GET", "HEAD").Name("HealthCheck")
	router.HandleFunc("/ping", healthCheck).Methods("GET", "HEAD").Name("Ping")
	server := &http.Server{
		Addr:              config.HealthAddress,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	logrus.Info("Health check handler is listening on ", config.HealthAddress)
	logrus.Fatal(server.ListenAndServe())
}

func healthCheck(w http.ResponseWriter, req *http.Request) {
	// 1) Test the metadata service.
	_, err := m.MetadataClient.GetSelfStack()
	if err != nil {
		logrus.Error("Health check failed: unable to reach metadata")
		http.Error(w, "Failed to reach metadata server", http.StatusInternalServerError)
		return
	}

	// 2) Test the selected DNS provider.
	if err := provider.HealthCheck(); err != nil {
		logrus.Errorf("Health check failed: provider error: %v", err)
		http.Error(w, "Failed to reach the external DNS provider", http.StatusInternalServerError)
		return
	}

	// 3) Test the platform API.
	if err := platformAPI.TestConnect(); err != nil {
		logrus.Error("Health check failed: unable to reach the platform API")
		http.Error(w, "Failed to connect to the platform API", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		_, _ = w.Write([]byte("pong"))
	}
}
