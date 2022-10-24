package main

import (
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/gjolly/go-rmadison/pkg/debianpkg"
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

func sortArch(line []string) {
	archs := line[3]
	archList := strings.Split(archs, ", ")
	sort.Strings(archList)

	line[3] = strings.Join(archList, ", ")
}

func groupByComponent(lines [][]string) [][]string {
	linesBySeries := make(map[string][]string)
	for _, line := range lines {
		key := line[2]
		version := line[1]
		if newLine, ok := linesBySeries[key]; ok && newLine[1] == version {
			newLine[3] += ", " + line[3]
			continue
		}
		linesBySeries[key] = line
	}

	out := make([][]string, len(linesBySeries))
	i := 0
	for _, line := range linesBySeries {
		sortArch(line)
		out[i] = line

		i++
	}

	return out
}

func main() {
	client := resty.New()

	flag.Parse()

	pkg := flag.Arg(0)

	baseURL := "http://localhost:8433"

	queryURL := fmt.Sprintf("%v/%v", baseURL, pkg)

	var pkgInfo []debianpkg.PackageInfo
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

	lines = groupByComponent(lines)

	lineFormat := fmt.Sprintf(" %%-%vv | %%-%vv | %%-%vv | %%-%vv\n", widths[0], widths[1], widths[2], widths[3])
	for _, line := range lines {
		fmt.Printf(lineFormat, line[0], line[1], line[2], line[3])
	}
}
