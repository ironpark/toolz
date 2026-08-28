package main

import (
	"context"
	"log"
	"os"

	"github.com/ironpark/toolz/cli/planr/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
