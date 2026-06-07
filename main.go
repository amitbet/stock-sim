package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/amitbet/stock-sim/internal/bootstrap"
	"github.com/amitbet/stock-sim/internal/httpapi"
)

func main() {
	data, err := bootstrap.LoadDataConfig()
	if err != nil {
		log.Fatal(err)
	}

	addr := ":" + bootstrap.EnvOrDefault("PORT", "3000")
	server, err := httpapi.NewServer(httpapi.Config{
		Addr:          addr,
		DBPath:        data.DBPath,
		DefaultSource: data.DefaultSource,
		UIDistPath:    "internal/httpapi/dist",
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("stock-sim listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
