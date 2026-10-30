package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/mod/semver"
)

// Pins that must not move downward when gopls/go.mod is merged in.
// The golang.org/x floors are the versions that close the module
// advisories. text and mod pull golang.org/x/tools v0.49.0.
var keepAtLeast = map[string]string{
	"github.com/fsnotify/fsnotify": "v1.10.1",
	"golang.org/x/crypto":          "v0.57.0",
	"golang.org/x/image":           "v0.45.0",
	"golang.org/x/mod":             "v0.41.0",
	"golang.org/x/net":             "v0.60.0",
	"golang.org/x/telemetry":       "v0.0.0-20260811182544-a038080d80e5",
	"golang.org/x/text":            "v0.42.0",
	"golang.org/x/tools":           "v0.49.0",
}

func cmdGoMod(from string) error {
	reqs, err := parseRequires(from)
	if err != nil {
		return err
	}
	for mod, ver := range reqs {
		if err := run("go", "get", mod+"@"+ver); err != nil {
			return fmt.Errorf("go get %s@%s: %w", mod, ver, err)
		}
	}
	current, err := parseRequires("go.mod")
	if err != nil {
		return err
	}
	for mod, floor := range keepAtLeast {
		if !versionBelow(current[mod], floor) {
			continue
		}
		if err := run("go", "get", mod+"@"+floor); err != nil {
			return fmt.Errorf("restore %s@%s: %w", mod, floor, err)
		}
	}
	return nil
}

// versionBelow reports whether current is missing or older than floor.
// An empty or invalid version is below the floor, so the floor is applied.
func versionBelow(current, floor string) bool {
	if current == "" || !semver.IsValid(current) || !semver.IsValid(floor) {
		return true
	}
	return semver.Compare(current, floor) < 0
}

func parseRequires(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "require (") {
			in = true
			continue
		}
		if in && line == ")" {
			in = false
			continue
		}
		if strings.HasPrefix(line, "require ") && !strings.HasPrefix(line, "require (") {
			fields := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(fields) >= 2 {
				out[fields[0]] = fields[1]
			}
			continue
		}
		if !in || line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && !strings.HasPrefix(fields[0], "//") {
			out[fields[0]] = fields[1]
		}
	}
	return out, sc.Err()
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Fprintf(os.Stderr, "+ %s %s\n", name, strings.Join(args, " "))
	return cmd.Run()
}
