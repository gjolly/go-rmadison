package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gjolly/go-rmadisson/pkg/debian"
	"github.com/go-resty/resty/v2"
)

type httpHandler struct {
	Cache *debian.Archive
}

func (h httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pkg := strings.TrimLeft(r.URL.Path, "/")
	log.Printf("lookup for %v", pkg)

	if strings.Contains(pkg, "/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	allInfo, ok := h.Cache.Packages[pkg]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// convert the dictionary to a list
	fmtInfo := make([]*debian.PackageInfo, len(allInfo))
	i := 0
	for _, info := range allInfo {
		fmtInfo[i] = info
		i++
	}

	jsonInfo, err := json.Marshal(fmtInfo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.Write(jsonInfo)
}

func main() {
	flag.Parse()
	cacheDir := flag.Arg(0)
	if cacheDir == "" {
		cacheDir, _ = os.MkdirTemp("", "gormadisontest")
	}

	baseURL, _ := url.Parse("http://archive.ubuntu.com/ubuntu/dists")
	portsURL, _ := url.Parse("http://ports.ubuntu.com/dists")

	client := resty.New()
	cache := &debian.Archive{
		BaseURL:  *baseURL,
		PortsURL: *portsURL,
		Client:   client,
		Pockets: []string{
			"bionic", "bionic-updates",
			"focal", "focal-updates",
			"jammy", "jammy-updates",
			"kinetic",
		},
		CacheDir: cacheDir,
	}

	packages, err := cache.InitCache()
	if err != nil {
		log.Println("error reading existing cache data", err)
	}
	log.Println("packages in cache:", packages)

	go func() {
		t := time.NewTicker(5 * time.Minute)
		for {
			now := time.Now()
			_, pkgStats, err := cache.RefreshCache()
			duration := time.Now().Sub(now)
			if err != nil {
				log.Printf("cache refreshed (with error '%v') in %v, %v packages in db", err, duration.Seconds(), pkgStats)
			} else {
				log.Printf("cache refreshed in %v, %v packages in db", duration.Seconds(), pkgStats)
			}

			<-t.C
		}
	}()

	handler := httpHandler{
		Cache: cache,
	}
	addr := ":8433"
	s := &http.Server{
		Addr:           addr,
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Printf("starting http server on %v\n", addr)
	log.Fatal(s.ListenAndServe())
}
