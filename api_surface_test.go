package prettyview

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestExportedSurfaceGolden freezes the public API of the /v2 module. It enumerates
// every exported identifier of prettyview and fonttheme (via go/parser — no go/packages,
// no x/tools dependency, and no type-checking, so it never imports internal/ Fyne
// packages) and compares a normalized signature listing to testdata/api_surface.txt.
// Any addition, removal, rename, or signature change fails until the golden is
// regenerated:
//
//	UPDATE_API_SURFACE=1 go test -run TestExportedSurfaceGolden .
//
// During v2.0.0 development the surface is still settling, so this golden is re-baselined
// as v2 symbols land. Once v2.0.0 ships, a diff here means a breaking change — it must
// ship under the next major module path (…/v3), not as a v2.x bump. See the Stability
// section in the README.
func TestExportedSurfaceGolden(t *testing.T) {
	var lines []string
	lines = append(lines, exportedSurface(t, ".", "prettyview")...)
	lines = append(lines, exportedSurface(t, "fonttheme", "fonttheme")...)
	sort.Strings(lines)
	got := strings.Join(lines, "\n") + "\n"

	const golden = "testdata/api_surface.txt"
	if os.Getenv("UPDATE_API_SURFACE") == "1" {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d identifiers)", golden, len(lines))
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read %s: %v (run with UPDATE_API_SURFACE=1 to create it)", golden, err)
	}
	if got != string(want) {
		t.Errorf("exported API surface changed.\nIf intentional, regenerate:\n"+
			"  UPDATE_API_SURFACE=1 go test -run TestExportedSurfaceGolden .\n\n%s",
			surfaceDiff(string(want), got))
	}
}

// exportedSurface parses every non-test .go file in dir and returns one normalized
// line per exported identifier (func/method signature, type kind + exported struct
// fields, const/var with type). pkg labels the package in each line.
func exportedSurface(t *testing.T, dir, pkg string) []string {
	t.Helper()
	fset := token.NewFileSet()
	// parser.ParseDir is deprecated; enumerate the non-test .go files ourselves and
	// ParseFile each. We never type-check, so no go/packages (and no internal/ Fyne
	// imports) — the frozen-surface guarantee is unchanged.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, dir+"/"+name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, f)
	}
	render := func(n ast.Node) string {
		var sb strings.Builder
		_ = printer.Fprint(&sb, fset, n)
		return strings.Join(strings.Fields(sb.String()), " ")
	}

	var out []string
	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				sig := strings.TrimPrefix(render(d.Type), "func")
				if d.Recv != nil {
					recv := render(d.Recv.List[0].Type)
					if !receiverExported(recv) {
						continue // method on an unexported type
					}
					out = append(out, pkg+" method ("+recv+") "+d.Name.Name+sig)
				} else {
					out = append(out, pkg+" func "+d.Name.Name+sig)
				}
			case *ast.GenDecl:
				out = append(out, genDeclSurface(pkg, d, render)...)
			}
		}
	}
	return out
}

func genDeclSurface(pkg string, d *ast.GenDecl, render func(ast.Node) string) []string {
	var out []string
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			if st, ok := s.Type.(*ast.StructType); ok {
				var fields []string
				for _, f := range st.Fields.List {
					if len(f.Names) == 0 {
						// Embedded field: its type determines the promoted (host-callable)
						// method set, so it is part of the frozen surface — capture it, or
						// swapping widget.BaseWidget for another embed would pass silently.
						fields = append(fields, "embeds "+render(f.Type))
						continue
					}
					for _, name := range f.Names {
						if name.IsExported() {
							fields = append(fields, name.Name+" "+render(f.Type))
						}
					}
				}
				sort.Strings(fields)
				eq := ""
				if s.Assign.IsValid() {
					eq = "= "
				}
				out = append(out, pkg+" type "+s.Name.Name+" "+eq+"struct { "+strings.Join(fields, "; ")+" }")
			} else {
				eq := ""
				if s.Assign.IsValid() {
					eq = "= "
				}
				out = append(out, pkg+" type "+s.Name.Name+" "+eq+render(s.Type))
			}
		case *ast.ValueSpec:
			kind := "var"
			if d.Tok == token.CONST {
				kind = "const"
			}
			for i, name := range s.Names {
				if !name.IsExported() {
					continue
				}
				line := pkg + " " + kind + " " + name.Name
				if s.Type != nil {
					line += " " + render(s.Type)
				}
				// Capture an explicit const value so repointing it (e.g. FormatAuto to a
				// different model constant) flips the golden. Untyped iota-block ordering
				// is out of scope (an implicit value renders nothing).
				if kind == "const" && i < len(s.Values) {
					line += " = " + render(s.Values[i])
				}
				out = append(out, line)
			}
		}
	}
	return out
}

// receiverExported reports whether a rendered receiver type (e.g. "*PrettyView" or
// "PrettyView") names an exported type.
func receiverExported(recv string) bool {
	recv = strings.TrimPrefix(recv, "*")
	if i := strings.IndexByte(recv, '['); i >= 0 { // strip type params
		recv = recv[:i]
	}
	return recv != "" && ast.IsExported(recv)
}

// surfaceDiff renders a minimal line-wise added/removed summary for the test output.
func surfaceDiff(want, got string) string {
	w := map[string]bool{}
	for _, l := range strings.Split(want, "\n") {
		w[l] = true
	}
	g := map[string]bool{}
	for _, l := range strings.Split(got, "\n") {
		g[l] = true
	}
	var b strings.Builder
	for _, l := range strings.Split(got, "\n") {
		if l != "" && !w[l] {
			b.WriteString("  + " + l + "\n")
		}
	}
	for _, l := range strings.Split(want, "\n") {
		if l != "" && !g[l] {
			b.WriteString("  - " + l + "\n")
		}
	}
	return b.String()
}
