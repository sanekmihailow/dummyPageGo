package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{LogAttempts: false}
	require.NoError(t, s.parseTemplates())
	return s
}

func do(t *testing.T, s *Server, method, target string, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, r)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	res := w.Result()
	out, _ := io.ReadAll(res.Body)
	return res, string(out)
}

func TestSitePages(t *testing.T) {
	s := newTestServer(t)
	for _, p := range []string{"/", "/about", "/services", "/contacts"} {
		res, body := do(t, s, http.MethodGet, p, "", nil)
		assert.Equal(t, http.StatusOK, res.StatusCode, p)
		assert.Equal(t, "nginx", res.Header.Get("Server"), p)
		assert.Contains(t, body, companyName, p)
	}
}

func TestNotFound(t *testing.T) {
	s := newTestServer(t)
	res, body := do(t, s, http.MethodGet, "/nope", "", nil)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
	assert.Contains(t, body, "404")
}

func TestContactSubmitHTMXReturnsFragment(t *testing.T) {
	s := newTestServer(t)
	res, body := do(t, s, http.MethodPost, "/contacts", "name=x&email=a@b.c", map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, "Спасибо")
	assert.NotContains(t, body, "<html", "HTMX response must be a fragment, not a full page")
}

func TestAdminPageRenders(t *testing.T) {
	s := newTestServer(t)
	res, body := do(t, s, http.MethodGet, "/admin", "", nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, "Вход в панель управления")
}

func TestAdminLoginAlwaysFails(t *testing.T) {
	s := newTestServer(t)
	res, body := do(t, s, http.MethodPost, "/admin/login",
		"username=admin&password=admin", map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, "Неверное имя пользователя или пароль")
	assert.NotContains(t, strings.ToLower(body), "set-cookie")
	assert.Empty(t, res.Header.Get("Set-Cookie"), "no session must ever be issued")
}

// TestAdminLoginNeverAuthenticates is the core guarantee: no combination of credentials,
// in either the htmx or the plain-form flow, may ever produce a session, a redirect, or
// anything other than the rejection page. Subtests run in parallel so the built-in login
// delay does not accumulate.
func TestAdminLoginNeverAuthenticates(t *testing.T) {
	s := newTestServer(t)

	creds := []string{
		"username=admin&password=admin",
		"username=admin&password=password",
		"username=root&password=toor",
		"username=administrator&password=123456",
		"username=admin&password=", // empty password
		"username=&password=",      // both empty
		"username=admin'--&password=x",
		"username=admin&password=' OR '1'='1", // SQL-injection-ish
		"username=" + strings.Repeat("A", 500) + "&password=" + strings.Repeat("B", 500),
	}

	for _, htmx := range []bool{true, false} {
		for _, body := range creds {
			t.Run("", func(t *testing.T) {
				t.Parallel()
				var hdr map[string]string
				if htmx {
					hdr = map[string]string{"HX-Request": "true"}
				}
				res, out := do(t, s, http.MethodPost, "/admin/login", body, hdr)

				// always 200 (never a 3xx redirect into some "authenticated" area)
				assert.Equal(t, http.StatusOK, res.StatusCode, body)
				assert.Less(t, res.StatusCode, 300, "no redirect may ever happen: %s", body)
				// no session is ever issued
				assert.Empty(t, res.Header.Get("Set-Cookie"), "no Set-Cookie allowed: %s", body)
				// the response is the rejection, not any logged-in content
				assert.Contains(t, out, "Неверное имя пользователя или пароль", body)
			})
		}
	}
}

func TestMaintenanceMode(t *testing.T) {
	s := &Server{Maintenance: true}
	require.NoError(t, s.parseTemplates())

	// public pages are replaced by the under-development notice with a 503 status
	for _, p := range []string{"/", "/about", "/services", "/contacts", "/whatever"} {
		res, body := do(t, s, http.MethodGet, p, "", nil)
		assert.Equal(t, http.StatusServiceUnavailable, res.StatusCode, p)
		assert.Contains(t, body, "в разработке", p)
	}

	// static assets still load so the page keeps its styling
	res, _ := do(t, s, http.MethodGet, "/static/style.css", "", nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	// /admin stays available even in maintenance mode: the login form is served...
	res, body := do(t, s, http.MethodGet, "/admin", "", nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, "Вход в панель управления")
	assert.NotContains(t, body, "в разработке")

	// ...and it still never authenticates
	res, body = do(t, s, http.MethodPost, "/admin/login",
		"username=admin&password=admin", map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, "Неверное имя пользователя или пароль")
}

func TestClientIPTrustProxy(t *testing.T) {
	s := &Server{TrustProxy: true}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	assert.Equal(t, "203.0.113.7", s.clientIP(req))

	s.TrustProxy = false
	req.RemoteAddr = "198.51.100.4:5555"
	assert.Equal(t, "198.51.100.4", s.clientIP(req))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "short", truncate("short"))
	long := strings.Repeat("a", maxLogField+10)
	got := truncate(long)
	assert.Equal(t, maxLogField+len("…"), len(got))
	assert.True(t, strings.HasSuffix(got, "…"))
}
