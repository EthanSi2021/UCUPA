// Command verify is the repository's non-mutating engineering and validation gate.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	checksOnly := flag.Bool("checks-only", false, "run engineering checks without the test, race, vet, tidy, demo and diff gates")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "verify: unexpected arguments")
		os.Exit(2)
	}
	if err := verify(*checksOnly); err != nil {
		fmt.Fprintln(os.Stderr, "verify:", err)
		os.Exit(1)
	}
}

func verify(checksOnly bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := repositoryRoot(cwd)
	if err != nil {
		return err
	}
	goTool, err := findGo()
	if err != nil {
		return err
	}
	stdlib, err := standardLibrary(root, goTool)
	if err != nil {
		return err
	}
	fmt.Println("Engineering checks:", root)
	issues := checkRepository(root, stdlib)
	errors, reviews := 0, 0
	for _, item := range issues {
		level := "ERROR"
		if item.review {
			level = "REVIEW"
			reviews++
		} else {
			errors++
		}
		fmt.Printf("%s %s: %s\n", level, item.path, item.message)
	}
	fmt.Printf("Engineering checks: %d error(s), %d responsibility review(s).\n", errors, reviews)
	if errors != 0 {
		return fmt.Errorf("engineering checks failed; runtime gates were not run")
	}
	if checksOnly {
		fmt.Println("Checks-only complete; runtime gates were not run.")
		return nil
	}
	failed := 0
	for _, gate := range validationGates(goTool) {
		fmt.Printf("\nRUN %s\n", gate.name)
		cmd := exec.Command(gate.program, gate.args...)
		cmd.Dir = root
		cmd.Env = commandEnvironment(gate.race)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", gate.name, err)
			if gate.race {
				fmt.Fprintln(os.Stderr, "Race is required: CGO_ENABLED=1 and a working C compiler are required; it was not skipped.")
			}
		} else {
			fmt.Println("PASS", gate.name)
		}
	}
	if failed != 0 {
		return fmt.Errorf("%d validation gate(s) failed", failed)
	}
	fmt.Println("All engineering and runtime gates passed.")
	return nil
}

type validationGate struct {
	name, program string
	args          []string
	race          bool
}

func validationGates(goTool string) []validationGate {
	return []validationGate{
		{"protobuf generation", goTool, []string{"run", "./cmd/protogen", "-check"}, false},
		{"tests", goTool, []string{"test", "./...", "-count=1", "-timeout=20m"}, false},
		{"race", goTool, []string{"test", "-race", "./...", "-count=1", "-timeout=20m"}, true},
		{"vet", goTool, []string{"vet", "./..."}, false},
		{"dependencies (tidy -diff, no writes)", goTool, []string{"mod", "tidy", "-diff"}, false},
		{"local demo", goTool, []string{"run", "./cmd/ucupa"}, false},
		{"diff whitespace", "git", []string{"diff", "--check"}, false},
	}
}

func repositoryRoot(start string) (string, error) {
	for dir := start; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("no repository .git found above %s", start)
		}
	}
}

func findGo() (string, error) {
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{filepath.Join(runtime.GOROOT(), "bin", name)}
	if self, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(self), name))
	}
	if path, err := exec.LookPath(name); err == nil {
		candidates = append(candidates, path)
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", fmt.Errorf("Go tool not found in GOROOT, executable directory or PATH")
}

func standardLibrary(root, goTool string) (map[string]bool, error) {
	cmd := exec.Command(goTool, "list", "std")
	cmd.Dir, cmd.Env = root, commandEnvironment(false)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list standard library: %w\n%s", err, out)
	}
	packages := make(map[string]bool)
	for _, name := range strings.Fields(string(out)) {
		packages[name] = true
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("Go returned an empty standard library list")
	}
	return packages, nil
}

func commandEnvironment(race bool) []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.EqualFold(key, "GOFLAGS") || strings.EqualFold(key, "GOWORK") || (race && strings.EqualFold(key, "CGO_ENABLED")) {
			continue
		}
		env = append(env, item)
	}
	// An empty GOFLAGS falls back to the user's persisted Go configuration.
	// A nonempty space overrides that value but parses as zero flags, without
	// disabling unrelated user settings such as CC or proxy configuration.
	env = append(env, "GOFLAGS= ", "GOWORK=off")
	if race {
		env = append(env, "CGO_ENABLED=1")
	}
	return env
}
