package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","app":"BE-MieAyam"}`))
	})

	addr := os.Getenv("APP_ADDR")
	if addr == "" { addr = ":8080" }
	log.Printf("BE-MieAyam listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
