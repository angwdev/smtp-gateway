// Command covercheck reads a Go coverage profile and fails when a layer is
// below its target: domain 100%, application 99%, infrastructure 95%,
// interfaces 90% (skill rule testing-coverage-99).
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const module = "github.com/angwdev/smtp-gateway/"

var targets = []struct {
	layer  string
	prefix string
	min    float64
}{
	{"domain", "internal/domain/", 100},
	{"application", "internal/application/", 99},
	{"infrastructure", "internal/infrastructure/", 95},
	{"interfaces", "internal/interfaces/", 90},
}

// Not gated: generated mocks, and moved v1 code whose tests need Pebble.
var excluded = []string{"/mock_", "internal/infrastructure/acme/", "internal/infrastructure/tlsutil/"}

type result struct {
	layer          string
	covered, total int
	min            float64
	ok             bool
}

func (r result) percent() float64 {
	if r.total == 0 {
		return 0
	}
	return 100 * float64(r.covered) / float64(r.total)
}

type block struct {
	statements int
	hit        bool
}

func check(profile io.Reader) ([]result, error) {
	// With -coverpkg=./... every test binary reports every block, so blocks
	// are merged by position: covered if any binary executed it.
	blocks := map[string]*block{}
	sc := bufio.NewScanner(profile)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected profile line: %q", line)
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("statement count in %q: %w", line, err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("hit count in %q: %w", line, err)
		}
		b := blocks[fields[0]]
		if b == nil {
			b = &block{statements: statements}
			blocks[fields[0]] = b
		}
		b.hit = b.hit || count > 0
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}

	results := make([]result, len(targets))
	for i, t := range targets {
		results[i] = result{layer: t.layer, min: t.min}
	}
	for position, b := range blocks {
		path := strings.TrimPrefix(position, module)
		if isExcluded(path) {
			continue
		}
		for i, t := range targets {
			if strings.HasPrefix(path, t.prefix) {
				results[i].total += b.statements
				if b.hit {
					results[i].covered += b.statements
				}
				break
			}
		}
	}
	for i := range results {
		results[i].ok = results[i].total > 0 && results[i].percent() >= results[i].min // an empty layer means a broken profile, not full coverage
	}
	return results, nil
}

func isExcluded(path string) bool {
	for _, e := range excluded {
		if strings.Contains(path, e) {
			return true
		}
	}
	return false
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: covercheck coverage.out")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1]) //nolint:gosec // CLI tool: the path is an operator-supplied argument, not external input
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	results, checkErr := check(f)
	closeErr := f.Close()
	if checkErr != nil {
		fmt.Fprintln(os.Stderr, checkErr)
		os.Exit(2)
	}
	if closeErr != nil {
		fmt.Fprintln(os.Stderr, closeErr)
		os.Exit(2)
	}
	failed := false
	for _, r := range results {
		mark := "ok  "
		if !r.ok {
			mark, failed = "FAIL", true
		}
		fmt.Printf("%s %-15s %6.2f%% (need %.0f%%, %d of %d statements)\n", mark, r.layer, r.percent(), r.min, r.covered, r.total)
	}
	if failed {
		fmt.Println("\nUncovered lines: tools/go tool cover -func=coverage.out | grep -v '100.0%'")
		os.Exit(1)
	}
}
