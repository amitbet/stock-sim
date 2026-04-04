// Command server runs the stock-sim HTTP API and embedded SPA (browser UI). CGO-free; suitable for older Windows.
package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"stock-sim/internal/bootstrap"
	"stock-sim/internal/httpapi"
)

func main() {
	data, err := bootstrap.LoadDataConfig()
	if err != nil {
		log.Fatal(err)
	}

	cfg := httpapi.Config{
		Addr:          bootstrap.EnvOrDefault("SIM_ADDR", "127.0.0.1:0"),
		DBPath:        data.DBPath,
		DefaultSource: data.DefaultSource,
		UIDistPath:    bootstrap.EnvOrDefault("SIM_UI_DIST", "internal/httpapi/dist"),
	}

	server, err := httpapi.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	for i := 0; i < 200; i++ {
		if u := server.HTTPBaseURL(); u != "" {
			go bootstrap.OpenBrowserWhenReady(u)
			log.Printf("stock-sim listening on %s using data source %s", u, cfg.DBPath)
			select {}
		}
		time.Sleep(5 * time.Millisecond)
	}
	log.Fatal("server did not become ready")
}
