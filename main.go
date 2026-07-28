package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: incipit <command> [args]")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "init":
		fmt.Println("init: not yet implemented")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}