package watcher

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/ivandosevv/stef/daemon/telemetry"
	"github.com/pmezard/go-difflib/difflib"
)

type Symbol struct {
	Code string
	Kind string
	Line int
}

// Extract uses Go's parser, rather than regex, to isolate declarations.
func Extract(name string, content []byte) map[string]Symbol {
	result := make(map[string]Symbol)
	if filepath.Ext(name) != ".go" {
		result[name] = Symbol{Code: string(content), Kind: "file", Line: 1}
		return result
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, content, parser.ParseComments)
	if err != nil {
		result[name] = Symbol{Code: string(content), Kind: "file", Line: 1}
		return result
	}
	add := func(key, kind string, node ast.Node, start token.Pos) {
		a, b := fset.Position(start).Offset, fset.Position(node.End()).Offset
		result[name+"#"+key] = Symbol{Code: string(content[a:b]), Kind: kind, Line: fset.Position(start).Line}
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			key := d.Name.Name
			kind := "function"
			if d.Recv != nil && len(d.Recv.List) > 0 {
				var buf bytes.Buffer
				if format.Node(&buf, fset, d.Recv.List[0].Type) == nil {
					key = buf.String() + "." + key
				}
				kind = "method"
			}
			start := d.Pos()
			if d.Doc != nil {
				start = d.Doc.Pos()
			}
			add(key, kind, d, start)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if len(d.Specs) == 1 {
						add(s.Name.Name, "type", d, d.Pos())
					} else {
						add(s.Name.Name, "type", s, s.Pos())
					}
				case *ast.ValueSpec:
					names := make([]string, 0, len(s.Names))
					for _, n := range s.Names {
						names = append(names, n.Name)
					}
					add(strings.Join(names, ","), d.Tok.String(), s, s.Pos())
				}
			}
			if d.Tok == token.IMPORT {
				add("imports@"+file.Name.Name, "imports", d, d.Pos())
			}
		}
	}
	// Keep a file node too: captures package/comments and holds unrepresented edits.
	result[name] = Symbol{Code: string(content), Kind: "file", Line: 1}
	return result
}

func Diff(name, before, after string) string {
	// difflib's matcher can consume quadratic time on adversarial line patterns.
	// Keep captured before/after previews while refusing an unbounded comparison.
	const maxLines = 4000
	if before != "" && after != "" && (strings.Count(before, "\n") > maxLines || strings.Count(after, "\n") > maxLines) {
		return "Diff unavailable: local comparison exceeds the 4000-line safety limit."
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: diffLines(before), B: diffLines(after), FromFile: "a/" + name, ToFile: "b/" + name, Context: 3})
	if err != nil {
		return "Diff unavailable"
	}
	return Clip(diff)
}

// Unlike difflib.SplitLines, an empty file has no lines and a final LF does not
// create a synthetic blank line. Keep the missing-LF marker with its one line
// token so the matcher notices EOF changes without counting the marker as code.
func diffLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.SplitAfter(value, "\n")
	last := len(lines) - 1
	if lines[last] == "" {
		return lines[:last]
	}
	lines[last] += "\n\\ No newline at end of file\n"
	return lines
}

func Clip(value string) string {
	if len(value) <= telemetry.MaxText {
		return value
	}
	const suffix = "\n… [truncated]"
	cutoff := telemetry.MaxText - len(suffix)
	for cutoff > 0 && value[cutoff]&0xc0 == 0x80 {
		cutoff--
	}
	return value[:cutoff] + suffix
}
