package usage

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/tomas-mazak/deadcode/state"
)

type Usage struct {
	state                *state.State
	visitedPackages      map[string]bool
	recordSamePackageUse bool
}

func New(state *state.State, recordSamePackageUse bool) *Usage {
	return &Usage{
		state:                state,
		visitedPackages:      map[string]bool{},
		recordSamePackageUse: recordSamePackageUse,
	}
}

func (u *Usage) RecordUsage(fsPath, moduleName, pkgName string) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, fsPath, nil, 0)
	if err != nil {
		return err
	}

	for p, pkg := range pkgs {
		if !strings.HasSuffix(p, "_test") {
			for _, file := range pkg.Files {
				//fmt.Printf("DEBUG: tracing %v\n", file.Name.Name)
				u.traceFile(file, moduleName, pkgName)
			}
		}
	}

	return nil
}

func (u *Usage) traceFile(file *ast.File, moduleName, pkgName string) {
	//fmt.Printf("DEBUG: tracing file %v\n", file.Name.Name)

	importMap := make(map[string]string)
	unqualifiedImports := []string{}

	for _, imp := range file.Imports {
		impPath := strings.Trim(imp.Path.Value, "\"")
		if pkgState, ok := u.state.GetPackageState(impPath); ok {
			pkgState.MarkImported()
			//fmt.Printf("DEBUG: found import %v, pkgInfo.name=%v, imp.Name=%v\n", impPath, pkgInfo.name, imp.Name)
			switch {
			case imp.Name == nil:
				importMap[pkgState.Name()] = impPath
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
					if pkgState, ok := u.state.GetPackageState(importMap[pkg.Name]); ok {
						pkgState.MarkIdentifierUsed(symbol)
					}
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
					if pkgState, ok := u.state.GetPackageState(pkg); ok {
						//fmt.Printf("DEBUG: unqualified symbol %v in %v\n", symbol, file.Name.Name)
						pkgState.MarkIdentifierUsed(symbol)
					}
				}
				if u.recordSamePackageUse {
					if pkgState, ok := u.state.GetPackageState(pkgName); ok {
						pkgState.MarkIdentifierUsed(symbol)
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
		if _, ok := u.visitedPackages[importMap[pkg]]; !ok {
			//fmt.Printf("DEBUG: continuing with %v\n", packages[importMap[pkg]].fsPath)
			u.visitedPackages[importMap[pkg]] = true
			if pkgState, ok := u.state.GetPackageState(importMap[pkg]); ok {
				u.RecordUsage(pkgState.FsPath(), moduleName, importMap[pkg])
			}
		}
	}
	for _, pkg := range unqualifiedImports {
		if _, ok := u.visitedPackages[pkg]; !ok {
			//fmt.Printf("DEBUG: continuing with %v\n", packages[pkg].fsPath)
			u.visitedPackages[pkg] = true
			if pkgState, ok := u.state.GetPackageState(pkg); ok {
				u.RecordUsage(pkgState.FsPath(), moduleName, pkg)
			}
		}
	}
}
