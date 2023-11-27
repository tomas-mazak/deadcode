package discover

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/tomas-mazak/deadcode/state"
)

func DiscoverPackages(usage *state.State, fsPath, importPath string) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, fsPath, nil, 0)
	if err != nil {
		return err
	}

	for p, pkg := range pkgs {
		if !strings.HasSuffix(p, "_test") {
			analyzePackage(usage, fsPath, importPath, pkg)
		}
	}

	entries, err := os.ReadDir(fsPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			DiscoverPackages(usage, fsPath+"/"+entry.Name(), importPath+"/"+entry.Name())
		}
	}

	return nil
}

func analyzePackage(usage *state.State, fsPath, pkgPath string, pkg *ast.Package) {
	for _, file := range pkg.Files {
		analyzeFile(usage, file, fsPath, pkgPath, pkg.Name)
	}
}

func analyzeFile(usage *state.State, file *ast.File, fsPath, pkgPath, pkgName string) {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if ok {
			usage.NewIdentifier(fsPath, pkgPath, pkgName, funcDecl.Name.Name)
		}
		genDec, ok := decl.(*ast.GenDecl)
		if ok {
			for _, spec := range genDec.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok {
					usage.NewIdentifier(fsPath, pkgPath, pkgName, typeSpec.Name.Name)
				}

				valueSpec, ok := spec.(*ast.ValueSpec)
				if ok {
					for _, ident := range valueSpec.Names {
						usage.NewIdentifier(fsPath, pkgPath, pkgName, ident.Name)
					}
				}
			}
		}
	}
}
