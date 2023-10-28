package archive

import (
	"io"
	"os"
	"testing"

	"github.com/gjolly/go-rmadison/pkg/debianpkg"
)

func TestParseReleaseFile(t *testing.T) {
	nobleReleaseFile, err := os.Open("./testdata/noble-release.txt")
	if err != nil {
		t.Fatal("failed to open test file", err)
	}

	releaseFile, err := ParseReleaseFile(nobleReleaseFile)
	if err != nil {
		t.Fatal("failed to parse release file", err)
	}

	if releaseFile == nil {
		t.Fatal("no error returned but nil pointer")
	}

	archs := []string{"amd64", "arm64", "armhf", "i386", "ppc64el", "riscv64", "s390x"}
	for iArch, expectedArch := range archs {
		if releaseFile.Architectures[iArch] != expectedArch {
			t.Error("failed to find arch", expectedArch)
		}
	}

	if releaseFile.Codename != "noble" {
		t.Error("wrong codename: expected noble, got ", releaseFile.Codename)
	}

	components := []string{"main", "restricted", "universe", "multiverse"}
	for iComponent, component := range components {
		if releaseFile.Components[iComponent] != component {
			t.Error("failed to find arch", component)
		}
	}

	packageFilePath := "main/debian-installer/binary-armhf/Packages.gz"
	packageFile, ok := releaseFile.PackageIndex[packageFilePath]
	if !ok {
		t.Fatal("failed to find package file", packageFilePath)
	}

	if packageFile.Hash != "e7ab72b8f37c7c9c9f6386fb8e3dfa40bf6fe4b67876703c5927e47cb8664ce4" {
		t.Error("wrong hash for index file")
	}

	if packageFile.Path != packageFilePath {
		t.Error("wrong path for index file")
	}

	if packageFile.Size != 40 {
		t.Error("wrong size for index file")
	}
}

func TestParsePackageIndexFile(t *testing.T) {
	file, err := os.Open("./testdata/jammy-packages.txt")
	if err != nil {
		t.Fatal("failed to open test file", err)
	}

	fileContent, err := io.ReadAll(file)
	if err != nil {
		t.Fatal("failed to read test file", err)
	}

	var (
		pkgInfo  = make(chan *debianpkg.PackageInfo)
		done     = make(chan struct{})
		packages = make([]*debianpkg.PackageInfo, 0)
	)

	go func() {
		for {
			select {
			case info := <-pkgInfo:
				packages = append(packages, info)
			case <-done:
				return
			}
		}
	}()

	err = parsePackageIndexFile(pkgInfo, string(fileContent), "jammy", "", "main", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	done <- struct{}{}

	expectedPackages := 6090
	if len(packages) != expectedPackages {
		t.Errorf("expected %v packages, got %v", expectedPackages, len(packages))
	}
}

func TestGetInfoFromIndexName(t *testing.T) {
	type testData struct {
		Input        string
		Suite        string
		Pocket       string
		Component    string
		Architecture string
	}

	testTable := []testData{
		{
			"ports.ubuntu.com_dists_mantic-updates_main_binary-armhf_Packages.gz",
			"mantic",
			"-updates",
			"main",
			"armhf",
		},
		{
			"ports.ubuntu.com_dists_mantic_main_binary-s390x_Packages.gz",
			"mantic",
			"",
			"main",
			"s390x",
		},
		{
			"archive.ubuntu.com_ubuntu_dists_xenial_main_binary-amd64_Packages.gz",
			"xenial",
			"",
			"main",
			"amd64",
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.Input, func(t *testing.T) {
			s, p, c, a, err := getInfoFromIndexName(testCase.Input)
			if err != nil {
				t.Error("unexpected error", err)
			}

			if s != testCase.Suite {
				t.Errorf("expected %v, got %v", testCase.Suite, s)
			}

			if p != testCase.Pocket {
				t.Errorf("expected %v, got %v", testCase.Pocket, p)
			}

			if c != testCase.Component {
				t.Errorf("expected %v, got %v", testCase.Component, c)
			}

			if a != testCase.Architecture {
				t.Errorf("expected %v, got %v", testCase.Architecture, a)
			}
		})
	}
}
