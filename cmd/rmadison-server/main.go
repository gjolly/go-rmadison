package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gjolly/go-rmadisson/pkg/debian"
	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
)

var log *zap.SugaredLogger

func init() {
	// Logger for the operations
	logger, _ := zap.NewDevelopment()
	log = logger.Sugar()
}

type httpHandler struct {
	Cache *debian.Archive
}

func (h httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pkg := strings.TrimLeft(r.URL.Path, "/")
	log.Debugf("lookup for %v", pkg)

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

	log.Info("Reading local cache")
	_, packages, err := cache.RefreshCache(true)
	if err != nil {
		log.Error("error reading existing cache data:", err)
	}
	log.Infof("packages in cache: %v", packages)

	go func() {
		t := time.NewTicker(5 * time.Minute)
		for {
			now := time.Now()
			_, pkgStats, err := cache.RefreshCache(false)
			duration := time.Now().Sub(now)
			if err != nil {
				log.Errorf("cache refreshed in %v (with error %v), %v packages updated", duration.Seconds(), err, pkgStats)
			} else {
				log.Infof("cache refreshed in %v, %v packages updated", duration.Seconds(), pkgStats)
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
	log.Infof("starting http server on %v\n", addr)
	log.Fatal(s.ListenAndServe())
}
