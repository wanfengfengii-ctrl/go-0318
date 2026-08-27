// Command server runs the abyssal pressure-housing qualification web service.
package main

import (
	"flag"
	"log"
	"net/http"

	"abyssal-pressure-housing-qualification/httpapi"
	"abyssal-pressure-housing-qualification/service"
	"abyssal-pressure-housing-qualification/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "abyssal.db", "SQLite database path")
	frontend := flag.String("frontend", "web", "frontend directory (empty to use embedded page)")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	svc := service.New(st)
	srv := httpapi.New(svc, *frontend)
	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
