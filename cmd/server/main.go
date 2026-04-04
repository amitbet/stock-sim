// Command server runs the stock-sim HTTP API and embedded SPA (browser UI). CGO-free; suitable for older Windows.
package main

import (
	"log"

	"stock-sim/internal/bootstrap"
	"stock-sim/internal/httpapi"
)

func main() {
	data, err := bootstrap.LoadDataConfig()
	if err != nil {
		log.Fatal(err)
	}

	cfg := httpapi.Config{
		Addr:          bootstrap.EnvOrDefault("SIM_ADDR", ":3002"),
		DBPath:        data.DBPath,
		DefaultSource: data.DefaultSource,
		UIDistPath:    bootstrap.EnvOrDefault("SIM_UI_DIST", "internal/httpapi/dist"),
	}

	server, err := httpapi.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	url := bootstrap.BrowserURL(cfg.Addr)
	go bootstrap.OpenBrowserWhenReady(url)

	log.Printf("stock-sim listening on %s using data source %s", cfg.Addr, cfg.DBPath)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
