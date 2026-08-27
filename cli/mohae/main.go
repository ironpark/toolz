package main

import (
	"fmt"
	"os"
)

const version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Printf("mohae %s\n", version)
		return
	}

	fmt.Println("mohae — reproducible trials for agent tools")
	fmt.Println("usage: mohae [--version]")
}
