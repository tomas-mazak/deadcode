package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tomas-mazak/deadcode/discover"
	"github.com/tomas-mazak/deadcode/state"
	"github.com/tomas-mazak/deadcode/usage"
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
	//log.Printf("DEBUG: path=%s", basePath)

	moduleName, err := getModuleName(basePath)
	if err != nil {
		panic(err)
	}

	state := state.New()

	discover.DiscoverPackages(state, basePath, moduleName)

	usageAnalyzer := usage.New(state, samePackageOk)
	usageAnalyzer.RecordUsage(basePath, moduleName, moduleName)

	reportResults(state)
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

func reportResults(state *state.State) {

	for _, pkg := range state.GetUnusedPackages() {
		fmt.Printf("PACKAGE NOT IMPORTED: pkg=%v\n", pkg)
	}

	for pkgPath, pkg := range state.Packages() {
		if pkg.IsImported() {
			for _, identifier := range pkg.UnusedIdentifiers() {
				fmt.Printf("IDENTIFIER UNUSED: pkg=%v identifier=%v\n", pkgPath, identifier)
			}
		}
	}
}
