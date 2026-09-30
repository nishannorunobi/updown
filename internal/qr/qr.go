// Package qr is a tiny offline QR code encoder — a Go port of the workspace's
// backup/qr.py (ISO/IEC 18004; structure after Project Nayuki's reference
// implementation). Byte mode, error correction level M, versions 1–10 (up to 213
// bytes — far more than the updown link needs), best of the 8 masks by the standard
// penalty rules. Standard library only.
package qr

import (
	"errors"
	"fmt"
	"strings"
)

// Level M, versions 1..10 (index 0 unused).
var (
	eccPerBlock = [11]int{0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26}
	numBlocks   = [11]int{0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5}
)

const formatBitsM = 0 // L=1, M=0, Q=3, H=2

// ErrTooLong is returned when the text does not fit version 10 at level M.
var ErrTooLong = errors.New("qr: text too long for versions 1-10")

// Encode returns the QR code of text as rows of modules (true = dark), no quiet zone.
func Encode(text string) ([][]bool, error) {
	ver, data, err := encodeData([]byte(text))
	if err != nil {
		return nil, err
	}
	words := interleave(ver, data)
	var best *grid
	bestPen := -1
	for mask := 0; mask < 8; mask++ {
		g := newGrid(ver)
		g.drawFunctionPatterns()
		g.drawCodewords(words)
		g.applyMask(mask)
		g.drawFormat(mask)
		if pen := g.penalty(); bestPen < 0 || pen < bestPen {
			best, bestPen = g, pen
		}
	}
	return best.m, nil
}

// SVG draws a code as a crisp black-on-white SVG, one unit per module, with the required
// quiet zone (same output as backup/qr.py's to_svg). Shown on the PC's host page.
func SVG(m [][]bool, quiet int) string {
	n := len(m) + 2*quiet
	var path strings.Builder
	for y, row := range m {
		for x, dark := range row {
			if dark {
				fmt.Fprintf(&path, "M%d,%dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" `+
		`shape-rendering="crispEdges"><rect width="%d" height="%d" fill="#fff"/>`+
		`<path d="%s" fill="#000"/></svg>`, n, n, n, n, path.String())
}

func rawModules(ver int) int {
	n := (16*ver+128)*ver + 64
	if ver >= 2 {
		na := ver/7 + 2
		n -= (25*na-10)*na - 55
		if ver >= 7 {
			n -= 36
		}
	}
	return n
}

func dataCodewords(ver int) int { return rawModules(ver)/8 - eccPerBlock[ver]*numBlocks[ver] }

func alignPositions(ver int) []int {
	if ver == 1 {
		return nil
	}
	na := ver/7 + 2
	step := (ver*8 + na*3 + 5) / (na*4 - 4) * 2
	pos := make([]int, na)
	pos[0] = 6
	for i := 0; i < na-1; i++ {
		pos[na-1-i] = ver*4 + 17 - 7 - i*step
	}
	return pos
}

func gfMul(x, y int) int {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= ((y >> i) & 1) * x
	}
	return z
}

func rsDivisor(degree int) []int {
	res := make([]int, degree)
	res[degree-1] = 1
	root := 1
	for k := 0; k < degree; k++ {
		for j := 0; j < degree; j++ {
			res[j] = gfMul(res[j], root)
			if j+1 < degree {
				res[j] ^= res[j+1]
			}
		}
		root = gfMul(root, 0x02)
	}
	return res
}

func rsRemainder(data, divisor []int) []int {
	res := make([]int, len(divisor))
	for _, b := range data {
		factor := b ^ res[0]
		copy(res, res[1:])
		res[len(res)-1] = 0
		for i, coef := range divisor {
			res[i] ^= gfMul(coef, factor)
		}
	}
	return res
}

func encodeData(data []byte) (int, []int, error) {
	ver, countBits := 0, 0
	for v := 1; v <= 10; v++ {
		cb := 8
		if v > 9 {
			cb = 16
		}
		if 4+cb+8*len(data) <= dataCodewords(v)*8 {
			ver, countBits = v, cb
			break
		}
	}
	if ver == 0 {
		return 0, nil, ErrTooLong
	}
	capBits := dataCodewords(ver) * 8
	var bits []int
	put := func(val, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, (val>>i)&1)
		}
	}
	put(0b0100, 4)
	put(len(data), countBits)
	for _, b := range data {
		put(int(b), 8)
	}
	put(0, min(4, capBits-len(bits)))
	put(0, (8-len(bits)%8)%8)
	for pad := 0xEC; len(bits) < capBits; pad ^= 0xEC ^ 0x11 {
		put(pad, 8)
	}
	words := make([]int, len(bits)/8)
	for i := range words {
		for _, b := range bits[i*8 : i*8+8] {
			words[i] = words[i]<<1 | b
		}
	}
	return ver, words, nil
}

func interleave(ver int, data []int) []int {
	nblocks, ecclen := numBlocks[ver], eccPerBlock[ver]
	raw := rawModules(ver) / 8
	nshort := nblocks - raw%nblocks
	shortlen := raw / nblocks
	div := rsDivisor(ecclen)
	blocks := make([][]int, 0, nblocks)
	k := 0
	for i := 0; i < nblocks; i++ {
		n := shortlen - ecclen
		if i >= nshort {
			n++
		}
		dat := append([]int(nil), data[k:k+n]...)
		k += n
		ecc := rsRemainder(dat, div)
		if i < nshort {
			dat = append(dat, 0)
		}
		blocks = append(blocks, append(dat, ecc...))
	}
	var out []int
	for i := range blocks[0] {
		for j, blk := range blocks {
			if i != shortlen-ecclen || j >= nshort {
				out = append(out, blk[i])
			}
		}
	}
	return out
}

type grid struct {
	ver, size int
	m, fn     [][]bool
}

func newGrid(ver int) *grid {
	s := ver*4 + 17
	g := &grid{ver: ver, size: s, m: make([][]bool, s), fn: make([][]bool, s)}
	for i := range g.m {
		g.m[i] = make([]bool, s)
		g.fn[i] = make([]bool, s)
	}
	return g
}

func (g *grid) setf(x, y int, dark bool) { g.m[y][x] = dark; g.fn[y][x] = true }

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (g *grid) drawFunctionPatterns() {
	s := g.size
	for i := 0; i < s; i++ {
		g.setf(6, i, i%2 == 0)
		g.setf(i, 6, i%2 == 0)
	}
	for _, c := range [][2]int{{3, 3}, {s - 4, 3}, {3, s - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := c[0]+dx, c[1]+dy
				if x >= 0 && x < s && y >= 0 && y < s {
					d := max(absInt(dx), absInt(dy))
					g.setf(x, y, d != 2 && d != 4)
				}
			}
		}
	}
	pos := alignPositions(g.ver)
	last := len(pos) - 1
	for i, px := range pos {
		for j, py := range pos {
			if (i == 0 && j == 0) || (i == 0 && j == last) || (i == last && j == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					g.setf(px+dx, py+dy, max(absInt(dx), absInt(dy)) != 1)
				}
			}
		}
	}
	g.drawFormat(0)
	if g.ver >= 7 {
		rem := g.ver
		for i := 0; i < 12; i++ {
			rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
		}
		bits := g.ver<<12 | rem
		for i := 0; i < 18; i++ {
			bit := (bits>>i)&1 == 1
			a, b := s-11+i%3, i/3
			g.setf(a, b, bit)
			g.setf(b, a, bit)
		}
	}
}

func (g *grid) drawFormat(mask int) {
	data := formatBitsM<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return (bits>>i)&1 == 1 }
	s := g.size
	for i := 0; i < 6; i++ {
		g.setf(8, i, bit(i))
	}
	g.setf(8, 7, bit(6))
	g.setf(8, 8, bit(7))
	g.setf(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		g.setf(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		g.setf(s-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		g.setf(8, s-15+i, bit(i))
	}
	g.setf(8, s-8, true) // the dark module
}

func (g *grid) drawCodewords(words []int) {
	s, i, total := g.size, 0, len(words)*8
	for right := s - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < s; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				upward := ((right + 1) & 2) == 0
				y := vert
				if upward {
					y = s - 1 - vert
				}
				if !g.fn[y][x] && i < total {
					g.m[y][x] = (words[i>>3]>>(7-(i&7)))&1 == 1
					i++
				}
			}
		}
	}
}

func (g *grid) applyMask(mask int) {
	f := [8]func(x, y int) bool{
		func(x, y int) bool { return (x+y)%2 == 0 },
		func(x, y int) bool { return y%2 == 0 },
		func(x, y int) bool { return x%3 == 0 },
		func(x, y int) bool { return (x+y)%3 == 0 },
		func(x, y int) bool { return (x/3+y/2)%2 == 0 },
		func(x, y int) bool { return x*y%2+x*y%3 == 0 },
		func(x, y int) bool { return (x*y%2+x*y%3)%2 == 0 },
		func(x, y int) bool { return ((x+y)%2+x*y%3)%2 == 0 },
	}[mask]
	for y := 0; y < g.size; y++ {
		for x := 0; x < g.size; x++ {
			if !g.fn[y][x] && f(x, y) {
				g.m[y][x] = !g.m[y][x]
			}
		}
	}
}

func (g *grid) penalty() int {
	s, m, p := g.size, g.m, 0
	lines := make([][]bool, 0, 2*s)
	lines = append(lines, m...)
	for x := 0; x < s; x++ {
		col := make([]bool, s)
		for y := 0; y < s; y++ {
			col[y] = m[y][x]
		}
		lines = append(lines, col)
	}
	finderA := []bool{true, false, true, true, true, false, true, false, false, false, false}
	finderB := make([]bool, len(finderA))
	for i := range finderA {
		finderB[i] = finderA[len(finderA)-1-i]
	}
	eq := func(a, b []bool) bool {
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	for _, line := range lines {
		run := 1
		for i := 1; i <= s; i++ {
			if i < s && line[i] == line[i-1] {
				run++
			} else {
				if run >= 5 {
					p += 3 + run - 5
				}
				run = 1
			}
		}
		for i := 0; i < s-10; i++ {
			if seg := line[i : i+11]; eq(seg, finderA) || eq(seg, finderB) {
				p += 40
			}
		}
	}
	for y := 0; y < s-1; y++ {
		for x := 0; x < s-1; x++ {
			c := m[y][x]
			if c == m[y][x+1] && c == m[y+1][x] && c == m[y+1][x+1] {
				p += 3
			}
		}
	}
	dark := 0
	for _, row := range m {
		for _, d := range row {
			if d {
				dark++
			}
		}
	}
	total := s * s
	k := (absInt(dark*20-total*10)+total-1)/total - 1
	return p + k*10
}
