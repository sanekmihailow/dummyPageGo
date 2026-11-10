package server

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/go-pkgz/lgr"
	"github.com/go-pkgz/routegroup"
)

//go:embed web/templates/*.html
var templatesFS embed.FS

//go:embed web/static
var staticFS embed.FS

// companyName is the fictional company the site pretends to represent.
const companyName = "Альтаир Системс"

// Server serves the dummy corporate site and the never-authenticating /admin page.
type Server struct {
	Host         string
	Port         int
	LogAttempts  bool
	LogPasswords bool
	TrustProxy   bool
	Maintenance  bool
	TLSCert      string // path to the TLS certificate (PEM); enables HTTPS together with TLSKey
	TLSKey       string // path to the TLS private key (PEM)
	Version      string

	pages       map[string]*template.Template
	admin       *template.Template
	maintenance *template.Template
}

// Run parses templates, starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	if err := s.parseTemplates(); err != nil {
		return err
	}

	tlsEnabled := s.TLSCert != "" && s.TLSKey != ""

	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	if tlsEnabled {
		httpSrv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	go func() {
		<-ctx.Done()
		log.Print("[INFO] shutting down")
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shCtx); err != nil {
			log.Printf("[WARN] graceful shutdown failed: %v", err)
		}
	}()

	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	if s.Maintenance {
		log.Printf("[INFO] listening on %s://%s (maintenance mode: under-development page for all requests)", scheme, addr)
	} else {
		log.Printf("[INFO] listening on %s://%s (log-attempts:%v log-passwords:%v trust-proxy:%v)",
			scheme, addr, s.LogAttempts, s.LogPasswords, s.TrustProxy)
	}

	// ListenAndServeTLS reads the cert/key files itself; empty paths mean plain HTTP.
	var err error
	if tlsEnabled {
		err = httpSrv.ListenAndServeTLS(s.TLSCert, s.TLSKey)
	} else {
		err = httpSrv.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// routes wires the handlers onto a routegroup over the stdlib mux.
func (s *Server) routes() http.Handler {
	router := routegroup.New(http.NewServeMux())
	router.Use(s.disguiseHeaders)
	if s.Maintenance {
		// keep the full site in the code, but serve only the "under development" page for now
		router.Use(s.maintenanceGate)
	}

	router.HandleFunc("GET /{$}", s.sitePage("/"))
	router.HandleFunc("GET /about", s.sitePage("/about"))
	router.HandleFunc("GET /services", s.sitePage("/services"))
	router.HandleFunc("GET /contacts", s.sitePage("/contacts"))
	router.HandleFunc("POST /contacts", s.contactSubmit)
	router.HandleFunc("GET /admin", s.adminPage)
	router.HandleFunc("POST /admin/login", s.adminLogin)

	sub, _ := fs.Sub(staticFS, "web/static")
	router.HandleFunc("GET /static/", s.cacheStatic(http.StripPrefix("/static/", http.FileServerFS(sub))))

	router.HandleFunc("/", s.notFound)
	return router
}

// disguiseHeaders makes responses look like a typical nginx-fronted site.
func (s *Server) disguiseHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Server", "nginx")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// maintenanceGate serves the "under development" page for every request except static
// assets (so the page keeps its styling) and /admin (the login form stays available), with
// a 503 status that proxies/crawlers understand.
func (s *Server) maintenanceGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/admin") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Retry-After", "3600")
		s.render(w, s.maintenance, "maintenance", http.StatusServiceUnavailable,
			pageData{Company: companyName, Year: time.Now().Year()})
	})
}

func (s *Server) cacheStatic(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		next.ServeHTTP(w, r)
	}
}

func (s *Server) parseTemplates() error {
	s.pages = make(map[string]*template.Template, len(sitePages))
	for key, p := range sitePages {
		t, err := template.ParseFS(templatesFS, "web/templates/layout.html", "web/templates/"+p.file)
		if err != nil {
			return err
		}
		s.pages[key] = t
	}
	admin, err := template.ParseFS(templatesFS, "web/templates/admin.html")
	if err != nil {
		return err
	}
	s.admin = admin

	maint, err := template.ParseFS(templatesFS, "web/templates/maintenance.html")
	if err != nil {
		return err
	}
	s.maintenance = maint
	return nil
}
