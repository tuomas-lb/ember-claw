package main

import (
	"os"

	"github.com/LastBotInc/ember-claw/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
