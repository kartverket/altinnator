package main

import (
	"log"
	"net/http"
	"time"

	"github.com/kartverket/altinnator/internal/altinnmock"
)

func main() {
	server := &http.Server{Addr: ":8090", Handler: altinnmock.New(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
