package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	name := flag.String("name", "backend", "Backend name")
	addr := flag.String("addr", ":9001", "Listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintf(w, "%s handled %s %s\n", *name, r.Method, r.URL.Path)
	})

	log.Printf("mock backend %s listening on %s", *name, *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
