// Package web holds the phone page, embedded into the program so a release needs no
// separate files and never loads anything from the internet (docs/contract.md §7).
package web

import _ "embed"

// Index is the phone page, served at GET /.
//
//go:embed index.html
var Index []byte

// Host is the PC's host page (QR code + live log), served at GET /host.
//
//go:embed host.html
var Host []byte
