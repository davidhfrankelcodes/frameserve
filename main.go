package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"crypto/subtle"
)

//go:embed static/*
var staticFS embed.FS

type Photo struct {
	URL   string `json:"url"`
	Name  string `json:"name"`
	Mtime int64  `json:"mtime"`
	Size  int64  `json:"size"`
}

type PhotosResponse struct {
	Photos []Photo `json:"photos"`
	Count  int     `json:"count"`
}

const (
	authCookieName = "frameserve_auth"
	// 365 days. “Set it and forget it” while still having *some* bounded lifetime.
	authCookieMaxAgeSeconds = 365 * 24 * 60 * 60
)

func main() {
	port := getenv("PORT", "80")
	photosDir := getenv("PHOTOS_DIR", "/photos")

	// If AUTH_TOKEN is set, we enable auth for everything except /healthz.
	// Flow:
	//  - First visit: /?token=YOURTOKEN (or any path with token=...)
	//  - Server sets an HttpOnly cookie and redirects to the same URL without the token param.
	//  - Subsequent requests use the cookie.
	//
	// Also supports:
	//  - Authorization: Bearer YOURTOKEN
	authToken := strings.TrimSpace(os.Getenv("AUTH_TOKEN"))

	absPhotosDir, err := filepath.Abs(photosDir)
	if err != nil {
		log.Fatalf("failed to resolve PHOTOS_DIR: %v", err)
	}

	log.Printf("Frameserve starting: port=%s photos_dir=%s auth=%v", port, absPhotosDir, authToken != "")

	mux := http.NewServeMux()

	// Slideshow UI (no gallery)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		serveEmbeddedFile(w, r, "static/index.html", "text/html; charset=utf-8")
	})

	// Info page (how to use the site)
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveEmbeddedFile(w, r, "static/info.html", "text/html; charset=utf-8")
	})

	// Static assets
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		// Prevent directory listing; only serve embedded files
		path := strings.TrimPrefix(r.URL.Path, "/")
		if !strings.HasPrefix(path, "static/") {
			http.NotFound(w, r)
			return
		}
		serveEmbeddedFile(w, r, path, "")
	})

	// API: list photos
	mux.HandleFunc("/api/photos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		photos, err := scanPhotos(absPhotosDir)
		if err != nil {
			http.Error(w, "failed to scan photos directory", http.StatusInternalServerError)
			log.Printf("scan error: %v", err)
			return
		}

		// Optional ordering controls via query params:
		// ?order=mtime_desc|mtime_asc|name_asc|name_desc (default mtime_desc)
		order := r.URL.Query().Get("order")
		sortPhotos(photos, order)

		resp := PhotosResponse{Photos: photos, Count: len(photos)}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(resp)
	})

	// Serve individual photos safely
	mux.HandleFunc("/photos/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(r.URL.Path, "/photos/")
		if name == "" {
			http.NotFound(w, r)
			return
		}

		// Only allow file names (no subdirectories) to keep it simple + safe.
		if strings.Contains(name, "/") || strings.Contains(name, `\`) {
			http.NotFound(w, r)
			return
		}

		// Extension allowlist
		if !isAllowedExt(name) {
			http.NotFound(w, r)
			return
		}

		fullPath, err := safeJoin(absPhotosDir, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		fi, err := os.Stat(fullPath)
		if err != nil || fi.IsDir() {
			http.NotFound(w, r)
			return
		}

		// Content-Type best effort based on extension
		ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}

		// Cache images aggressively; list refresh handles new images.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		http.ServeFile(w, r, fullPath)
	})

	// Health check (left intentionally unauthenticated so health checks work cleanly)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	var handler http.Handler = mux
	handler = securityHeaders(handler)

	// Wrap with auth if AUTH_TOKEN is configured
	if authToken != "" {
		handler = authMiddleware(authToken, handler)
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}

func getenv(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func serveEmbeddedFile(w http.ResponseWriter, r *http.Request, path string, forcedContentType string) {
	b, err := staticFS.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if forcedContentType != "" {
		w.Header().Set("Content-Type", forcedContentType)
	} else {
		ext := strings.ToLower(filepath.Ext(path))
		if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set("Content-Type", ct)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
	}

	// Static assets can be cached
	if strings.HasPrefix(path, "static/") && path != "static/index.html" && path != "static/info.html" {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}

	_, _ = w.Write(b)
}

func scanPhotos(dir string) ([]Photo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var photos []Photo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isAllowedExt(name) {
			continue
		}

		fullPath, err := safeJoin(dir, name)
		if err != nil {
			continue
		}

		fi, err := os.Stat(fullPath)
		if err != nil || fi.IsDir() {
			continue
		}

		mtime := fi.ModTime().Unix()
		// Cache-bust param v=mtime so browsers refresh when a file changes.
		url := fmt.Sprintf("/photos/%s?v=%d", urlPathEscape(name), mtime)

		photos = append(photos, Photo{
			URL:   url,
			Name:  name,
			Mtime: mtime,
			Size:  fi.Size(),
		})
	}

	return photos, nil
}

func sortPhotos(photos []Photo, order string) {
	switch order {
	case "mtime_asc":
		sort.Slice(photos, func(i, j int) bool { return photos[i].Mtime < photos[j].Mtime })
	case "name_asc":
		sort.Slice(photos, func(i, j int) bool { return strings.ToLower(photos[i].Name) < strings.ToLower(photos[j].Name) })
	case "name_desc":
		sort.Slice(photos, func(i, j int) bool { return strings.ToLower(photos[i].Name) > strings.ToLower(photos[j].Name) })
	case "mtime_desc", "":
		fallthrough
	default:
		sort.Slice(photos, func(i, j int) bool { return photos[i].Mtime > photos[j].Mtime })
	}
}

func isAllowedExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func safeJoin(baseDir, fileName string) (string, error) {
	if fileName == "" {
		return "", errors.New("empty name")
	}
	clean := filepath.Clean(fileName)
	clean = filepath.Base(clean)

	joined := filepath.Join(baseDir, clean)

	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	joinedAbs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(baseAbs, joinedAbs)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", errors.New("path escapes base dir")
	}
	return joinedAbs, nil
}

func urlPathEscape(s string) string {
	repl := strings.NewReplacer(
		"%", "%25",
		" ", "%20",
		"#", "%23",
		"?", "%3F",
	)
	return repl.Replace(s)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		w.Header().Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"img-src 'self' data:",
			"style-src 'self'",
			"script-src 'self'",
		}, "; "))

		next.ServeHTTP(w, r)
	})
}

func stableHash(photos []Photo) string {
	h := sha256.New()
	for _, p := range photos {
		io.WriteString(h, p.Name)
		io.WriteString(h, ":")
		io.WriteString(h, strconv.FormatInt(p.Mtime, 10))
		io.WriteString(h, "\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ---- Auth (shared token) ----

func authMiddleware(token string, next http.Handler) http.Handler {
	want := []byte(token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let /healthz pass for infra health checks.
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		// If user provides token via query string once, set cookie then redirect.
		// Accept token=... or t=...
		q := r.URL.Query()
		if provided := firstNonEmpty(q.Get("token"), q.Get("t")); provided != "" {
			if constantTimeEqual(want, []byte(provided)) {
				setAuthCookie(w, r, token)

				// Redirect to same URL with token removed (so you can bookmark clean URLs later).
				cleanURL := *r.URL
				cq := cleanURL.Query()
				cq.Del("token")
				cq.Del("t")
				cleanURL.RawQuery = cq.Encode()

				http.Redirect(w, r, cleanURL.String(), http.StatusFound)
				return
			}
			// If they tried a token and it's wrong, fall through to unauthorized response.
		}

		// Cookie auth
		if c, err := r.Cookie(authCookieName); err == nil && c != nil {
			if constantTimeEqual(want, []byte(c.Value)) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Bearer token auth
		if bearer := parseBearer(r.Header.Get("Authorization")); bearer != "" {
			if constantTimeEqual(want, []byte(bearer)) {
				next.ServeHTTP(w, r)
				return
			}
		}

		unauthorized(w, r)
	})
}

func setAuthCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := isProbablyHTTPS(r)

	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   authCookieMaxAgeSeconds,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func unauthorized(w http.ResponseWriter, r *http.Request) {
	// Minimal, human-friendly response that works on TVs/kiosks.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)

	_, _ = io.WriteString(w, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>Frameserve - Unauthorized</title>
  <style>
    :root{color-scheme:dark}
    body{margin:0;padding:24px;background:#000;color:#fff;font-family:system-ui,-apple-system,Segoe UI,Roboto,Arial,sans-serif;line-height:1.5}
    code{font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono","Courier New",monospace}
    .card{max-width:920px;margin:0 auto;background:rgba(255,255,255,0.06);border:1px solid rgba(255,255,255,0.10);border-radius:14px;padding:16px 18px}
    a{color:#9ad1ff;text-decoration:none} a:hover{text-decoration:underline}
  </style>
</head>
<body>
  <div class="card">
    <h1>Unauthorized</h1>
    <p>This Frameserve instance requires a shared access token.</p>
    <p><strong>One-time setup on this device:</strong></p>
    <p>Open this URL once (replace <code>YOURTOKEN</code>):</p>
    <p><code>`+htmlEscape(r.URL.Path)+`?token=YOURTOKEN</code></p>
    <p>After that, the device will stay logged in via a long-lived cookie.</p>
    <p class="muted">If you cleared cookies or switched browsers, repeat the one-time setup.</p>
  </div>
</body>
</html>`)
}

func constantTimeEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

func parseBearer(authz string) string {
	authz = strings.TrimSpace(authz)
	if authz == "" {
		return ""
	}
	parts := strings.SplitN(authz, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if strings.ToLower(strings.TrimSpace(parts[0])) != "bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func isProbablyHTTPS(r *http.Request) bool {
	// Direct TLS
	if r.TLS != nil {
		return true
	}
	// Common reverse-proxy headers
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}

func htmlEscape(s string) string {
	repl := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return repl.Replace(s)
}
