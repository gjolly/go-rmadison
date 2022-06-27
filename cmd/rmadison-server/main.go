package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gjolly/go-rmadisson/pkg/debian"
	"github.com/go-resty/resty/v2"
)

func downloadRawIndex(client *resty.Client, series, pocket, component, arch string) (string, error) {
	archive := "http://archive.ubuntu.com/ubuntu"
	if arch != "amd64" && arch != "i386" {
		archive = "http://ports.ubuntu.com/ubuntu-ports"
	}
	url := fmt.Sprintf(
		"%v/dists/%v%v/%v/binary-%v/Packages.gz",
		archive,
		series,
		pocket,
		component,
		arch,
	)
	resp, err := client.R().
		SetDoNotParseResponse(true).
		Get(url)
	if err != nil {
		return "", err
	}

	body := resp.RawBody()
	gzipReader, err := gzip.NewReader(body)
	if err != nil {
		return "", err
	}
	defer gzipReader.Close()

	result, err := ioutil.ReadAll(gzipReader)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s", result), nil
}

func getIndex(client *resty.Client, pkgInfoChan chan *debian.PackageInfo, series, pocket, component, arch string) error {
	rawIndex, err := downloadRawIndex(client, series, pocket, component, arch)
	if err != nil {
		return err
	}

	packageInfo := strings.Split(rawIndex, "\n\n")

	for _, info := range packageInfo {
		infoLines := strings.Split(info, "\n")
		pkgName := ""

		var pkgInfo *debian.PackageInfo

		for _, line := range infoLines {
			cleanLine := strings.Trim(line, "\n")
			if cleanLine == "" {
				continue
			}

			rawAttribute := strings.Split(cleanLine, ": ")

			if len(rawAttribute) < 2 {
				//fmt.Printf("failed to parse property: %#v\n", rawAttribute)
				continue
			}
			key := rawAttribute[0]
			value := strings.TrimPrefix(cleanLine, fmt.Sprintf("%v: ", key))

			// here we assume that we see Package before any other field
			// it makes sense but it's also not very safe
			if key == "Package" {
				pkgName = value
				pkgInfo = &debian.PackageInfo{
					Name:         pkgName,
					Component:    component,
					Suite:        series,
					Pocket:       pocket,
					Architecture: arch,
				}
			}
			if key == "Version" {
				pkgInfo.Version = value
			}
		}

		if pkgInfo != nil {
			pkgInfoChan <- pkgInfo
		}
	}

	return nil
}

// Cache is a key value store
type Cache struct {
	Packages map[string][]*debian.PackageInfo
}

func (c *Cache) updateCache() {
	client := resty.New()
	client.SetRetryCount(3).
		SetRetryWaitTime(5 * time.Second).
		SetRetryMaxWaitTime(20 * time.Second)

	pockets := []string{"", "-updates", "-proposed"}
	components := []string{"main", "universe", "multiverse"}
	series := []string{"bionic", "focal", "impish", "jammy", "kinetic"}
	archs := []string{"amd64", "arm64"}

	resultChan := make(chan *debian.PackageInfo, 1000)
	done := make(chan struct{})
	wg := new(sync.WaitGroup)

	for _, suite := range series {
		for _, component := range components {
			for _, pocket := range pockets {
				for _, arch := range archs {
					wg.Add(1)
					go func(suite, pocket, component, arch string) {
						defer wg.Done()
						err := getIndex(client, resultChan, suite, pocket, component, arch)
						if err != nil {
							log.Println(err)
							return
						}
					}(suite, pocket, component, arch)
				}
			}
		}
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	for {
		select {
		case info := <-resultChan:
			if _, exist := c.Packages[info.Name]; !exist {
				c.Packages[info.Name] = []*debian.PackageInfo{info}
				continue
			}

			updated := false
			for _, currentInfo := range c.Packages[info.Name] {
				if currentInfo.Component == info.Component && currentInfo.Suite == info.Suite && currentInfo.Pocket == info.Pocket && currentInfo.Architecture == info.Architecture {
					currentInfo.Version = info.Version

					updated = true
					break
				}
			}
			if !updated {
				c.Packages[info.Name] = append(c.Packages[info.Name], info)
			}
		case <-done:
			if len(resultChan) == 0 {
				return
			}
		}
	}
}

type httpHandler struct {
	Cache *Cache
}

func (h httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pkg := strings.TrimLeft(r.URL.Path, "/")
	log.Printf("lookup for %v", pkg)

	if strings.Contains(pkg, "/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	info, ok := h.Cache.Packages[pkg]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	jsonInfo, err := json.Marshal(info)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.Write(jsonInfo)
}

func main() {
	cache := new(Cache)
	cache.Packages = make(map[string][]*debian.PackageInfo)

	go func() {
		t := time.NewTicker(15 * time.Minute)
		for {
			cache.updateCache()
			log.Println("cache updated")
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
