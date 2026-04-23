package main

import (
	"fmt"
	"runtime/debug"
)

// version is overridden at link time via -ldflags=-X. Empty default
// makes development builds say "dev" rather than blank, while
// release tarballs print the tagged version.
var version = "dev"

func runVersion(_ []string) error {
	revision := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				revision = setting.Value
				break
			}
		}
	}
	if revision != "" {
		fmt.Printf("yggdrasil %s (%s)\n", version, shortHash(revision))
		return nil
	}
	fmt.Printf("yggdrasil %s\n", version)
	return nil
}

func shortHash(h string) string {
	if len(h) < 8 {
		return h
	}
	return h[:8]
}
