package engine_test

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The host contract is the exported surface of engine, llm, graph and tools.
// Some of that surface is spelled with types from engine-internal packages
// (aliases such as engine.MapStore, Options fields such as SecretStore, and
// re-exported functions such as engine.SetDefaultStore). Those symbols are
// frozen: changing them breaks hosts exactly like changing engine itself.
//
// This test walks the exported surface of the four contract packages, follows
// every referenced engine-internal symbol transitively (struct fields,
// signatures, exported methods), and requires the reachable set to equal
// frozenInternalSymbols. A new leak, or a removed one, fails until the list
// and docs/architecture/HOST-ENGINE-BOUNDARY.md are updated deliberately.

const fluxModule = "github.com/GrayCodeAI/flux"

// fluxRoot is the module root relative to the engine package directory.
const fluxRoot = ".."

var hostContractPackages = []string{
	fluxModule + "/engine",
	fluxModule + "/graph",
	fluxModule + "/llm",
	fluxModule + "/tools",
}

// frozenInternalSymbols is the complete set of engine-internal symbols
// reachable from the host contract. Keep it sorted and in sync with the
// "Frozen engine-internal types" section of HOST-ENGINE-BOUNDARY.md.
var frozenInternalSymbols = []string{
	fluxModule + "/credentials.DefaultStore",
	fluxModule + "/credentials.MapStore",
	fluxModule + "/credentials.SetDefaultStore",
	fluxModule + "/credentials.Store",
	fluxModule + "/operationsgraph.Export",
	fluxModule + "/operationsgraph.Input",
	fluxModule + "/provider/cache.CacheConfig",
	fluxModule + "/provider/resilience.AdaptiveRateLimitConfig",
	fluxModule + "/provider/resilience.HeaderExtractor",
	fluxModule + "/provider/resilience.RateLimitHeaders",
}

type surfaceDecl struct {
	file      *ast.File
	typeSpec  *ast.TypeSpec
	funcDecl  *ast.FuncDecl
	valueSpec *ast.ValueSpec
	index     int // position of the name inside valueSpec
}

type surfacePkg struct {
	path    string
	name    string
	decls   map[string]surfaceDecl
	methods map[string][]surfaceDecl // receiver base type name -> methods
	imports map[*ast.File]map[string]string
}

type surfaceScanner struct {
	t        *testing.T
	fset     *token.FileSet
	pkgs     map[string]*surfacePkg
	visited  map[string]bool
	internal map[string][]string // internal symbol -> contract paths that reach it
}

func newSurfaceScanner(t *testing.T) *surfaceScanner {
	return &surfaceScanner{
		t:        t,
		fset:     token.NewFileSet(),
		pkgs:     map[string]*surfacePkg{},
		visited:  map[string]bool{},
		internal: map[string][]string{},
	}
}

func isHostContract(path string) bool {
	for _, p := range hostContractPackages {
		if p == path {
			return true
		}
	}
	return false
}

func isFluxPackage(path string) bool {
	return path == fluxModule || strings.HasPrefix(path, fluxModule+"/")
}

// isStdlib mirrors the go command's rule: standard-library import paths have
// no dot in their first element.
func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

var majorVersionElem = regexp.MustCompile(`^v[0-9]+$`)

// assumedPackageName guesses the name of a non-Flux package imported without
// an explicit name, the same way goimports does. A wrong guess cannot hide a
// leak: an unresolved qualifier in a type expression fails the test.
func assumedPackageName(path string) string {
	elems := strings.Split(path, "/")
	name := elems[len(elems)-1]
	if majorVersionElem.MatchString(name) && len(elems) > 1 {
		name = elems[len(elems)-2]
	}
	if i := strings.Index(name, ".v"); i > 0 {
		name = name[:i]
	}
	name = strings.TrimPrefix(name, "go-")
	name = strings.TrimSuffix(name, "-go")
	return strings.ReplaceAll(name, "-", "_")
}

func (s *surfaceScanner) load(path string) *surfacePkg {
	if pkg, ok := s.pkgs[path]; ok {
		return pkg
	}
	dir := filepath.Join(fluxRoot, filepath.FromSlash(strings.TrimPrefix(path, fluxModule)))
	bp, err := build.ImportDir(dir, 0)
	if err != nil {
		s.t.Fatalf("load %s: %v", path, err)
	}
	pkg := &surfacePkg{
		path:    path,
		name:    bp.Name,
		decls:   map[string]surfaceDecl{},
		methods: map[string][]surfaceDecl{},
		imports: map[*ast.File]map[string]string{},
	}
	s.pkgs[path] = pkg
	for _, name := range bp.GoFiles {
		file, err := parser.ParseFile(s.fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			s.t.Fatalf("parse %s/%s: %v", path, name, err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					pkg.decls[d.Name.Name] = surfaceDecl{file: file, funcDecl: d}
				} else if len(d.Recv.List) == 1 {
					recv := receiverTypeName(d.Recv.List[0].Type)
					pkg.methods[recv] = append(pkg.methods[recv], surfaceDecl{file: file, funcDecl: d})
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch sp := spec.(type) {
					case *ast.TypeSpec:
						pkg.decls[sp.Name.Name] = surfaceDecl{file: file, typeSpec: sp}
					case *ast.ValueSpec:
						for i, n := range sp.Names {
							pkg.decls[n.Name] = surfaceDecl{file: file, valueSpec: sp, index: i}
						}
					}
				}
			}
		}
	}
	return pkg
}

func receiverTypeName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}

func (s *surfaceScanner) importsOf(pkg *surfacePkg, file *ast.File) map[string]string {
	if m, ok := pkg.imports[file]; ok {
		return m
	}
	m := map[string]string{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			s.t.Fatalf("%s: bad import %s", pkg.path, imp.Path.Value)
		}
		var name string
		switch {
		case imp.Name != nil:
			name = imp.Name.Name
		case isFluxPackage(path):
			name = s.load(path).name
		default:
			name = assumedPackageName(path)
		}
		if name == "." {
			s.t.Errorf("%s: dot import of %s hides package qualifiers from this check", pkg.path, path)
			continue
		}
		if name != "_" {
			m[name] = path
		}
	}
	pkg.imports[file] = m
	return m
}

// reference records that the contract surface reaches path.name and walks it.
func (s *surfaceScanner) reference(path, name, from string) {
	switch {
	case isHostContract(path):
		return // scanned as a root
	case isStdlib(path):
		return
	}
	key := path + "." + name
	s.internal[key] = append(s.internal[key], from)
	if isFluxPackage(path) {
		s.visit(s.load(path), name, key)
	}
}

// local handles an unqualified identifier declared in pkg.
func (s *surfaceScanner) local(pkg *surfacePkg, name, from string) {
	if _, ok := pkg.decls[name]; !ok {
		return // predeclared identifier or type parameter
	}
	if isHostContract(pkg.path) {
		s.visit(pkg, name, from)
		return
	}
	s.reference(pkg.path, name, from)
}

func (s *surfaceScanner) visit(pkg *surfacePkg, name, from string) {
	key := pkg.path + "." + name
	if s.visited[key] {
		return
	}
	s.visited[key] = true
	d, ok := pkg.decls[name]
	if !ok {
		s.t.Errorf("%s references %s, which is not a top-level declaration", from, key)
		return
	}
	switch {
	case d.typeSpec != nil:
		if d.typeSpec.TypeParams != nil {
			s.walkType(pkg, d.file, d.typeSpec.TypeParams, key)
		}
		s.walkType(pkg, d.file, d.typeSpec.Type, key)
		for _, m := range pkg.methods[name] {
			if m.funcDecl.Name.IsExported() {
				s.walkType(pkg, m.file, m.funcDecl.Type, key+"."+m.funcDecl.Name.Name)
			}
		}
	case d.funcDecl != nil:
		s.walkType(pkg, d.file, d.funcDecl.Type, key)
	case d.valueSpec != nil:
		switch {
		case d.valueSpec.Type != nil:
			s.walkType(pkg, d.file, d.valueSpec.Type, key)
		case d.index < len(d.valueSpec.Values):
			s.walkValue(pkg, d.file, d.valueSpec.Values[d.index], key)
		}
		// An untyped spec without values repeats the previous const spec,
		// which is visited on its own.
	}
}

// walkType follows every named type in a type expression, skipping
// unexported struct fields, which are not part of the surface.
func (s *surfaceScanner) walkType(pkg *surfacePkg, file *ast.File, expr ast.Node, from string) {
	ast.Inspect(expr, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.StructType:
			for _, f := range n.Fields.List {
				if len(f.Names) > 0 && !anyExported(f.Names) {
					continue
				}
				s.walkType(pkg, file, f.Type, from)
			}
			return false
		case *ast.Field:
			s.walkType(pkg, file, n.Type, from)
			return false
		case *ast.SelectorExpr:
			id, ok := n.X.(*ast.Ident)
			if !ok {
				s.t.Errorf("%s: unexpected qualified type %T", from, n.X)
				return false
			}
			path, ok := s.importsOf(pkg, file)[id.Name]
			if !ok {
				s.t.Errorf("%s: cannot resolve package qualifier %q; teach assumedPackageName about it", from, id.Name)
				return false
			}
			s.reference(path, n.Sel.Name, from)
			return false
		case *ast.Ident:
			s.local(pkg, n.Name, from)
			return false
		}
		return true
	})
}

// walkValue follows the declared type of an initializer for a var or const
// declared without an explicit type.
func (s *surfaceScanner) walkValue(pkg *surfacePkg, file *ast.File, expr ast.Expr, from string) {
	switch e := expr.(type) {
	case *ast.BasicLit:
	case *ast.Ident:
		s.local(pkg, e.Name, from)
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			if path, ok := s.importsOf(pkg, file)[id.Name]; ok {
				s.reference(path, e.Sel.Name, from)
				return
			}
		}
		s.t.Errorf("%s: cannot type initializer %s; declare the value with an explicit type", from, exprString(s.fset, e))
	case *ast.CallExpr:
		s.walkValue(pkg, file, e.Fun, from)
	case *ast.CompositeLit:
		s.walkType(pkg, file, e.Type, from)
	case *ast.FuncLit:
		s.walkType(pkg, file, e.Type, from)
	case *ast.ParenExpr:
		s.walkValue(pkg, file, e.X, from)
	case *ast.UnaryExpr:
		s.walkValue(pkg, file, e.X, from)
	case *ast.BinaryExpr:
		s.walkValue(pkg, file, e.X, from)
		s.walkValue(pkg, file, e.Y, from)
	default:
		s.t.Errorf("%s: unsupported initializer %s; declare the value with an explicit type", from, exprString(s.fset, e))
	}
}

func exprString(fset *token.FileSet, e ast.Expr) string {
	return fset.Position(e.Pos()).String()
}

func anyExported(names []*ast.Ident) bool {
	for _, n := range names {
		if n.IsExported() {
			return true
		}
	}
	return false
}

func (s *surfaceScanner) scanContract() {
	for _, path := range hostContractPackages {
		pkg := s.load(path)
		names := make([]string, 0, len(pkg.decls))
		for name := range pkg.decls {
			if ast.IsExported(name) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			s.visit(pkg, name, path+"."+name)
		}
	}
}

func TestHostContractExposesOnlyFrozenInternalSymbols(t *testing.T) {
	s := newSurfaceScanner(t)
	s.scanContract()

	frozen := map[string]bool{}
	for _, sym := range frozenInternalSymbols {
		frozen[sym] = true
	}
	var reached []string
	for sym := range s.internal {
		reached = append(reached, sym)
	}
	sort.Strings(reached)
	for _, sym := range reached {
		if !frozen[sym] {
			from := s.internal[sym]
			sort.Strings(from)
			t.Errorf("engine-internal symbol %s is reachable from the host contract via %s: stop exposing it, or freeze it in frozenInternalSymbols and HOST-ENGINE-BOUNDARY.md", sym, strings.Join(from, ", "))
		}
	}
	for _, sym := range frozenInternalSymbols {
		if _, ok := s.internal[sym]; !ok {
			t.Errorf("%s is frozen but no longer reachable from the host contract: remove it from frozenInternalSymbols and HOST-ENGINE-BOUNDARY.md", sym)
		}
	}
	if !sort.StringsAreSorted(frozenInternalSymbols) {
		t.Error("keep frozenInternalSymbols sorted")
	}
}

func TestFrozenInternalSymbolsAreDocumented(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fluxRoot, "docs", "architecture", "HOST-ENGINE-BOUNDARY.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, sym := range frozenInternalSymbols {
		short := "`" + strings.TrimPrefix(sym, fluxModule+"/") + "`"
		if !strings.Contains(doc, short) {
			t.Errorf("HOST-ENGINE-BOUNDARY.md does not list frozen symbol %s", short)
		}
	}
}
