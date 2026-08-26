package main

import (
	"flag"
	"fmt"
)

func formatGreeting(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

func main() {
	name := flag.String("name", "world", "name to greet")
	flag.Parse()
	fmt.Println(formatGreeting(*name))
}
