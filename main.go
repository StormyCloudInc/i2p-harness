// Package main is the entry point for the Crush CLI.
package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/charmbracelet/crush/internal/cmd"
	_ "github.com/charmbracelet/crush/internal/dns"
	"github.com/charmbracelet/crush/internal/i2p"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	// Install the SAM-only transport before any application clients start.
	http.DefaultTransport = i2p.NewTransport()
	if os.Getenv("CRUSH_PROFILE") != "" {
		go func() {
			slog.Info("Serving pprof at localhost:6060")
			if httpErr := http.ListenAndServe("localhost:6060", nil); httpErr != nil {
				slog.Error("Failed to pprof listen", "error", httpErr)
			}
		}()
	}

	cmd.Execute()
}
