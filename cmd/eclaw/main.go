package main

import (
	"os"

	"github.com/tuomas-lb/ember-claw/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
