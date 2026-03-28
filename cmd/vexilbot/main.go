package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: vexilbot <config-path>\n")
		os.Exit(1)
	}
	fmt.Printf("vexilbot starting with config: %s\n", os.Args[1])
}
