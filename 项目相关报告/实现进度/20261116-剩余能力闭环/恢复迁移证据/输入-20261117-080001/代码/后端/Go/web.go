package jobsim

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) webHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.NotFound(w, r)
			return
		}
		for _, segment := range strings.Split(r.URL.Path, "/") {
			if strings.HasPrefix(segment, ".") {
				http.NotFound(w, r)
				return
			}
		}
		root, e := filepath.EvalSymlinks(a.Config.WebDir)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		root, e = filepath.Abs(root)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		name := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		info, e := os.Stat(name)
		if e != nil {
			if filepath.Ext(r.URL.Path) != "" {
				http.NotFound(w, r)
				return
			}
			name = filepath.Join(root, "index.html")
			info, e = os.Stat(name)
		} else if info.IsDir() {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			name = filepath.Join(root, "index.html")
			info, e = os.Stat(name)
		}
		if e != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		real, e := filepath.EvalSymlinks(name)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		relative, e := filepath.Rel(root, real)
		if e != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			http.NotFound(w, r)
			return
		}
		allowed := map[string]bool{".html": true, ".js": true, ".css": true, ".json": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true, ".ico": true, ".woff": true, ".woff2": true, ".ttf": true}
		if !allowed[strings.ToLower(filepath.Ext(real))] {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'")
		if filepath.Ext(real) == ".html" {
			w.Header().Set("Cache-Control", "no-store")
		}
		http.ServeFile(w, r, real)
	})
}
