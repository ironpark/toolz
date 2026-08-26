package main

import (
	"fmt"
	"os"
)

func main() {
	config, err := LoadConfigFromOS()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := NewRelay(config).Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
