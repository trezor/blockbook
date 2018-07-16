// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2014 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

// Package fmtext contains extensions to the base Go fmt package.
package fmtext

import (
	"fmt"
	"io"
	"os"
)

// Scans an integer from the standard input. Panics on failure!
func ScanInt() int {
	return FscanInt(os.Stdin)
}

// Scans a 64 bit float from the standard input. Panics on failure!
func ScanFloat() float64 {
	return FscanFloat(os.Stdin)
}

// Scans a whitespace delimited string from the standard input. Panics on failure!
func ScanString() string {
	return FscanString(os.Stdin)
}

// Scans a string until newline or EOF from the standard input. Panics on failure!
func ScanLine() string {
	return FscanLine(os.Stdin)
}

// Scans an integer from the specified stream. Panics on failure!
func FscanInt(r io.Reader) int {
	var value int
	if n, err := fmt.Fscan(r, &value); n != 1 || err != nil {
		panic(fmt.Sprintf("scan int failed: n = %d, err = %v", n, err))
	}
	return value
}

// Scans a 64 bit float from the specified stream. Panics on failure!
func FscanFloat(r io.Reader) float64 {
	var value float64
	if n, err := fmt.Fscan(r, &value); n != 1 || err != nil {
		panic(fmt.Sprintf("scan float64 failed: n = %d, err = %v", n, err))
	}
	return value
}

// Scans a whitespace delimited string from the specified stream. Panics on failure!
func FscanString(r io.Reader) string {
	var value string
	if n, err := fmt.Fscan(r, &value); n != 1 || err != nil {
		panic(fmt.Sprintf("scan string failed: n = %d, err = %v", n, err))
	}
	return value
}

// Scans a string until newline or EOF from the specified stream. Panics on failure!
func FscanLine(r io.Reader) string {
	line, char := "", ' '
	for {
		n, err := fmt.Fscanf(r, "%c", &char)
		switch {
		case n == 1 && err == nil:
			if char == '\n' {
				return line
			}
			line += string(char)
		case n == 0 && err == io.EOF:
			return line
		default:
			panic(fmt.Sprintf("scan line failed: n = %d, err = %v", n, err))
		}
	}
}
