// Package release reads release.conf — the only place the product name and version live
// (docs/contract.md §1). Nothing about the release is compiled into the program.
package release

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nishannorunobi/updown/internal/kv"
)

// Info is what release.conf says this package is.
type Info struct {
	ProductName string // PRODUCT_NAME
	Version     string // VERSION
	Target      string // TARGET — "dev" when run from the source tree
	Release     string // RELEASE — PRODUCT_NAME-VERSION-TARGET unless set
	Build       string // BUILD — the program, relative to the package
	Path        string // the release.conf that was read
}

// Find returns the release.conf to use: next to the package (exeDir/../release.conf for
// bin/updown), then next to the program, then the working directory (source tree).
func Find(exeDir, workDir string) (string, error) {
	for _, p := range []string{
		filepath.Join(exeDir, "..", "release.conf"),
		filepath.Join(exeDir, "release.conf"),
		filepath.Join(workDir, "release.conf"),
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return filepath.Clean(p), nil
		}
	}
	return "", errors.New("release.conf not found next to the program or in the working directory")
}

// Load reads and validates one release.conf.
func Load(path string) (Info, error) {
	m, err := kv.ParseFile(path)
	if err != nil {
		return Info{}, err
	}
	in := Info{
		ProductName: m["PRODUCT_NAME"],
		Version:     m["VERSION"],
		Target:      m["TARGET"],
		Release:     m["RELEASE"],
		Build:       m["BUILD"],
		Path:        path,
	}
	if in.ProductName == "" || in.Version == "" {
		return Info{}, fmt.Errorf("%s: PRODUCT_NAME and VERSION are required", path)
	}
	if in.Target == "" {
		in.Target = "dev"
	}
	if in.Release == "" {
		in.Release = in.ProductName + "-" + in.Version + "-" + in.Target
	}
	return in, nil
}
