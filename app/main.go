package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	log "github.com/go-pkgz/lgr"
	flags "github.com/jessevdk/go-flags"

	"github.com/itvolga/dummypage/app/server"
)

// opts holds all configuration, settable via flags or environment variables.
// Booleans default to false (go-flags cannot pass "--flag=false"), so the two toggles
// whose natural default is "on" are expressed as negatives: --no-log and --live.
var opts struct {
	Host         string `long:"host" env:"LISTEN_HOST" default:"0.0.0.0" description:"host/IP to listen on"`
	Port         int    `long:"port" env:"LISTEN_PORT" default:"8080" description:"port to listen on"`
	NoLog        bool   `long:"no-log" env:"NO_LOG" description:"disable logging of /admin login attempts"`
	LogPasswords bool   `long:"log-passwords" env:"LOG_PASSWORDS" description:"include submitted passwords in attempt logs"`
	TrustProxy   bool   `long:"trust-proxy" env:"TRUST_PROXY" description:"take client IP from X-Real-IP/X-Forwarded-For"`
	Live         bool   `long:"live" env:"LIVE" description:"serve the full site instead of the under-development page"`
	TLSCert      string `long:"tls-cert" env:"TLS_CERT" description:"path to TLS certificate (PEM); enables HTTPS together with --tls-key"`
	TLSKey       string `long:"tls-key" env:"TLS_KEY" description:"path to TLS private key (PEM)"`
	Dbg          bool   `long:"dbg" env:"DEBUG" description:"debug mode"`
}

// revision is set at build time via -ldflags "-X main.revision=...".
var revision = "unknown"

func main() {
	fmt.Printf("dummypage %s\n", revision)

	p := flags.NewParser(&opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	if _, err := p.Parse(); err != nil {
		os.Exit(2)
	}

	setupLog(opts.Dbg)
	log.Printf("[DEBUG] options: %+v", opts)

	if (opts.TLSCert == "") != (opts.TLSKey == "") {
		log.Fatalf("[ERROR] --tls-cert and --tls-key must be provided together")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &server.Server{
		Host:         opts.Host,
		Port:         opts.Port,
		LogAttempts:  !opts.NoLog,
		LogPasswords: opts.LogPasswords,
		TrustProxy:   opts.TrustProxy,
		Maintenance:  !opts.Live,
		TLSCert:      opts.TLSCert,
		TLSKey:       opts.TLSKey,
		Version:      revision,
	}
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("[ERROR] server failed: %v", err)
	}
}

// setupLog configures the global lgr logger.
func setupLog(dbg bool) {
	logOpts := []log.Option{log.Msec, log.LevelBraces}
	if dbg {
		logOpts = append(logOpts, log.Debug, log.CallerFile, log.CallerFunc)
	}
	log.SetupStdLogger(logOpts...)
	log.Setup(logOpts...)
}
