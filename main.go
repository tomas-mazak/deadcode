package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"

	"golang.org/x/mod/modfile"
)

var samePackageOk bool = true
var basePath string

func init() {
	flag.Parse()
	if flag.NArg() == 0 {
		basePath = "."
	} else {
		basePath = flag.Arg(0)
	}
}

func main() {
	log.Printf("DEBUG: path=%s", basePath)

	moduleName, err := getModuleName(basePath)
	if err != nil {
		panic(err)
	}

	findExportedIdentifiers(basePath, moduleName)

	traceUsage(basePath, moduleName, moduleName)

	analyzeResults()
}

func analyzeResults() {
	for pkgPath, pkg := range packages {
		if !pkg.imported {
			fmt.Printf("PACKAGE NOT IMPORTED: pkg=%v\n", pkgPath)
		} else {
			for identifier, used := range pkg.identifiers {
				if !used {
					fmt.Printf("IDENTIFIER UNUSED: pkg=%v identifier=%v\n", pkgPath, identifier)
				}
			}
		}
	}
}

var visitedPackages map[string]bool = map[string]bool{}

func traceUsage(fsPath, moduleName, pkgName string) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, fsPath, nil, 0)
	if err != nil {
		return err
	}

	for p, pkg := range pkgs {
		if !strings.HasSuffix(p, "_test") {
			for _, file := range pkg.Files {
				//fmt.Printf("DEBUG: tracing %v\n", file.Name.Name)
				traceFile(file, moduleName, pkgName)
			}
		}
	}

	return nil
}

func traceFile(file *ast.File, moduleName, pkgName string) {
	//fmt.Printf("DEBUG: tracing file %v\n", file.Name.Name)

	importMap := make(map[string]string)
	unqualifiedImports := []string{}

	for _, imp := range file.Imports {
		impPath := strings.Trim(imp.Path.Value, "\"")
		if pkgInfo, ok := packages[impPath]; ok {
			pkgInfo.imported = true
			//fmt.Printf("DEBUG: found import %v, pkgInfo.name=%v, imp.Name=%v\n", impPath, pkgInfo.name, imp.Name)
			switch {
			case imp.Name == nil:
				importMap[pkgInfo.name] = impPath
			case imp.Name.Name == "_":
				break
			case imp.Name.Name == ".":
				unqualifiedImports = append(unqualifiedImports, impPath)
			default:
				importMap[imp.Name.Name] = impPath
			}
		}
	}

	var traversalStack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		// resolve usage of qualified symbols in imported packages
		if selectorExpr, ok := n.(*ast.SelectorExpr); ok {
			// only consider Ident.Ident expressions (otherwise we could also match FuncCall().Ident)
			if pkg, ok := selectorExpr.X.(*ast.Ident); ok {
				symbol := selectorExpr.Sel.Name
				if _, ok := importMap[pkg.Name]; ok {
					//fmt.Printf("DEBUG: found usage of %v.%v in %v\n", pkg, symbol, file.Name)
					packages[importMap[pkg.Name]].identifiers[symbol] = true
				}
			}
		}

		// Resolve unqualified identifiers: declared in the same package or imported using dot imports
		if ident, ok := n.(*ast.Ident); ok {
			skip := false

			switch parent := traversalStack[len(traversalStack)-1].(type) {
			case *ast.SelectorExpr:
				// Our symbol is qualified, this is handled above
				skip = true
			case *ast.TypeSpec:
				// Our symbol is a type name in a type declaration
				if parent.Name == n {
					skip = true
				}
			case *ast.FuncDecl:
				// Our symbol is a function name in a function declaration
				if parent.Name == n {
					skip = true
				}
			case *ast.Field:
				// Our symbol is a field name in a struct or interface declaration
				for _, fname := range parent.Names {
					if n == fname {
						skip = true
					}
				}
			case *ast.ValueSpec:
				// Our symbol is a variable name in a variable declaration
				for _, varname := range parent.Names {
					if n == varname {
						skip = true
					}
				}
			case *ast.AssignStmt:
				// Our symbol is on a left-hand side of short declaration
				if parent.Tok.String() == ":=" {
					for _, varname := range parent.Lhs {
						if n == varname {
							skip = true
						}
					}
				}
				// Known corner cases:
				// - short declarations where not all variables on LHS are being declared
				// - usage of local identifiers with the same name as global identifiers would mark the global
				//   identifier as used, even if it wasn't. Although if the user declares local identifiers
				//   starting with capital letters, it's his fault, not mine
			}

			// Symbol is not being declared, so we assume it is being used. It is not qualified, so it could
			// either be declared in the current package or in one of the dot-imported packages
			if !skip {
				symbol := ident.Name
				for _, pkg := range unqualifiedImports {
					if _, ok := packages[pkg].identifiers[symbol]; ok {
						//fmt.Printf("DEBUG: unqualified symbol %v in %v\n", symbol, file.Name.Name)
						packages[pkg].identifiers[symbol] = true
					}
				}
				if samePackageOk {
					if _, ok := packages[pkgName]; ok {
						if _, ok := packages[pkgName].identifiers[symbol]; ok {
							packages[pkgName].identifiers[symbol] = true
						}
					}
				}
			}
		}

		// Manage the stack. Inspect calls a function like this:
		//   f(node)
		//   for each child {
		//      f(child) // and recursively for child's children
		//   }
		//   f(nil)
		// source: https://stackoverflow.com/a/66810485
		if n == nil {
			// Done with node's children. Pop.
			traversalStack = traversalStack[:len(traversalStack)-1]
		} else {
			// Push the current node for children.
			traversalStack = append(traversalStack, n)
		}
		return true
	})

	for pkg := range importMap {
		if _, ok := visitedPackages[importMap[pkg]]; !ok {
			//fmt.Printf("DEBUG: continuing with %v\n", packages[importMap[pkg]].fsPath)
			visitedPackages[importMap[pkg]] = true
			traceUsage(packages[importMap[pkg]].fsPath, moduleName, importMap[pkg])
		}
	}
	for _, pkg := range unqualifiedImports {
		if _, ok := visitedPackages[pkg]; !ok {
			//fmt.Printf("DEBUG: continuing with %v\n", packages[pkg].fsPath)
			visitedPackages[pkg] = true
			traceUsage(packages[pkg].fsPath, moduleName, pkg)
		}
	}
}

func getModuleName(path string) (string, error) {
	data, err := os.ReadFile(path + "/go.mod")
	if err != nil {
		return "", err
	}

	mf, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return "", err
	}

	return mf.Module.Mod.Path, nil
}

func findExportedIdentifiers(fsPath, importPath string) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, fsPath, nil, 0)
	if err != nil {
		return err
	}

	for p, pkg := range pkgs {
		if !strings.HasSuffix(p, "_test") {
			analyzePackage(fsPath, importPath, pkg)
		}
	}

	entries, err := os.ReadDir(fsPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			findExportedIdentifiers(fsPath+"/"+entry.Name(), importPath+"/"+entry.Name())
		}
	}

	return nil
}

func analyzePackage(fsPath, pkgPath string, pkg *ast.Package) {
	for _, file := range pkg.Files {
		analyzeFile(file, fsPath, pkgPath, pkg.Name)
	}
}

func analyzeFile(file *ast.File, fsPath, pkgPath, pkgName string) {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if ok {
			registerIdentifier(fsPath, pkgPath, pkgName, funcDecl.Name.Name)
		}
		genDec, ok := decl.(*ast.GenDecl)
		if ok {
			for _, spec := range genDec.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok {
					registerIdentifier(fsPath, pkgPath, pkgName, typeSpec.Name.Name)
				}

				valueSpec, ok := spec.(*ast.ValueSpec)
				if ok {
					for _, ident := range valueSpec.Names {
						registerIdentifier(fsPath, pkgPath, pkgName, ident.Name)
					}
				}
			}
		}
	}
}

type packageInfo struct {
	name        string
	fsPath      string
	imported    bool
	identifiers map[string]bool
}

var packages map[string]*packageInfo = map[string]*packageInfo{}

func registerIdentifier(fsPath, pkgPath, pkgName, name string) {
	// only consider exported identifiers - must begin with a capital letter
	if !isExported(name) {
		return
	}

	//log.Printf("DEBUG: fsPath=%v pkgPath=%v pkgName=%v name=%v", fsPath, pkgPath, pkgName, name)

	if _, ok := packages[pkgPath]; !ok {
		packages[pkgPath] = &packageInfo{
			name:        pkgName,
			fsPath:      fsPath,
			imported:    false,
			identifiers: map[string]bool{},
		}
	}

	packages[pkgPath].identifiers[name] = false
}

func isExported(name string) bool {
	if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
		return true
	}
	return false
}
