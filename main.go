package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
)

var (
	commits    []string
	outdir     string
	currentcmd *exec.Cmd

	benchcmd   = []string{"go", "test"}
	comparecmd = []string{"benchcmp"}
)

// setup & parse flags, check command availability, build commands from flags
func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <commit-ish>...\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.StringVar(&outdir, "outdir", "", "directory to store benchmark results. if blank, uses the OS's temp dir and is cleaned up afterwards")
	verbose := flag.Bool("verbose", false, "chatty logging")
	flag.String("test.run", "NONE", "run only the tests and examples matching the regular expression")
	flag.String("test.bench", ".", "run benchmarks matching the regular expression")
	flag.Bool("test.short", false, "tell long running tests to shorten their run time")
	flag.Bool("test.benchmem", false, "include memory allocation statistics for comparison")
	flag.Bool("best", false, "compare best times")
	flag.Bool("changed", false, "show only benchmarks that have changed")
	flag.Bool("mag", false, "sort benchmarks by magnitude of change")
	flag.Parse()
	// setup log
	log.SetFlags(0)
	if !*verbose {
		log.SetOutput(ioutil.Discard)
	}
	// check that there are enough commits to compare
	commits = flag.Args()
	if len(commits) < 2 {
		fmt.Fprintln(os.Stderr, "not enough commits to compare")
		flag.Usage()
		os.Exit(1)
	}
	// check that the necessary commands are present
	for _, command := range []string{"go", "git", "benchcmp"} {
		log.Println("checking for presence of", command)
		if _, err := exec.LookPath(command); err != nil {
			fmt.Fprintln(os.Stderr, "command not found:", command)
			os.Exit(1)
		}
	}
	// build commands from args
	buildCommand(&benchcmd, "test.", []string{"run", "bench", "short", "benchmem"})
	buildCommand(&comparecmd, "", []string{"best", "changed", "mag"})
	comparecmd = append(comparecmd, "", "")[:len(comparecmd)] // make capacity for 2 more args
	log.Println("benchmark command:", benchcmd)
	log.Println("benchcmp command:", append(comparecmd, "file1", "file2"))
}

func main() {
	defer catch()
	go handleInterrupt()
	var err error
	if outdir == "" {
		outdir, err = ioutil.TempDir("", "gitbenchcmp")
		check("cannot create temp dir to store benchmarks:", err)
		defer func() { log.Println("removing temp dir"); check(os.RemoveAll(outdir)) }()
	} else {
		err = os.MkdirAll(outdir, 0666)
		check("cannot create output directory:", err)
	}
	checkTreeClean()
	defer func(ref string) { checkout(ref) }(getHeadRef())
	defer killCurrentCmd()
	outfiles := make([]string, len(commits))
	for i, commitish := range commits {
		outfiles[i] = benchCommit(commitish)
	}
	for i := range outfiles[:len(outfiles)-1] {
		fmt.Println()
		compare(outfiles[i], outfiles[i+1])
	}
}

// benchCommit checks out commitish and runs the go test benchmark on it, redirecting
// its' output to a file under outdir, then returns the name of the file
func benchCommit(commitish string) (name string) {
	name = filepath.Join(outdir, commitish)
	log.Println("creating benchmark file:", name)
	file, err := createNew(name)
	if err != nil {
		hash := getCommitHash(commitish)
		if _, ok := err.(*os.PathError); ok {
			log.Println("patherror creating file", name, ":", err, "using hash instead")
			name = filepath.Join(outdir, hash[:12])
			file, err = createNew(name)
		}
		if err == os.ErrExist {
			log.Println("file already exists, appending hash:", name)
			name = name + "-" + hash[:12]
			file, err = createNew(name)
		}
	}
	check("cannot create temp file for benchmark results", name, ":", err)
	defer func() { check("error closing bench output file", name, ":", file.Close()) }()
	checkout(commitish)
	log.Println("running benchmark...")
	currentcmd = exec.Command(benchcmd[0], benchcmd[1:]...)
	currentcmd.Stdout = file
	err = currentcmd.Run()
	check("cannot run benchmarks for", commitish, ":", err)
	return
}

func compare(file1, file2 string) {
	log.Println("comparing benchmark files:", file1, file2)
	currentcmd = exec.Command(comparecmd[0], append(comparecmd[1:], file1, file2)...)
	currentcmd.Stdout = os.Stdout
	currentcmd.Stderr = os.Stderr
	err := currentcmd.Run()
	check("error running benchcmp:", err)
}

func killCurrentCmd() {
	c := currentcmd
	if c != nil && c.ProcessState == nil && c.Process != nil {
		log.Println("killing the currently running process:", c.Args)
		c.Process.Kill()
	}
}

// createNew is os.Create but errors if the file already exists
func createNew(name string) (*os.File, error) {
	return os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
}

// wrappers for git internals. these handle git commands internally and are never killed
func getCommitHash(commitish string) (hash string) {
	log.Println("getting commit hash for", commitish)
	cmd := exec.Command("git", "rev-parse", commitish+"^{commit}")
	out, err := cmd.Output()
	check("cannot get hash for", commitish, ":", err)
	return string(out)
}

func getHeadRef() string {
	log.Println("getting HEAD ref")
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD", "--")
	out, err := cmd.Output()
	check("cannot get current HEAD ref:", err)
	return strings.TrimSpace(string(out))
}

func checkTreeClean() {
	log.Println("checking if the tree is clean")
	cmd := exec.Command("git", "diff-index", "--quiet", "HEAD", "--")
	err := cmd.Run()
	check("working tree is dirty:", err)
}

func checkout(commitish string) {
	log.Println("checking out", commitish)
	cmd := exec.Command("git", "checkout", "--quiet", commitish, "--")
	err := cmd.Run()
	check("cannot checkout", commitish, ":", err)
}

// buildCommand	passes on the values of the flags listed in `flags` from those
// passed to this command. Values are looked up with the name prefix+flags[n],
// but appended to cmd with the name flags[n].
func buildCommand(cmd *[]string, prefix string, flags []string) {
	for _, name := range flags {
		f := flag.Lookup(prefix + name)
		value := f.Value.String()
		if value == f.DefValue && isZeroValue(value) {
			continue
		}
		*cmd = append(*cmd, "-"+name+"="+value)
	}
}

// isZeroValue guesses whether the string represents the zero
// value for a flag. It is not accurate but in practice works OK.
// from flag package
func isZeroValue(value string) bool {
	switch value {
	case "false":
		return true
	case "":
		return true
	case "0":
		return true
	}
	return false
}

// error handling convenience wrappers
var interrupted bool

func handleInterrupt() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	for {
		<-c
		interrupted = true
		// change to verbose mode, also syncronizes with logging events
		log.SetOutput(os.Stderr)
		killCurrentCmd()
	}
}

type checkedErr string

// check sprints args and panics if any of args is a non-nil error
func check(args ...interface{}) {
	for _, arg := range args {
		if err, ok := arg.(error); ok && err != nil {
			s := fmt.Sprintln(args...)
			// switch to verbose mode
			log.SetOutput(os.Stderr)
			log.Print(s)
			panic(checkedErr(s))
		}
	}
	if interrupted {
		panic("interrupted")
	}
}

func catch() {
	v := recover()
	if v != nil {
		// if it's checkedErr it's already been printed
		if _, ok := v.(checkedErr); !ok {
			fmt.Fprint(os.Stderr, v)
		}
		os.Exit(1)
	}
}
