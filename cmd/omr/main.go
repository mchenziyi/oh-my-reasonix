package main

import (
	"fmt"
	"os"
)

func main() {
	code := run(os.Args[1:])
	if code != 0 {
		os.Exit(code)
	}
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	var err error
	switch args[0] {
	case "init":
		err = runInstall(args[1:], false)
	case "upgrade":
		err = runInstall(args[1:], true)
	case "uninstall":
		err = runUninstall(args[1:])
	case "doctor":
		err = runDoctor(args[1:])
	case "config":
		err = runConfig(args[1:])
	case "profile":
		err = runProfile(args[1:])
	case "session":
		err = runSession(args[1:])
	case "benchmark":
		err = runBenchmark(args[1:])
	case "claude":
		err = runClaude(args[1:])
	case "hook":
		err = runHook(args[1:])
	case "task":
		err = runTask(args[1:])
	case "version":
		err = runVersion(args[1:])
	case "comment-check":
		err = runCommentCheck(args[1:])
	case "run":
		err = runRun(args[1:])
	default:
		usage()
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "omr:", err)
		return 1
	}
	return 0
}

func usage() {
	fmt.Printf("%s init|upgrade|uninstall|doctor|config|profile|session|benchmark|comment-check|version\n", os.Args[0])
	fmt.Println("Use --help on a command for flags.")
}
