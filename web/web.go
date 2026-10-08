// Package web holds the phone page, embedded into the program so a release needs no
// separate files and never loads anything from the internet (docs/contract.md §7).
package web

import "embed"

// Index is the phone page, served at GET /.
//
//go:embed index.html
var Index []byte

// Host is the PC's host page (QR code + live log), served at GET /host.
//
//go:embed host.html
var Host []byte

// Parts are the host page's blocks (connection, QR codes, transfer log …), one file each, loaded into the
// page's <div data-part="NAME.html"> through GET /host/part/NAME.html. A new block = a new file here.
//
//go:embed parts/*.html
var Parts embed.FS
