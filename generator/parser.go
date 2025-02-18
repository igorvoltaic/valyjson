package generator

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"

	"github.com/iv-menshenin/go-ast/explorer"
	"github.com/iv-menshenin/valyjson/generator/discoverer"
	"github.com/iv-menshenin/valyjson/generator/static"
)

type (
	Gen struct {
		fileName string
		parsed   ast.Node
		result   ast.File
		packageN string

		discovery *discoverer.Discoverer
	}
)

func (g *Gen) Parse() (err error) {
	if err = g.discovery.Discover(); err != nil {
		panic(err)
	}
	g.parsed, err = parseGo(g.fileName)
	if err != nil {
		return err
	}
	g.packageN = g.parsed.(*ast.File).Name.Name
	if err == nil {
		g.result.Name = g.parsed.(*ast.File).Name
	}
	return
}

func (g *Gen) FixImports(internals ...string) {
	// discovery used imports and build their declaration
	for i := 0; i < len(internals); i += 2 {
		explorer.RegisterPackage(internals[i], explorer.Package{
			Path: internals[i+1],
			Kind: explorer.PkgKindInternal,
		})
	}
	discovery := explorer.New()
	discovery.Explore(&g.result)
	var decls = make([]ast.Decl, 0, len(g.result.Decls)+1)

	imports := discovery.ImportSpec()
	if len(imports) > 0 {
		decls = append(decls, &ast.GenDecl{
			Tok:   token.IMPORT,
			Specs: discovery.ImportSpec(),
		})
	}
	decls = append(decls, g.result.Decls...)
	g.result.Decls = decls
}

func (g *Gen) Print(name string) {
	f, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	defer fmtGOFile(name)
	defer f.Close()
	_, err = fmt.Fprint(f, "// Code generated [github.com/iv-menshenin/valyjson]; DO NOT EDIT.\n")
	if err != nil {
		panic(err)
	}

	if err := printer.Fprint(f, token.NewFileSet(), &g.result); err != nil {
		panic(err)
	}
	err = static.Print(static.Context{
		Package: g.packageN,
	}, path.Dir(name))
	if err != nil {
		panic(err)
	}
}

func fmtGOFile(fileName string) (err error) {
	var fileSet = token.NewFileSet()
	f, err := parser.ParseFile(fileSet, fileName, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	out, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer func() {
		if e := out.Close(); e != nil && err == nil {
			err = e
		}
	}()
	return format.Node(out, fileSet, f)
}

func parseGo(file string) (ast.Node, error) {
	f := token.NewFileSet()
	return parser.ParseFile(f, file, nil, parser.ParseComments|parser.AllErrors)
}

func New(file string) *Gen {
	return &Gen{
		fileName:  file,
		discovery: discoverer.New(path.Dir(file)),
	}
}
