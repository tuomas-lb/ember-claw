package main

import (
	"flag"
	"os"

	"github.com/tuomas-lb/ember-claw/internal/cli"
	"k8s.io/klog/v2"
)

func main() {
	// Suppress noisy client-go/SPDY port-forward logs (e.g., "connection reset by peer").
	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "false")
	_ = flag.Set("stderrthreshold", "FATAL")

	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
