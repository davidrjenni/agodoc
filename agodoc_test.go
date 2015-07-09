// Copyright (c) 2014 David R. Jenni. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"reflect"
	"testing"
)

type offsetTest struct {
	data       []byte
	offset     int
	byteOffset int
}

var offsetTests = []offsetTest{
	{[]byte("abcdef"), 0, 0},
	{[]byte("abcdef"), 1, 1},
	{[]byte("abcdef"), 5, 5},
	{[]byte("日本語def"), 0, 0},
	{[]byte("日本語def"), 1, 3},
	{[]byte("日本語def"), 5, 11},
}

func TestByteOffset(t *testing.T) {
	for _, test := range offsetTests {
		off, err := byteOffset(bytes.NewReader(test.data), test.offset)
		if err != nil {
			t.Errorf("got error %v", err)
		}
		if off != test.byteOffset {
			t.Errorf("expected byte offset %d, got %d", test.byteOffset, off)
		}
	}
}

type empty struct{}

var importDir = reflect.TypeOf(empty{}).PkgPath()

func TestCurrImportPath(t *testing.T) {
	currDir, err := currImportDir()
	if err != nil {
		t.Fatal(err)
	}

	if currDir != importDir {
		t.Fatalf("expected '%s', got '%s'\n", importDir, currDir)
	}
}

var searchTests = []struct {
	prg   string
	off   int
	path  string
	ident string
}{
	// Imports
	{`package main; import _ "io"`, 21, "io", ""},
	{`package main; import _ "io"`, 22, "io", ""},
	{`package main; import _ "io"`, 23, "io", ""},
	{`package main; import _ "io"`, 24, "io", ""},
	{`package main; import _ "io"`, 25, "io", ""},
	{`package main; import _ "io"`, 26, "io", ""},
	{`package main; import _ "io"`, 27, "io", ""},
	{`package main; import foo "io"`, 21, "io", ""},
	{`package main; import _ "text/scanner"`, 27, "text/scanner", ""},
	{`package main; import "io"; func main() { var _ io.Reader } `, 22, "io", ""},
	{`package main; import ( "bytes"; "io" ); func main() { var _ io.Reader; var _ bytes.Buffer } `, 24, "bytes", ""},

	// Builtin type, function, const and variable
	{`package main; func main() { var _ bool }`, 34, "builtin", "bool"},
	{`package main; func main() { _ = make([]byte, 0) }`, 32, "builtin", "make"},
	{`package p; func main() { var _ bool = true }`, 39, "builtin", "true"},
	{`package p; func main() { var _ interface{} = nil }`, 47, "builtin", "nil"},

	// Declaration of a type, function, method, const and variable
	{`package p; type T struct{}`, 16, importDir, "T"},
	{`package p; func F() {}`, 16, importDir, "F"},
	{`package p; type T struct{}; func (_ T) M()`, 39, importDir, "M"},
	{`package p; const C = 42`, 17, importDir, "C"},
	{`package p; var V int = 42`, 15, importDir, "V"},

	// Usage of package local defined type, function, method, const and variable
	{`package p; type T struct{}; func main() { var _ T }`, 48, importDir, "T"},
	{`package p; type T struct{}; func (_ T) m() {}`, 36, importDir, "T"},
	{`package p; type T struct{}; func (_ *T) m() {}`, 37, importDir, "T"},
	{`package p; type T struct{}; type U struct{ t T }`, 46, importDir, "T"},
	{`package p; type T struct{}; type U struct{ T }`, 44, importDir, "T"},
	{`package p; func F() {}; func main() { F() }`, 38, importDir, "F"},
	{`package p; type T struct{}; func (_ T) M() {}; func main() { t := T{}; t.M() }`, 73, importDir, "M"},
	{`package p; const C = 42; func main() { var _ int = C }`, 51, importDir, "C"},
	{`package p; var V = 42; func main() { var _ int = V }`, 49, importDir, "V"},

	// Usage of imported type, function, field, const and variable
	{`package p; import "io"; func main() { var _ io.Reader }`, 46, "io", ""},
	{`package p; import "io"; func main() { var _ io.Reader }`, 47, "io", "Reader"},
	{`package p; import foo "io"; func main() { var _ foo.Reader }`, 52, "io", "Reader"},
	{`package p; import "io"; type T struct{ r io.Reader}`, 44, "io", "Reader"},
	{`package p; import "io"; type U struct{ io.Reader }`, 43, "io", "Reader"},
	{`package p; import "io"; type Reader struct{}; func main() { var _ io.Reader }`, 69, "io", "Reader"},
	{`package p; import "fmt"; func main() { fmt.Println("Hello World!") }`, 43, "fmt", "Println"},
	{`package p; import "math"; func main() { var _ int = math.MaxInt8 }`, 57, "math", "MaxInt8"},
	{`package p; import "net/http"; func main() { var _ int = http.StatusOK }`, 61, "net/http", "StatusOK"},
	{`package p; import "os"; func main() { var _ *os.File = os.Stdout }`, 58, "os", "Stdout"},
}

func TestIdentAtOffset(t *testing.T) {
	for i, test := range searchTests {
		path, ident, err := searchAtOff(test.off, test.prg)
		if err != nil {
			t.Errorf("Test %d: %v", i, err)
			continue
		}
		if path != test.path || ident != test.ident {
			t.Errorf("Test %d: expected '%s' '%s', got '%s' '%s'\n", i, test.path, test.ident, path, ident)
		}
	}
}
