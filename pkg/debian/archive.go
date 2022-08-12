package debian

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
)

// ReleaseFileEntry is a entry in a release file
type ReleaseFileEntry struct {
	Hash string
	Size uint
	Path string
}

// ReleaseFile is a debian archive release file
type ReleaseFile struct {
	Origin        string
	Label         string
	Suite         string
	Version       string
	Codename      string
	Date          time.Time
	Architectures []string
	Components    []string
	Description   string
	Sha256        map[string]ReleaseFileEntry
}

// Archive is a debian archive
type Archive struct {
	BaseURL     url.URL
	PortsURL    url.URL
	Client      *resty.Client
	ReleaseInfo map[string]*ReleaseFile
	Pockets     []string
	CacheDir    string
	Packages    map[string]map[string]*PackageInfo
}

// DownloadReleaseFile downloads a release file for the
// given pocket
func (a *Archive) downloadReleaseFile(pocket string) ([]byte, error) {
	fileURL := url.URL(a.BaseURL)
	fileURL.Path = path.Join(fileURL.Path, pocket, "InRelease")

	resp, err := a.Client.R().
		Get(fileURL.String())
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, errors.Wrapf(err, "failed to fetch Release file from %v (%v)", fileURL, resp.Status())
	}

	return resp.Body(), nil
}

// GetReleaseInfo downloads all the release files for the pockets and parses them
func (a *Archive) GetReleaseInfo() (map[string]*ReleaseFile, error) {
	releaseInfo := make(map[string]*ReleaseFile)
	for _, pocket := range a.Pockets {
		releaseFile, err := a.downloadReleaseFile(pocket)
		if err != nil {
			return nil, err
		}
		releaseInfo[pocket], err = ParseReleaseFile(releaseFile)

		if err != nil {
			return nil, err
		}
	}

	return releaseInfo, nil
}

// ParseReleaseFile parses the content of a release file
func ParseReleaseFile(raw []byte) (*ReleaseFile, error) {
	releaseFile := new(ReleaseFile)

	txtFile := fmt.Sprintf("%s", raw)
	lines := strings.Split(txtFile, "\n")

	for iLine := 0; iLine < len(lines); iLine++ {
		line := lines[iLine]
		if strings.Contains(line, "BEGIN PGP SIGNATURE") {
			// Go doesn't support OpenPGP https://github.com/golang/go/issues/44226
			// Use modern crypto tools, we are not in 2000 anymore.
			break
		}
		keyValue := strings.Split(line, ": ")
		if len(keyValue) == 2 {
			key := keyValue[0]
			value := keyValue[1]

			v := reflect.Indirect(reflect.ValueOf(releaseFile))
			field := v.FieldByName(key)
			if field == (reflect.Value{}) {
				// we don't know about this field
				continue
			}

			if key == "Date" {
				date, err := time.Parse(time.RFC1123Z, value)
				if err != nil {
					continue
				}
				field.Set(reflect.ValueOf(date))

				continue
			}
			if key == "Architectures" || key == "Components" {
				slice := strings.Split(value, " ")
				field.Set(reflect.ValueOf(slice))

				continue
			}

			field.SetString(keyValue[1])
		}

		if strings.HasPrefix(line, "MD5Sum") {
			releaseFile.Sha256 = make(map[string]ReleaseFileEntry, 0)
			iLine++
			for iLine < len(lines) {
				line = lines[iLine]
				lineElmt := strings.Fields(line)

				// we reached the end of the file list
				if len(lineElmt) != 3 {
					break
				}
				size, err := strconv.Atoi(lineElmt[1])
				if err != nil {
					return nil, err
				}
				fileEntry := ReleaseFileEntry{
					Hash: lineElmt[0],
					Size: uint(size),
					Path: lineElmt[2],
				}
				releaseFile.Sha256[fileEntry.Path] = fileEntry

				iLine++
			}
		}
	}

	return releaseFile, nil
}

func downlaodFile(client *resty.Client, fileURL url.URL, outputFilePath string) error {
	resp, err := client.R().
		SetOutput(outputFilePath).
		Get(fileURL.String())
	if err != nil {
		return errors.Wrap(err, "failed to fetch Release file")
	}

	if resp.IsError() {
		return fmt.Errorf("failed to fetch Release file from %v (%v)", fileURL, resp.Status())
	}

	return nil
}

// DownloadIfNeeded downloads the package index files for the given pocket
// if the hashes from filesToDownload are direrent from the ones in a.ReleaseInfo
// returns the number of files downloaded
func (a *Archive) DownloadIfNeeded(pocket string, filesToDownload map[string]ReleaseFileEntry, packagesChan chan *PackageInfo) (int, error) {
	pocketBaseURL := url.URL(a.BaseURL)
	pocketBaseURL.Path = path.Join(pocketBaseURL.Path, pocket)

	pocketPortsURL := url.URL(a.PortsURL)
	pocketPortsURL.Path = path.Join(pocketPortsURL.Path, pocket)

	nbFile := 0
	wg := new(sync.WaitGroup)
	for filePath, fileInfo := range filesToDownload {
		if a.ReleaseInfo != nil && fileInfo.Hash == a.ReleaseInfo[pocket].Sha256[filePath].Hash {
			continue
		}
		nbFile++
		fileURL := url.URL(pocketPortsURL)
		if strings.Contains(filePath, "amd64") || strings.Contains(filePath, "i386") {
			fileURL = url.URL(pocketBaseURL)
		}
		fileURL.Path = path.Join(fileURL.Path, filePath)

		outputFileName := strings.ReplaceAll(fileURL.Hostname()+fileURL.Path, "/", "_")

		wg.Add(1)
		go func(fileURL url.URL, fileName string) {
			defer wg.Done()
			filePath := path.Join(a.CacheDir, fileName)
			err := downlaodFile(a.Client, fileURL, filePath)
			if err != nil {
				log.Println(err)
				return
			}

			err = a.parsePackageIndex(packagesChan, fileName)
			if err != nil {
				log.Println(err)
			}
		}(fileURL, outputFileName)
	}

	wg.Wait()

	return nbFile, nil
}

func (a *Archive) refreshCacheForPocket(pocket string, releaseInfo map[string]ReleaseFileEntry, packagesChan chan *PackageInfo) (int, error) {
	filesToDownload := make(map[string]ReleaseFileEntry)

	for filePath, info := range releaseInfo {
		if strings.Contains(filePath, "Packages.gz") && !strings.Contains(filePath, "installer") {
			filesToDownload[filePath] = info
		}
	}

	nbFile, err := a.DownloadIfNeeded(pocket, filesToDownload, packagesChan)
	if err != nil {
		return nbFile, err
	}
	return nbFile, nil
}

// RefreshCache checks if the archive indexes have changed and
// redownload them if needed
func (a *Archive) RefreshCache() (int, int, error) {
	newInfo, err := a.GetReleaseInfo()
	if err != nil {
		return 0, 0, err
	}

	totalNbFile := 0

	packages := make(chan *PackageInfo, 100)
	wg := new(sync.WaitGroup)
	for _, pocket := range a.Pockets {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			nbFile, err := a.refreshCacheForPocket(p, newInfo[p].Sha256, packages)
			if err != nil {
				log.Println(err)
				return
			}
			totalNbFile += nbFile
		}(pocket)
	}

	done := make(chan struct{})
	stats := make(chan int)
	go a.updatePackageInfo(packages, done, stats)

	wg.Wait()
	done <- struct{}{}

	a.ReleaseInfo = newInfo

	return totalNbFile, <-stats, nil
}

func (a *Archive) listFilesInCache(filter string) ([]string, error) {
	files, err := ioutil.ReadDir(a.CacheDir)
	if err != nil {
		return nil, err
	}

	filteredFiles := make([]string, 0)

	re := regexp.MustCompile(filter)
	for _, file := range files {
		if re.MatchString(file.Name()) {
			filteredFiles = append(filteredFiles, file.Name())
		}
	}

	return filteredFiles, nil
}

func uncompressFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	gzipReader, err := gzip.NewReader(file)
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

func parsePackageIndexFile(out chan *PackageInfo, rawBody, suite, pocket, component, arch string) error {
	packageInfo := strings.Split(rawBody, "\n\n")

	for _, info := range packageInfo {
		infoLines := strings.Split(info, "\n")
		pkgName := ""

		var pkgInfo *PackageInfo

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
				pkgInfo = &PackageInfo{
					Name:         pkgName,
					Component:    component,
					Suite:        suite,
					Pocket:       pocket,
					Architecture: arch,
				}
			}
			if key == "Version" {
				pkgInfo.Version = value
			}
		}

		if pkgInfo != nil {
			out <- pkgInfo
		}
	}

	return nil
}

func getInfoFromIndexName(name string) (string, string, string, string, error) {
	meaningfulName := strings.Split(name, "dists_")
	if len(meaningfulName) != 2 {
		return "", "", "", "", fmt.Errorf("%v doesn't contain 'dists_'", meaningfulName)
	}

	parts := strings.Split(meaningfulName[1], "_")

	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("%v doesn't contain 4 parts", meaningfulName[1])
	}
	suitePocket := parts[0]
	component := parts[1]
	binaryArch := parts[2]

	suitePocketList := strings.Split(suitePocket, "-")
	suite := suitePocketList[0]
	pocket := ""
	if len(suitePocketList) == 2 {
		pocket = "-" + suitePocketList[1]
	}
	arch := strings.Split(binaryArch, "-")[1]

	return suite, pocket, component, arch, nil
}

func (a *Archive) parsePackageIndex(out chan *PackageInfo, file string) error {
	filePath := path.Join(a.CacheDir, file)
	textFile, err := uncompressFile(filePath)
	if err != nil {
		return err
	}

	suite, pocket, component, arch, err := getInfoFromIndexName(file)
	if err != nil {
		return err
	}

	return parsePackageIndexFile(out, textFile, suite, pocket, component, arch)
}

func (a *Archive) updatePackageInfo(packages chan *PackageInfo, done chan struct{}, stats chan int) {
	insertedPkg := 0
	if a.Packages == nil {
		a.Packages = make(map[string]map[string]*PackageInfo)
	}

	for {
		select {
		case pkg := <-packages:
			if _, ok := a.Packages[pkg.Name]; !ok {
				a.Packages[pkg.Name] = make(map[string]*PackageInfo, 0)
			}

			key := fmt.Sprintf("%v%v_%v_%v", pkg.Suite, pkg.Pocket, pkg.Component, pkg.Architecture)
			insertedPkg++
			a.Packages[pkg.Name][key] = pkg
		case <-done:
			stats <- insertedPkg
			return
		}
	}
}

// InitCache reads the files on disk and parese them
// Using this function avoid redownloading the entire cache from
// the archive when the apps start.
func (a *Archive) InitCache() (int, error) {
	files, err := os.ReadDir(a.CacheDir)
	if err != nil {
		return 0, err
	}

	if len(files) == 0 {
		return 0, nil
	}

	packagesChan := make(chan *PackageInfo, 100)
	done := make(chan struct{})
	stats := make(chan int)
	go a.updatePackageInfo(packagesChan, done, stats)

	wg := new(sync.WaitGroup)
	for _, file := range files {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			err := a.parsePackageIndex(packagesChan, path)
			if err != nil {
				log.Println(err)
			}
		}(file.Name())
	}
	wg.Wait()
	done <- struct{}{}

	return <-stats, nil
}
