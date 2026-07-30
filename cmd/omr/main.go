package main

import (
	"os"
)

func main() {
	code := run(os.Args[1:])
	if code != 0 {
		os.Exit(code)
	}
}
