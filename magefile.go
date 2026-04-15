//go:build mage
// +build mage

package main

import (
	"os"
	"strings"
	"time"

	"github.com/magefile/mage/sh"
)

var (
	binaryName = "terradrift"
	version    = getGitVersion()
	buildTime  = time.Now().UTC().Format("2006-01-02T15:04:05Z")
)

func getGitVersion() string {
	out, err := sh.Output("git", "describe", "--tags", "--always", "--dirty")
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(out)
}

// Build the binary
func Build() error {
	return sh.Run("go", "build",
		"-ldflags", "-X main.version="+version+" -X main.buildTime="+buildTime,
		"-o", binaryName, ".")
}

// Run tests
func Test() error {
	return sh.Run("go", "test", "-v", "-race", "./...")
}

// Run linter
func Lint() error {
	return sh.Run("golangci-lint", "run", "./...")
}

// Clean build artifacts
func Clean() error {
	os.Remove(binaryName)
	os.RemoveAll("dist")
	return nil
}
