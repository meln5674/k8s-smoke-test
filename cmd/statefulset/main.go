package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	flag "github.com/spf13/pflag"
)

var (
	rwxVolumeMount = flag.String("rwx-volume-mount", "/var/lib/k8s-smoke-test/rwx", "Path the RWX volume was mounted to")
	rwoVolumeMount = flag.String("rwo-volume-mount", "/var/lib/k8s-smoke-test/rwo", "Path the RWO volume was mounted to")
	listen         = flag.String("listen", "0.0.0.0:8080", "Address to listen on")
	deploymentURL  = flag.String("deployment-url", "http://k8s-smoke-test-deployment/health", "URL for the deployment to GET")
)

func handleRWOWriteRequest(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	path := filepath.Join(*rwoVolumeMount, req.URL.Path)
	relpath, err := filepath.Rel(*rwoVolumeMount, path)
	if err != nil {
		log.Print("Rejected possibly malicious path", req.URL.Path, ": ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	parts := filepath.SplitList(relpath)
	if parts[0] == ".." {
		log.Print("Rejected possibly malicious path", req.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = os.MkdirAll(filepath.Join(*rwoVolumeMount, filepath.Join(parts[0:len(parts)-1]...)), 0600)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	f, err := os.Create(filepath.Join(*rwoVolumeMount, relpath))
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer f.Close()
	_, err = io.Copy(f, req.Body)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func handleRWORequest(server http.Handler) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s", req.Method, req.URL.String())
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/rwo/")
		resp, err := http.Get(*deploymentURL)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if resp.StatusCode != http.StatusOK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch req.Method {
		case http.MethodGet:
			server.ServeHTTP(w, req)
			return
		case http.MethodPost:
			handleRWOWriteRequest(w, req)
			return
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}
}

func handleRWXRequest(server http.Handler) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s", req.Method, req.URL.String())
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/rwx/")
		resp, err := http.Get(*deploymentURL)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if resp.StatusCode != http.StatusOK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		server.ServeHTTP(w, req)
	}
}

func healthcheck(w http.ResponseWriter, req *http.Request) {
	log.Printf("%s %s", req.Method, req.URL.String())
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func main() {
	flag.Parse()

	http.HandleFunc("/health", healthcheck)
	http.HandleFunc("/rwx/", handleRWXRequest(http.FileServer(http.Dir(*rwxVolumeMount))))
	http.HandleFunc("/rwo/", handleRWORequest(http.FileServer(http.Dir(*rwoVolumeMount))))

	http.ListenAndServe(*listen, nil)
}
