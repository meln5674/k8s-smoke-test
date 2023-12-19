package main

import (
	"log"
	"net/http"
	"strings"

	flag "github.com/spf13/pflag"
)

var (
	rwxVolumeMount = flag.String("rwx-volume-mount", "/var/lib/k8s-smoke-test/rwx", "Path the RWX volume was mounted to")
	listen         = flag.String("listen", "0.0.0.0:8080", "Address to listen on")
	statefulSetURL = flag.String("statefulset-url", "http://k8s-smoke-test-0.k8s-smoke-test-statefulset:8080/health", "URL for the deployment to GET")
)

func handleRWXRequest(server http.Handler) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s", req.Method, req.URL.String())
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/rwx/")
		resp, err := http.Get(*statefulSetURL)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if resp.StatusCode != http.StatusOK {
			log.Println(err)
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

	http.ListenAndServe(*listen, nil)
}
