// Command gomedia-bench is the developer tool for the go-media benchmark
// framework (see bench/README.md).
//
//	gomedia-bench gen      generate the fixture corpus from bench/manifest.json
//	gomedia-bench run      run benchmark suites N times and store raw results
//	gomedia-bench compare  compare two result labels (mean, stddev, % change)
//
// The tool never runs commands through a shell: every external program
// (ffmpeg, go, sysctl, git) is started with an explicit argument vector.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "gen":
		err = cmdGen(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "compare":
		err = cmdCompare(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gomedia-bench:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  gomedia-bench gen     [-manifest bench/manifest.json] [-data bench/.data] [-force] [-only regex]
  gomedia-bench run     -label NAME [-suite e2e|internal|all] [-count 5] [-benchtime 1s] [-bench regex] [-pgo=true] [-jobs 4] [-out bench/results]
  gomedia-bench compare [-format md|text] BASE_LABEL_DIR NEW_LABEL_DIR`)
}
