// Command mkzip packs one release folder into a zip — a build-time tool (standard library
// only, so the build stays offline and needs nothing installed in the image).
//
//	go run ./tools/mkzip <folder> <out.zip>
//
// The folder's own name is the top directory inside the zip. Executable bits are kept
// (start.sh, bin/updown), so an unzip on Linux/macOS gives runnable files.
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mkzip <folder> <out.zip>")
		os.Exit(2)
	}
	if err := pack(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "mkzip:", err)
		os.Exit(1)
	}
}

func pack(dir, out string) error {
	dir = filepath.Clean(dir)
	parent := filepath.Dir(dir)
	tmp := out + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(parent, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			h.Name += "/"
			_, err = zw.CreateHeader(h)
			return err
		}
		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if err := zw.Close(); walkErr == nil {
		walkErr = err
	}
	if err := f.Close(); walkErr == nil {
		walkErr = err
	}
	if walkErr != nil {
		os.Remove(tmp)
		return walkErr
	}
	return os.Rename(tmp, out)
}
