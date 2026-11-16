package main

import (
	_ "embed"
	"strings"
)

// versionFile holds the release version. The release pull request bumps it,
// and the release workflows check it against the tag and CHANGELOG.md.
//
//go:embed VERSION
var versionFile string

// version is the semantic version of the last release, such as "0.1.0".
// A build between releases prints the last release.
func version() string {
	return strings.TrimSpace(versionFile)
}
