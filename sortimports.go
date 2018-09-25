package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/build"
	"go/format"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kisielk/gotool"
)

type imp struct {
	preComments []string
	postComment string
	ident       string
	path        string
}

var exitCode int
var cwd string

var nflag = flag.Bool("n", false, "print files that have changed, but do not write")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: sortimports [pkg...]\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	stdin := flag.String("c", "", "operate on stdin, with the local package path derived from this flags value")
	flag.Parse()
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get current working directory: %v", err)
		os.Exit(2)
	}
	cwd = wd
	if *stdin != "" {
		processStdin(*stdin)
	}
	pkgs := flag.Args()
	// Only assume the package in cwd if stdin wasn't used.
	if len(pkgs) == 0 && *stdin == "" {
		pkgs = []string{"."}
	}
	for _, pkg := range gotool.ImportPaths(pkgs) {
		sortImports(pkg)
	}
	os.Exit(exitCode)
}

func processStdin(path string) {
	// Absolute paths must be made relative, with prefixed "./" to work with
	// build.Import.
	if filepath.IsAbs(path) {
		var err error
		path, err = filepath.Rel(cwd, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "can't create relative path: %s", err)
			os.Exit(2)
		}
		path = "./" + path
	}
	pkg, err := build.Import(path, cwd, build.FindOnly)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read package: %s", err)
		os.Exit(2)
	}
	input, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading stdin: %s", err)
		os.Exit(1)
	}
	output, err := process(pkg.ImportPath, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in processing: %s", err)
		os.Exit(1)
	}
	output, err = format.Source(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error formatting: %s", err)
		os.Exit(1)
	}
	os.Stdout.Write(output)
}

func sortImports(pkgPath string) {
	pkg, err := build.Import(pkgPath, cwd, build.FindOnly)
	if err != nil {
		warning("cannot read package %q: %v", pkg, err)
		return
	}
	pkgPath = pkg.ImportPath
	infos, err := ioutil.ReadDir(pkg.Dir)
	if err != nil {
		warning("cannot read %q: %v", pkg.Dir, err)
		return
	}
	for _, info := range infos {
		path := filepath.Join(pkg.Dir, info.Name())
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			warning("cannot read %q: %v", path, err)
			continue
		}
		result, err := process(pkgPath, data)
		if err != nil {
			warning("%s: %v", path, err)
			continue
		}
		result, err = format.Source(result)
		if err != nil {
			warning("%s: %v", path, err)
			continue
		}
		if bytes.Equal(result, data) {
			continue
		}
		fmt.Printf("%s\n", path)
		if *nflag {
			continue
		}
		if err := ioutil.WriteFile(path, result, 0666); err != nil {
			warning("cannot write %q: %v", path, err)
		}
	}
}

func warning(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "warning: %s\n", fmt.Sprintf(f, a...))
	exitCode = 1
}

var importRegex = regexp.MustCompile(`import \((([^)]|\n)*)\)`)

func process(pkgPath string, data []byte) ([]byte, error) {
	indexes := importRegex.FindSubmatchIndex(data)
	if indexes == nil {
		return data, nil
	}
	i0, i1 := indexes[2], indexes[3]
	var out bytes.Buffer
	out.Write(data[0:i0])
	in := bytes.NewReader(data[i0:i1])
	if err := sortImportSection(pkgPath, &out, in); err != nil {
		return nil, err
	}
	out.Write(data[i1:])
	return out.Bytes(), nil
}

func sortImportSection(pkgPath string, w io.Writer, r io.Reader) error {
	var imps []imp
	for scan := bufio.NewScanner(r); scan.Scan(); {
		var preComments []string
		text := strings.TrimSpace(scan.Text())
		for strings.HasPrefix(text, "//") || text == "" {
			if text != "" {
				preComments = append(preComments, text)
			}
			if !scan.Scan() {
				return fmt.Errorf("found comments not attached to import")
			}
			text = strings.TrimSpace(scan.Text())
		}
		var postComment string
		if i := strings.Index(text, "//"); i != -1 {
			postComment = text[i:]
			text = text[0:i]
		}
		fields := strings.Fields(text)
		if len(fields) == 0 || len(fields) > 2 {
			return fmt.Errorf("invalid import line %q", scan.Text())
		}
		path, err := strconv.Unquote(fields[len(fields)-1])
		if err != nil {
			return fmt.Errorf("cannot parse %q as string literal", scan.Text())
		}
		i := imp{
			preComments: preComments,
			postComment: postComment,
			path:        path,
		}
		if len(fields) == 2 {
			i.ident = fields[0]
		}
		imps = append(imps, i)
	}
	byg := byGroup{
		localPackagePrefix: localPackagePrefix(pkgPath),
		imports:            imps,
	}
	sort.Sort(byg)
	g := 0
	for _, i := range imps {
		if byg.group(i.path) != g {
			fmt.Fprintln(w, "")
			g = byg.group(i.path)
		}
		for _, c := range i.preComments {
			fmt.Fprintf(w, "%s\n", c)
		}
		fmt.Fprintf(w, "%s %q %s\n", i.ident, i.path, i.postComment)
	}
	return nil
}

type byGroup struct {
	localPackagePrefix string
	imports            []imp
}

func (byg byGroup) Less(i, j int) bool {
	i0, i1 := byg.imports[i], byg.imports[j]
	if g0, g1 := byg.group(i0.path), byg.group(i1.path); g0 != g1 {
		return g0 < g1
	}
	return i0.path < i1.path
}

func (byg byGroup) Swap(i, j int) {
	byg.imports[i], byg.imports[j] = byg.imports[j], byg.imports[i]
}

func (byg byGroup) Len() int {
	return len(byg.imports)
}

func (byg byGroup) group(path string) int {
	prefix := byg.localPackagePrefix
	if prefix != "" && strings.HasPrefix(path, prefix) {
		if len(path) == len(prefix) || path[len(prefix)] == '/' {
			return 2
		}
	}
	if strings.Contains(path, ".") {
		return 1
	}
	return 0
}

var matchers = []func(pkg string) []string{
	matcher(`^(gopkg\.in/([^/]*/)?[^/]+\.v[0-9]+(-unstable)?)(/|$)`),
	matcher(`^(github\.com/[^/]+/[^/]+)`),
	matcher(`^(bitbucket\.org/[^/]+/[^/]+)`),
	matcher(`^(launchpad\.net/[^/]+)`),
	matcher(`^(code\.google\.com/p/[^/]+)`),
	matcher(`^([a-z]+\.[^/]+/[^/]+)`),
}

func matcher(pat string) func(pkg string) []string {
	re := regexp.MustCompile(pat)
	return func(pkg string) []string {
		return re.FindStringSubmatch(pkg)
	}
}

func localPackagePrefix(pkg string) string {
	for _, m := range matchers {
		if matches := m(pkg); matches != nil {
			return matches[1]
		}
	}
	return ""
}
