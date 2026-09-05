package main

import (
	"os"

	"github.com/jbonadiman/mirror-to-org/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
