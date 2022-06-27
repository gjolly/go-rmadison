package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/gjolly/go-rmadisson/pkg/debian"
	"github.com/go-resty/resty/v2"
)

func contains(elmt string, slice []string) bool {
	for _, e := range slice {
		if elmt == e {
			return true
		}
	}

	return false
}

func main() {
	client := resty.New()

	architectureFlag := flag.String("architecture", "amd64", "only show info for ARCH(s)")
	componentFlag := flag.String("component", "main,universe,multiverse", "only show info for COMPONENT(s)")
	suiteFlag := flag.String("suite", "bionic,focal,impish,jammy,kinetic", "only show info for this suite")
	flag.Parse()

	pkg := flag.Arg(0)

	architectures := strings.Split(*architectureFlag, ",")
	components := strings.Split(*componentFlag, ",")
	suites := strings.Split(*suiteFlag, ",")

	baseURL := "http://localhost:8433"

	queryURL := fmt.Sprintf("%v/%v", baseURL, pkg)

	var pkgInfo []debian.PackageInfo
	resp, err := client.R().
		SetResult(&pkgInfo).
		Get(queryURL)

	if err != nil {
		log.Fatal(err)
	}
	if resp.IsError() {
		log.Fatal(resp.Status())
	}

	widths := make([]int, 4)
	lines := make([][]string, 0)
	for _, info := range pkgInfo {
		if !contains(info.Component, components) || !contains(info.Suite, suites) || !contains(info.Architecture, architectures) {
			continue
		}
		formatedComponent := ""
		if info.Component != "main" {
			formatedComponent = "/" + info.Component
		}
		line := []string{info.Name, info.Version, info.Suite + info.Pocket + formatedComponent, info.Architecture}
		for i, word := range line {
			if len(word) > widths[i] {
				widths[i] = len(word)
			}
		}
		lines = append(lines, line)
	}

	lineFormat := fmt.Sprintf(" %%-%vv | %%-%vv | %%-%vv | %%-%vv\n", widths[0], widths[1], widths[2], widths[3])
	for _, line := range lines {
		fmt.Printf(lineFormat, line[0], line[1], line[2], line[3])
	}
}
