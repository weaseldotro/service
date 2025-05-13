package service

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func LogRequest(req *http.Request) {
	if req.Header.Get("X-Real-IP") != "" {
		req.RemoteAddr = req.Header.Get("X-Real-IP")
	}
	ip, _, _ := net.SplitHostPort(req.RemoteAddr)
	slog.Info("HTTP request", "IP", ip, "method", req.Method, "URL", req.URL)
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(w, req)
		LogRequest(req)
	})
}

func SPAHandler(w http.ResponseWriter, r *http.Request) {
	// get the absolute path to prevent directory traversal
	path, err := filepath.Abs(r.URL.Path)
	if err != nil {
		// if we failed to get the absolute path respond with a 400 bad request
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// prepend the path with the path to the static directory
	path = filepath.Join(".", path)

	if path == "." {
		// handle root
		if _, err := os.Stat("index.html"); !os.IsNotExist(err) {
			http.ServeFile(w, r, "index.html")
			return
		}

		http.ServeFile(w, r, "./default.html")
		return
	}

	if _, err := os.Stat(path + ".html"); err == nil && path != "index" {
		http.ServeFile(w, r, path+".html")
		return
	}

	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		http.ServeFile(w, r, "./default.html")
		return
	}

	if fileInfo.IsDir() {
		if _, err := os.Stat(path + "/index.html"); os.IsNotExist(err) {
			http.ServeFile(w, r, "./default.html")
			return
		}

		http.ServeFile(w, r, path+"/index.html")
		return
	}

	if os.IsNotExist(err) {
		http.ServeFile(w, r, "./default.html")
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.ServeFile(w, r, path)
}
