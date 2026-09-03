package main

import (
	"context"
	"log"
	"os"

	"github.com/ironpark/toolz/cli/planr/cmd"
)

func main() {
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
