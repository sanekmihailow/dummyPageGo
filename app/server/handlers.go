package server

import (
	"html/template"
	"math/rand/v2"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/go-pkgz/lgr"
)

const (
	maxBodyBytes = 4 << 10
	maxLogField  = 128
)

type page struct {
	file  string
	title string
}

var sitePages = map[string]page{
	"/":         {"index.html", "Главная"},
	"/about":    {"about.html", "О компании"},
	"/services": {"services.html", "Услуги"},
	"/contacts": {"contacts.html", "Контакты"},
	"404":       {"404.html", "Страница не найдена"},
}

type pageData struct {
	Company string
	Title   string
	Path    string
	Year    int
	Error   string
	Sent    bool
}

func (s *Server) newData(key string) pageData {
	return pageData{
		Company: companyName,
		Title:   sitePages[key].title,
		Path:    key,
		Year:    time.Now().Year(),
	}
}

func (s *Server) render(w http.ResponseWriter, t *template.Template, name string, status int, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("[WARN] render %s failed: %v", name, err)
	}
}

func (s *Server) sitePage(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, s.pages[key], "layout", http.StatusOK, s.newData(key))
	}
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, s.pages["404"], "layout", http.StatusNotFound, s.newData("404"))
}

// contactSubmit pretends to accept the contact form; nothing is stored.
func (s *Server) contactSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	_ = r.ParseForm()

	data := s.newData("/contacts")
	data.Sent = true
	if isHTMX(r) {
		s.render(w, s.pages["/contacts"], "contact-ok", http.StatusOK, data)
		return
	}
	s.render(w, s.pages["/contacts"], "layout", http.StatusOK, data)
}

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, s.admin, "admin", http.StatusOK, pageData{Company: companyName, Year: time.Now().Year()})
}

// adminLogin never authenticates: it optionally logs the attempt, waits a bit
// like a real password check would, then always reports invalid credentials.
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	_ = r.ParseForm()

	if s.LogAttempts {
		if s.LogPasswords {
			log.Printf("[INFO] admin login attempt ip=%s ua=%q user=%q password=%q",
				s.clientIP(r), truncate(r.UserAgent()), truncate(r.PostFormValue("username")), truncate(r.PostFormValue("password")))
		} else {
			log.Printf("[INFO] admin login attempt ip=%s ua=%q user=%q",
				s.clientIP(r), truncate(r.UserAgent()), truncate(r.PostFormValue("username")))
		}
	}

	// imitate a real credential check without burning CPU; bail out if the client leaves
	select {
	case <-time.After(time.Second + rand.N(time.Second)):
	case <-r.Context().Done():
		return
	}

	data := pageData{Company: companyName, Year: time.Now().Year(), Error: "Неверное имя пользователя или пароль."}
	if isHTMX(r) {
		s.render(w, s.admin, "login-error", http.StatusOK, data)
		return
	}
	s.render(w, s.admin, "admin", http.StatusOK, data)
}

func (s *Server) clientIP(r *http.Request) string {
	if s.TrustProxy {
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			return truncate(ip)
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			return truncate(strings.TrimSpace(first))
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// truncate caps a value before logging and keeps the result valid UTF-8.
func truncate(v string) string {
	if len(v) <= maxLogField {
		return v
	}
	return strings.ToValidUTF8(v[:maxLogField], "") + "…"
}
