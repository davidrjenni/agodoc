// Copyright (c) 2014 David R. Jenni. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
agodoc is a wrapper around godoc for use with Acme.
It shows the documentation of the identifier under the cursor.
*/
package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/token"
	"io"
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

	filename, off, err := selection(win)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get selection: %v\n", err)
		os.Exit(1)
	}

	prg, err := loadProgram()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot load program: %v\n", err)
		os.Exit(1)
	}

	obj, err := searchObject(filename, prg, off)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot find object: %v\n", err)
		os.Exit(1)
	}

	switch x := obj.(type) {
	case *types.Builtin:
		godoc("builtin", obj.Name())
	case *types.PkgName:
		godoc(x.Imported().Path())
	case *types.Const, *types.Func, *types.TypeName, *types.Var:
		if !x.Exported() {
			fmt.Fprintf(os.Stderr, "cannot print documentation of unexported identifier %s\n", obj.Name())
			os.Exit(1)
		}
		fmt.Println(obj.Pkg().Name())
		godoc(obj.Pkg().Path(), obj.Name())
	default:
		fmt.Fprintf(os.Stderr, "cannot print documentation of %v\n", obj)
		os.Exit(1)
	}
}

func openWin() (*acme.Win, error) {
	id, err := strconv.Atoi(os.Getenv("winid"))
	if err != nil {
		return nil, err
	}
	return acme.Open(id, nil)
}

func selection(win *acme.Win) (filename string, off int, err error) {
	filename, err = readFilename(win)
	if err != nil {
		return "", 0, err
	}
	q0, _, err := readAddr(win)
	if err != nil {
		return "", 0, err
	}
	off, err = byteOffset(bufio.NewReader(&bodyReader{win}), q0)
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

func loadProgram() (*loader.Program, error) {
	var conf loader.Config
	files, err := filepath.Glob("*.go")
	if err != nil {
		return nil, err
	}
	conf.CreateFromFilenames("", files...)
	return conf.Load()
}

func searchObject(filename string, prg *loader.Program, off int) (types.Object, error) {
	filename = filepath.Base(filename)
	info := prg.Created[0]
	var file *ast.File
	found := false
	for _, file = range info.Files {
		f := prg.Fset.File(file.Pos())
		if f.Name() == filename {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("file not found")
	}
	ident := identAtOffset(prg.Fset, file, off)
	if ident == nil {
		return nil, fmt.Errorf("no identifier here")
	}
	if obj := info.Uses[ident]; obj != nil {
		return obj, nil

	}
	if obj := info.Defs[ident]; obj != nil {
		return obj, nil
	}
	return nil, fmt.Errorf("cannot find identifier %s in file", ident.Name)
}

func identAtOffset(fset *token.FileSet, f *ast.File, off int) (ident *ast.Ident) {
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			pos := fset.Position(id.Pos()).Offset
			if pos <= off && off < pos+len(id.Name) {
				ident = id
			}
		}
		return ident == nil
	})
	return ident
}

func godoc(args ...string) {
	c := exec.Command("godoc", args...)
	c.Stderr, c.Stdout = os.Stderr, os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "godoc failed: %v\n", err)
		os.Exit(1)
	}
}
