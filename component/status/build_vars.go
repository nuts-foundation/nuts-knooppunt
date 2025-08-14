package status

import (
	"fmt"
	"runtime"
	"strings"
)

// Hash of the commit the binary was built on
var GitCommit = "0"

// Version tag the commit is on
var GitVersion string

// The branch the binary was built from
var GitBranch = "development"

func Version() string {
	if GitVersion != "" && GitVersion != "undefined" {
		return GitVersion
	}
	return GitBranch
}

func OSArch() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func BuildInfo() string {
	b := strings.Builder{}
	b.WriteString("Git version: ")
	b.WriteString(Version())
	b.WriteString("\n")

	b.WriteString("Git commit: ")
	b.WriteString(GitCommit)
	b.WriteString("\n")

	b.WriteString("OS/Arch: ")
	b.WriteString(OSArch())
	b.WriteString("\n")

	return b.String()
}
