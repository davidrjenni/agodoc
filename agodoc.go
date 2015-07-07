// Copyright (c) 2014 David R. Jenni. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
agodoc is a wrapper around godoc for use with Acme.
It shows the documentation of the identifier under the cursor.
*/
package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"9fans.net/go/acme"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
)

type bodyReader struct{ *acme.Win }

func (r bodyReader) Read(data []byte) (int, error) {
	return r.Win.Read("body", data)
}

func main() {
	win, err := openWin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open window: %v\n", err)
		os.Exit(1)
	}
	buf, err := ioutil.ReadAll(&bodyReader{win})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read content: %v\n", err)
		os.Exit(1)
	}
	filename, off, err := selection(win, buf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get selection: %v\n", err)
		os.Exit(1)
	}

	fileInfos, err := ioutil.ReadDir(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read directory: %v\n", err)
		os.Exit(1)
	}
	var filenames []string
	for _, f := range fileInfos {
		if strings.HasSuffix(f.Name(), ".go") && f.Name() != filename {
			filenames = append(filenames, f.Name())
		}
	}
	path, ident, err := searchAtOff(off, string(buf), filenames...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot find identifier: %v\n", err)
		os.Exit(1)
	}
	if ident != "" {
		godoc(path, ident)
	} else {
		godoc(path)
	}
}

func openWin() (*acme.Win, error) {
	id, err := strconv.Atoi(os.Getenv("winid"))
	if err != nil {
		return nil, err
	}
	return acme.Open(id, nil)
}

func selection(win *acme.Win, buf []byte) (filename string, off int, err error) {
	filename, err = readFilename(win)
	if err != nil {
		return "", 0, err
	}
	filename = filepath.Base(filename)
	q0, _, err := readAddr(win)
	if err != nil {
		return "", 0, err
	}
	off, err = byteOffset(bytes.NewReader(buf), q0)
	if err != nil {
		return "", 0, err
	}
	return
}

func readFilename(win *acme.Win) (string, error) {
	b, err := win.ReadAll("tag")
	if err != nil {
		return "", err
	}
	tag := string(b)
	i := strings.Index(tag, " ")
	if i == -1 {
		return "", fmt.Errorf("cannot get filename from tag")
	}
	return tag[0:i], nil
}

func readAddr(win *acme.Win) (q0, q1 int, err error) {
	if _, _, err := win.ReadAddr(); err != nil {
		return 0, 0, err
	}
	if err := win.Ctl("addr=dot"); err != nil {
		return 0, 0, err
	}
	return win.ReadAddr()
}

func byteOffset(r io.RuneReader, off int) (bo int, err error) {
	for i := 0; i != off; i++ {
		_, s, err := r.ReadRune()
		if err != nil {
			return 0, err
		}
		bo += s
	}
	return
}

// searchAtOff returns the path and the identifier name at the given offset.
func searchAtOff(off int, src string, filenames ...string) (string, string, error) {
	var conf loader.Config
	var files []*ast.File

	for _, name := range filenames {
		f, err := parseFile(&conf, name, nil)
		if err != nil {
			return "", "", err
		}
		files = append(files, f)
	}
	f, err := parseFile(&conf, "", src)
	if err != nil {
		return "", "", err
	}
	files = append(files, f)

	for _, imp := range f.Imports {
		if isAtOff(conf.Fset, off, imp) {
			return strings.Trim(imp.Path.Value, "\""), "", nil
		}
	}

	ident := identAtOffset(conf.Fset, f, off)
	if ident == nil {
		return "", "", errors.New("no identifier here")
	}

	conf.CreateFromFiles("", files...)
	prg, err := conf.Load()
	if err != nil {
		return "", "", err
	}
	info := prg.Created[0]
	if obj := info.Uses[ident]; obj != nil {
		return fromObj(obj, conf.Fset, f, off)
	}
	if obj := info.Defs[ident]; obj != nil {
		return fromObj(obj, conf.Fset, f, off)
	}
	return "", "", errors.New("could not find identifier")
}

func parseFile(conf *loader.Config, name string, src interface{}) (*ast.File, error) {
	f, err := conf.ParseFile(name, src)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(f.Name.Name, "_test") {
		f.Name.Name = f.Name.Name[:len(f.Name.Name)-len("_test")]
	}
	return f, nil
}

// currImportDir returns the current import directory.
func currImportDir() (string, error) {
	path, err := os.Getwd()
	if err != nil {
		return "", err
	}
	pkg, err := build.ImportDir(path, build.ImportComment)
	if err != nil {
		return "", err
	}
	return pkg.ImportPath, nil
}

// fromObj returns the path and the identifier name of an object.
func fromObj(obj types.Object, fset *token.FileSet, f *ast.File, off int) (string, string, error) {
	switch o := obj.(type) {
	case *types.Builtin:
		return "builtin", o.Name(), nil
	case *types.PkgName:
		return o.Imported().Path(), "", nil
	case *types.Const, *types.Func, *types.Nil, *types.TypeName, *types.Var:
		if obj.Pkg() == nil {
			return "builtin", obj.Name(), nil
		}
		if !isImported(fset, f, off) {
			impDir, err := currImportDir()
			return impDir, obj.Name(), err
		}
		return obj.Pkg().Path(), obj.Name(), nil
	default:
		return "", "", fmt.Errorf("cannot print documentation of %v\n", obj)
	}
}

// isImported returns true whether the identifier at the given offset is imported or not.
func isImported(fset *token.FileSet, f *ast.File, off int) (isImp bool) {
	ast.Inspect(f, func(node ast.Node) bool {
		switch s := node.(type) {
		case *ast.SelectorExpr:
			if isAtOff(fset, off, s.Sel) {
				if ident, ok := s.X.(*ast.Ident); ok {
					for _, imp := range f.Imports {
						if imp.Name != nil && imp.Name.Name == ident.Name {
							isImp = true
							return false
						}
						if strings.Trim(imp.Path.Value, "\"") == ident.Name {
							isImp = true
						}
					}
				}
				return false
			}
		}
		return true
	})
	return isImp
}

// identAtOffset returns the identifier at an offset in a file.
func identAtOffset(fset *token.FileSet, f *ast.File, off int) (ident *ast.Ident) {
	ast.Inspect(f, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			if isAtOff(fset, off, node) {
				ident = n
				return false
			}
		}
		return true
	})
	return ident
}

// isAtOff reports whether a node is at a given offset in a file set.
func isAtOff(fset *token.FileSet, off int, n ast.Node) bool {
	return fset.Position(n.Pos()).Offset <= off && off <= fset.Position(n.End()).Offset
}

// godoc runs the godoc command with the given arguments.
func godoc(args ...string) {
	c := exec.Command("godoc", args...)
	c.Stderr, c.Stdout = os.Stderr, os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "godoc failed: %v\n", err)
		os.Exit(1)
	}
}
