package debianpkg

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PackageMaintainer describes a package maintainer
type PackageMaintainer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// PackageInfo holds the metadata for a debian package
type PackageInfo struct {
	Name          string             `json:"name"`
	Version       string             `json:"version"`
	Component     string             `json:"component"`
	Suite         string             `json:"suite"`
	Pocket        string             `json:"pocket"`
	Architecture  string             `json:"architecture"`
	Source        string             `json:"source"`
	Section       string             `json:"section"`
	Maintainer    *PackageMaintainer `json:"maintainer"`
	SHA256        string             `json:"sha256"`
	Size          int                `json:"size"`
	InstalledSize int                `json:"installed-size"`
	FileName      string             `json:"filename"`
	Replaces      []string           `json:"replaces"`
	Conflicts     []string           `json:"conflicts"`
	Suggests      []string           `json:"suggests"`
	Description   string             `json:"description"`
}

// Set sets a field on the object
func (pkgInfo *PackageInfo) Set(key, value string) error {
	if key == "Version" {
		pkgInfo.Version = value
		return nil
	}
	if key == "Source" {
		pkgInfo.Source = value
		return nil
	}
	if key == "Section" {
		pkgInfo.Section = value
		return nil
	}
	if key == "Size" {
		pkgInfo.Size, _ = strconv.Atoi(value)
		return nil
	}
	if key == "Installed-Size" {
		pkgInfo.InstalledSize, _ = strconv.Atoi(value)
		return nil
	}
	if key == "Conflicts" {
		pkgInfo.Conflicts = strings.Split(value, ", ")
		return nil
	}
	if key == "Replaces" {
		pkgInfo.Replaces = strings.Split(value, ", ")
		return nil
	}
	if key == "Suggests" {
		pkgInfo.Suggests = strings.Split(value, ", ")
		return nil
	}
	if key == "SHA256" {
		pkgInfo.SHA256 = value
		return nil
	}
	if key == "Description" {
		pkgInfo.Description = value
		return nil
	}
	if key == "Filename" {
		pkgInfo.FileName = value
		return nil
	}
	if key == "Maintainer" {
		re := regexp.MustCompile(`(?P<name>.*) <(?P<email>.*)>`)
		matches := re.FindStringSubmatch(value)
		if len(matches) != 3 {
			return fmt.Errorf("Unable to read maintainer info %v", value)
		}

		pkgInfo.Maintainer = &PackageMaintainer{
			Name:  matches[1],
			Email: matches[2],
		}

		return nil
	}

	return nil
}
