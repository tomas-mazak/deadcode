package state

type PackageState struct {
	name        string          // package name (as defined by the `package` directive)
	fsPath      string          // local filesystem path to the package directory
	imported    bool            // is this package imported in another package?
	identifiers map[string]bool // "usage" status of all exported identifiers declared in the package
}

func (ps *PackageState) Name() string {
	return ps.name
}

func (ps *PackageState) FsPath() string {
	return ps.fsPath
}

func (ps *PackageState) IsImported() bool {
	return ps.imported
}

func (ps *PackageState) MarkImported() {
	ps.imported = true
}

func (ps *PackageState) UnusedIdentifiers() []string {
	var unusedIdentifiers []string

	for identifier, used := range ps.identifiers {
		if !used {
			unusedIdentifiers = append(unusedIdentifiers, identifier)
		}
	}

	return unusedIdentifiers
}

func (ps *PackageState) MarkIdentifierUsed(identifier string) {
	if _, ok := ps.identifiers[identifier]; ok {
		ps.identifiers[identifier] = true
	}
}

type State struct {
	packages map[string]*PackageState
}

func New() *State {
	return &State{
		packages: make(map[string]*PackageState),
	}
}

func (s *State) Packages() map[string]*PackageState {
	return s.packages
}

func (s *State) GetPackageState(pkg string) (pkgState *PackageState, ok bool) {
	pkgState, ok = s.packages[pkg]
	return
}

func (s *State) GetUnusedPackages() []string {
	var unusedPackages []string
	for pkg, pkgState := range s.packages {
		if !pkgState.imported {
			unusedPackages = append(unusedPackages, pkg)
		}
	}
	return unusedPackages
}

func (s *State) NewIdentifier(fsPath, pkgPath, pkgName, name string) {
	// only consider exported identifiers - must begin with a capital letter
	if !isExported(name) {
		return
	}

	//log.Printf("DEBUG: fsPath=%v pkgPath=%v pkgName=%v name=%v", fsPath, pkgPath, pkgName, name)

	if _, ok := s.packages[pkgPath]; !ok {
		s.packages[pkgPath] = &PackageState{
			name:        pkgName,
			fsPath:      fsPath,
			imported:    false,
			identifiers: map[string]bool{},
		}
	}

	s.packages[pkgPath].identifiers[name] = false
}

func isExported(name string) bool {
	if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
		return true
	}
	return false
}
