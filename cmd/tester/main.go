package main

import (
	"os"

	"github.com/b-harvest/modules-test-tool/cmd/tester/cmd"
)

func main() {
	if err := cmd.RootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
