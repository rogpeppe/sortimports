package main

import (
	"bufio"
	"bytes"
	stdflag "flag"
	"fmt"
	"go/format"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

type imp struct {
	preComments []string
	postComment string
	ident       string
	path        string
}

var exitCode int

var flag = stdflag.NewFlagSet(os.Args[0], stdflag.ContinueOnError)

var nflag = flag.Bool("n", false, "print files that have changed, but do not write")

func main() {
	os.Exit(Main())
}

func Main() int {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: sortimports [pkg...]\n")
		flag.PrintDefaults()
	}
	if err := flag.Parse(os.Args[1:]); err != nil {
		return 2
	}
	pkgPaths := flag.Args()
	if len(pkgPaths) == 0 {
		pkgPaths = []string{"."}
	}
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedFiles | packages.NeedModule | packages.NeedImports,
	}, pkgPaths...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get load packages: %v", err)
		return 1
	}
	for _, pkg := range pkgs {
		sortImports(pkg)
	}
	return exitCode
}

func sortImports(pkg *packages.Package) {
	if !pkg.Module.Main {
		warning("not formatting packages in dependency modules")
		return
	}
	if len(pkg.GoFiles) == 0 {
		warning("no Go files found in %q", pkg.PkgPath)
		return
	}
	// We want to process all Go files in the directory,
	// not just ones with matching tags, so read the directory
	// instead of using pkg.GoFiles.
	dir := filepath.Dir(pkg.GoFiles[0])
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		warning("cannot read %q: %v", dir, err)
		return
	}
	for _, info := range infos {
		path := filepath.Join(dir, info.Name())
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			warning("cannot read %q: %v", path, err)
			continue
		}
		result, err := process(pkg, data)
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

func process(pkg *packages.Package, data []byte) ([]byte, error) {
	indexes := importRegex.FindSubmatchIndex(data)
	if indexes == nil {
		return data, nil
	}
	i0, i1 := indexes[2], indexes[3]
	var out bytes.Buffer
	out.Write(data[0:i0])
	in := bytes.NewReader(data[i0:i1])
	if err := sortImportSection(pkg, &out, in); err != nil {
		return nil, err
	}
	out.Write(data[i1:])
	return out.Bytes(), nil
}

func sortImportSection(pkg *packages.Package, w io.Writer, r io.Reader) error {
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
		pkg:     pkg,
		imports: imps,
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
	pkg     *packages.Package
	imports []imp
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

	if dp := byg.pkg.Imports[path]; dp != nil && dp.Module != nil && dp.Module.Main {
		return 2
	}
	if strings.Contains(path, ".") {
		return 1
	}
	return 0
}
