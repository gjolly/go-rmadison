package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gjolly/go-rmadison/pkg/archive"
	"github.com/gjolly/go-rmadison/pkg/database"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	_ "github.com/mattn/go-sqlite3"
)

var log *zap.SugaredLogger

func init() {
	// Logger for the operations
	logger, _ := zap.NewDevelopment()
	log = logger.Sugar()
}

type httpHandler struct {
	Database *database.DB
}

func (h httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pkg := strings.TrimLeft(r.URL.Path, "/")
	log.Debugf("lookup for %v", pkg)

	if strings.Contains(pkg, "/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	allInfo, err := h.Database.GetPackage(pkg)
	if err != nil {
		log.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	jsonInfo, err := json.Marshal(allInfo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.Write(jsonInfo)
}

func refreshCaches(archives []*archive.Archive) {
	for _, cache := range archives {
		go func(cache *archive.Archive) {
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
		}(cache)
	}
}

// Config is the configuration of the rmadison server
type Config struct {
	Caches   []*archive.Archive
	Database *database.DB
}

type archiveYAMLConf struct {
	BaseURL  string   `yaml:"base_url"`
	PortsURL string   `yaml:"ports_url"`
	Pockets  []string `yaml:"pockets"`
}

func parseConfig() (*Config, error) {
	configPaths := []string{
		"server.yaml",
		"/etc/rmadison/server",
	}
	userConfigDir, err := os.UserConfigDir()
	if err == nil {
		configPaths = append(configPaths, path.Join(userConfigDir, "rmadison", "server.yaml"))
	}

	var configFile *os.File
	for _, configPath := range configPaths {
		configFile, err = os.Open(configPath)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("cannot find any config file in %v", configPaths)
	}

	configBytes, err := ioutil.ReadAll(configFile)
	if err != nil {
		return nil, err
	}
	rawConfig := new(struct {
		CacheDirectory string             `yaml:"cache_directory"`
		Database       string             `yaml:"database"`
		LogLevel       string             `yaml:"log_level"`
		Archives       []*archiveYAMLConf `yaml:"archives"`
	})
	yaml.Unmarshal(configBytes, rawConfig)
	conf := new(Config)
	conf.Caches = make([]*archive.Archive, len(rawConfig.Archives))

	httpClient := resty.New()

	db, err := database.NewConn("sqlite3", rawConfig.Database, log)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to database %v", rawConfig.Database)
	}
	conf.Database = db

	for i, archiveConf := range rawConfig.Archives {
		if archiveConf.BaseURL == "" {
			return nil, fmt.Errorf("missing base_url for archive %v", i)
		}

		baseURL, err := url.Parse(archiveConf.BaseURL)
		if err != nil {
			return nil, err
		}

		if archiveConf.PortsURL == "" {
			log.Infof("missing ports_url for archive %v, using base url", i)
			archiveConf.PortsURL = archiveConf.BaseURL
		}

		portsURL, err := url.Parse(archiveConf.PortsURL)
		if err != nil {
			return nil, err
		}
		conf.Caches[i] = &archive.Archive{
			BaseURL:  baseURL,
			PortsURL: portsURL,
			Pockets:  archiveConf.Pockets,
			CacheDir: rawConfig.CacheDirectory,
			Client:   httpClient,
			Database: db,
			Log:      log,
		}
	}

	return conf, err
}

func startPprofServer(addr string) {
	r := http.NewServeMux()

	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.HandleFunc("/debug/pprof/trace", pprof.Trace)

	s := &http.Server{
		Addr:    addr,
		Handler: r,
	}
	log.Infof("starting pprof server on %v\n", addr)
	log.Fatal(s.ListenAndServe())
}

func main() {
	go startPprofServer(":8434")

	flag.Parse()

	conf, err := parseConfig()
	if err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	if len(conf.Caches) == 0 {
		log.Fatal("No archive defined in config file")
	}

	refreshCaches(conf.Caches)
	handler := httpHandler{
		Database: conf.Database,
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
