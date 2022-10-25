package archive

import (
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gjolly/go-rmadison/pkg/database"
	"github.com/gjolly/go-rmadison/pkg/debianpkg"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var log *zap.SugaredLogger

func init() {
	// Logger for the operations
	logger, _ := zap.NewDevelopment()
	log = logger.Sugar()
}

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
	PackageIndex  map[string]ReleaseFileEntry
	Hash          string
}

// Archive is a debian archive
type Archive struct {
	BaseURL     *url.URL
	PortsURL    *url.URL
	Client      *resty.Client
	ReleaseInfo map[string]*ReleaseFile
	Pockets     []string
	CacheDir    string
	Database    *database.DB
	DBPath      string
}

func (a *Archive) getReleaseFileLocationsForPocket(pocket string) (url.URL, string) {
	fileURL := url.URL(*a.BaseURL)
	fileURL.Path = path.Join(fileURL.Path, pocket, "InRelease")
	outputFileName := strings.ReplaceAll(fileURL.Hostname()+fileURL.Path, "/", "_")

	outputFilePath := path.Join(a.CacheDir, outputFileName)

	return fileURL, outputFilePath
}

// GetReleaseInfo downloads all the release files for the pockets and parses them
func (a *Archive) GetReleaseInfo(local bool) (map[string]*ReleaseFile, error) {
	releaseInfo := make(map[string]*ReleaseFile)
	for _, pocket := range a.Pockets {
		fileURL, outputFilePath := a.getReleaseFileLocationsForPocket(pocket)

		file, err := os.Open(outputFilePath)
		if err != nil || !local {
			log.Debugf("[release] fetching %v", outputFilePath)
			err := downloadFile(a.Client, fileURL, outputFilePath)
			if err != nil {
				return nil, err
			}

			file, err = os.Open(outputFilePath)
			if err != nil {
				return nil, err
			}
		} else {
			log.Debugf("[release] local %v", outputFilePath)
		}
		defer file.Close()

		shaSum := sha256.New()
		if _, err := io.Copy(shaSum, file); err != nil {
			return nil, fmt.Errorf("failed to compute hash for %v", outputFilePath)
		}
		shaSumStr := fmt.Sprintf("%x", shaSum)

		// If the index file hasn't changed, let's not re-parse it
		if releaseFile, ok := a.ReleaseInfo[pocket]; ok && shaSumStr == releaseFile.Hash {
			log.Debugf("[release] nothing to do %v", outputFilePath)
			continue
		}

		log.Debugf("[release] parsing %v", outputFilePath)
		releaseInfo[pocket], err = ParseReleaseFile(file)
		if err != nil {
			log.Errorf("failed to parse Release file (%v): %v", outputFilePath, err)
			continue
		}
		releaseInfo[pocket].Hash = shaSumStr

		if err != nil {
			return nil, err
		}
	}

	return releaseInfo, nil
}

func parseIndexLine(line string) *ReleaseFileEntry {
	lineElmt := strings.Fields(line)

	// we reached the end of the file list
	if len(lineElmt) != 3 {
		return nil
	}

	size, err := strconv.Atoi(lineElmt[1])
	if err != nil {
		return nil
	}

	return &ReleaseFileEntry{
		Hash: lineElmt[0],
		Size: uint(size),
		Path: lineElmt[2],
	}
}

// ParseReleaseFile parses the content of a release file
func ParseReleaseFile(file *os.File) (*ReleaseFile, error) {
	file.Seek(0, 0)
	raw, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

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

		if strings.HasPrefix(line, "SHA256") {
			releaseFile.PackageIndex = make(map[string]ReleaseFileEntry, 0)
			iLine++
			for iLine < len(lines) {
				line = lines[iLine]
				fileEntry := parseIndexLine(line)
				// we reached the end of the file list
				if fileEntry == nil {
					break
				}
				releaseFile.PackageIndex[fileEntry.Path] = *fileEntry

				iLine++
			}
		}
	}

	return releaseFile, nil
}

func downloadFile(client *resty.Client, fileURL url.URL, outputFilePath string) error {
	resp, err := client.
		SetRetryCount(3).
		SetRetryWaitTime(5 * time.Second).
		SetRetryMaxWaitTime(20 * time.Second).
		R().
		SetOutput(outputFilePath).
		Get(fileURL.String())
	if err != nil {
		return errors.Wrap(err, "failed to fetch Release file")
	}

	if resp.IsError() {
		return fmt.Errorf("failed to fetch file from %v (%v)", fileURL, resp.Status())
	}

	return nil
}

// DownloadIfNeeded downloads the package index files for the given pocket
// if the hashes from filesToDownload are direrent from the ones in a.ReleaseInfo
// returns the number of files downloaded
func (a *Archive) DownloadIfNeeded(local bool, pocket string, filesToDownload map[string]ReleaseFileEntry, packagesChan chan *debianpkg.PackageInfo) (int, error) {
	pocketBaseURL := url.URL(*a.BaseURL)
	pocketBaseURL.Path = path.Join(pocketBaseURL.Path, pocket)

	pocketPortsURL := url.URL(*a.PortsURL)
	pocketPortsURL.Path = path.Join(pocketPortsURL.Path, pocket)

	nbFile := 0
	wg := new(sync.WaitGroup)
	for filePath, fileInfo := range filesToDownload {
		if a.ReleaseInfo != nil {
			if _, ok := a.ReleaseInfo[pocket]; ok && fileInfo.Hash == a.ReleaseInfo[pocket].PackageIndex[filePath].Hash {
				continue
			}
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
			if _, err := os.Stat(filePath); !local || errors.Is(err, os.ErrNotExist) {
				err := downloadFile(a.Client, fileURL, filePath)
				if err != nil {
					log.Errorf("error downloading: %v: %v", fileURL.String(), err)
					return
				}
				log.Debugf("[package][%v] Downloaded %v", pocket, filePath)
			}

			err := a.parsePackageIndex(packagesChan, fileName)
			if err != nil {
				log.Errorf("failed to parse package index %v: %v", fileName, err)
			}
		}(fileURL, outputFileName)
	}

	wg.Wait()

	return nbFile, nil
}

func (a *Archive) refreshCacheForPocket(local bool, pocket string, releaseInfo map[string]ReleaseFileEntry, packagesChan chan *debianpkg.PackageInfo) (int, error) {
	filesToDownload := make(map[string]ReleaseFileEntry)

	for filePath, info := range releaseInfo {
		if strings.Contains(filePath, "Packages.gz") && !strings.Contains(filePath, "installer") {
			filesToDownload[filePath] = info
		}
	}

	nbFile, err := a.DownloadIfNeeded(local, pocket, filesToDownload, packagesChan)
	if err != nil {
		return nbFile, err
	}
	return nbFile, nil
}

// RefreshCache checks if the archive indexes have changed and
// redownload them if needed
func (a *Archive) RefreshCache(local bool) (int, int, error) {
	newInfo, err := a.GetReleaseInfo(local)
	if err != nil {
		return 0, 0, err
	}
	log.Debug("[release] finished processing release indexes")

	totalNbFile := 0

	packages := make(chan *debianpkg.PackageInfo, 1000)
	wg := new(sync.WaitGroup)
	for _, pocket := range a.Pockets {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			var (
				nbFile int
				err    error
			)

			// if newInfo[p] does not exists, it means it hasn't changed, there
			// is nothing to refresh
			if _, ok := newInfo[p]; !ok {
				return
			}

			nbFile, err = a.refreshCacheForPocket(local, p, newInfo[p].PackageIndex, packages)
			log.Debugf("[packages][%v] refreshed", p)
			if err != nil {
				log.Error(err)
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
	defer file.Close()
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

func parsePackageIndexFile(out chan *debianpkg.PackageInfo, rawBody, suite, pocket, component, arch string) error {
	packageInfo := strings.Split(rawBody, "\n\n")

	for _, info := range packageInfo {
		infoLines := strings.Split(info, "\n")
		pkgName := ""

		var pkgInfo *debianpkg.PackageInfo

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
			key := strings.Clone(rawAttribute[0])
			value := strings.Clone(strings.TrimPrefix(cleanLine, fmt.Sprintf("%v: ", key)))

			// here we assume that we see Package before any other field
			// it makes sense but it's also not very safe
			if key == "Package" {
				pkgName = value
				pkgInfo = &debianpkg.PackageInfo{
					Name:         pkgName,
					Component:    component,
					Suite:        suite,
					Pocket:       pocket,
					Architecture: arch,
				}
			}
			if pkgInfo != nil {
				err := pkgInfo.Set(key, value)
				if err != nil {
					log.Debugf("[package] error reading maintainer info (%v): %v", pkgInfo.Name, err)
				}
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

func (a *Archive) parsePackageIndex(out chan *debianpkg.PackageInfo, file string) error {
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

func (a *Archive) updatePackageInfo(packages chan *debianpkg.PackageInfo, done chan struct{}, stats chan int) {
	insertedPkg := 0

	for {
		select {
		case pkg := <-packages:
			err := a.Database.PrepareInsertPackage(pkg)

			insertedPkg++

			if err != nil {
				log.Errorf("failed to insert package %v in db: %v", pkg.Name, err)
			}
			if insertedPkg%10000 == 0 {
				log.Debugf("Inserted %v packages", insertedPkg)
				err := a.Database.InsertPrepared()
				if err != nil {
					log.Errorf("transaction failed: %v", err)
				}
			}
		case <-done:
			err := a.Database.InsertPrepared()
			if err != nil {
				log.Errorf("transaction failed: %v", err)
			}
			stats <- insertedPkg
			return
		}
	}
}
