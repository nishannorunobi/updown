#!/usr/bin/env python3
"""qr.py — tiny offline QR code generator (standard library only).

Used by backup/run_qr.sh (shown by share_server.py) to turn the download link into a QR code, so the
phone just scans instead of typing a URL. No package to install, no internet.

    python3 backup/qr.py "http://172.20.10.6:8000/x.zip" > qr.svg
    python3 backup/qr.py --terminal "http://172.20.10.6:8000/x.zip"   # draw it in the console

Byte mode, error correction level M (15 % — survives a glare-y laptop screen),
versions 1–10 (up to 213 bytes, far more than a URL needs), best of the 8 masks by
the standard penalty rules. Follows ISO/IEC 18004; structure after Project Nayuki's
reference implementation.
"""
import sys

# Level M, versions 1..10 (index 0 unused).
ECC_PER_BLOCK = [None, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26]
NUM_BLOCKS    = [None,  1,  1,  1,  2,  2,  4,  4,  4,  5,  5]
FORMAT_BITS_M = 0  # L=1, M=0, Q=3, H=2


def _raw_modules(ver):
    n = (16 * ver + 128) * ver + 64
    if ver >= 2:
        na = ver // 7 + 2
        n -= (25 * na - 10) * na - 55
        if ver >= 7:
            n -= 36
    return n


def _data_codewords(ver):
    return _raw_modules(ver) // 8 - ECC_PER_BLOCK[ver] * NUM_BLOCKS[ver]


def _align_positions(ver):
    if ver == 1:
        return []
    na = ver // 7 + 2
    step = (ver * 8 + na * 3 + 5) // (na * 4 - 4) * 2
    return [6] + [ver * 4 + 17 - 7 - i * step for i in range(na - 1)][::-1]


def _gf_mul(x, y):
    z = 0
    for i in range(7, -1, -1):
        z = (z << 1) ^ ((z >> 7) * 0x11D)
        z ^= ((y >> i) & 1) * x
    return z


def _rs_divisor(degree):
    res = [0] * (degree - 1) + [1]
    root = 1
    for _ in range(degree):
        for j in range(degree):
            res[j] = _gf_mul(res[j], root)
            if j + 1 < degree:
                res[j] ^= res[j + 1]
        root = _gf_mul(root, 0x02)
    return res


def _rs_remainder(data, divisor):
    res = [0] * len(divisor)
    for b in data:
        factor = b ^ res.pop(0)
        res.append(0)
        for i, coef in enumerate(divisor):
            res[i] ^= _gf_mul(coef, factor)
    return res


def _encode_data(text):
    data = text.encode("utf-8")
    for ver in range(1, 11):
        count_bits = 8 if ver <= 9 else 16
        if 4 + count_bits + 8 * len(data) <= _data_codewords(ver) * 8:
            break
    else:
        raise ValueError("text too long for QR versions 1-10")
    cap = _data_codewords(ver) * 8
    bits = []

    def put(val, n):
        bits.extend((val >> i) & 1 for i in range(n - 1, -1, -1))

    put(0b0100, 4)
    put(len(data), count_bits)
    for b in data:
        put(b, 8)
    put(0, min(4, cap - len(bits)))
    put(0, (-len(bits)) % 8)
    pad = 0xEC
    while len(bits) < cap:
        put(pad, 8)
        pad ^= 0xEC ^ 0x11
    words = [int("".join(map(str, bits[i:i + 8])), 2) for i in range(0, len(bits), 8)]
    return ver, words


def _interleave(ver, data):
    nblocks, ecclen = NUM_BLOCKS[ver], ECC_PER_BLOCK[ver]
    raw = _raw_modules(ver) // 8
    nshort = nblocks - raw % nblocks
    shortlen = raw // nblocks
    div = _rs_divisor(ecclen)
    blocks, k = [], 0
    for i in range(nblocks):
        dat = data[k:k + shortlen - ecclen + (0 if i < nshort else 1)]
        k += len(dat)
        ecc = _rs_remainder(dat, div)
        if i < nshort:
            dat = dat + [0]
        blocks.append(dat + ecc)
    out = []
    for i in range(len(blocks[0])):
        for j, blk in enumerate(blocks):
            if i != shortlen - ecclen or j >= nshort:
                out.append(blk[i])
    return out


class _Grid:
    def __init__(self, ver):
        self.ver = ver
        self.size = ver * 4 + 17
        self.m = [[False] * self.size for _ in range(self.size)]
        self.fn = [[False] * self.size for _ in range(self.size)]

    def setf(self, x, y, dark):
        self.m[y][x] = dark
        self.fn[y][x] = True

    def draw_function_patterns(self):
        s = self.size
        for i in range(s):
            self.setf(6, i, i % 2 == 0)
            self.setf(i, 6, i % 2 == 0)
        for cx, cy in ((3, 3), (s - 4, 3), (3, s - 4)):
            for dy in range(-4, 5):
                for dx in range(-4, 5):
                    x, y = cx + dx, cy + dy
                    if 0 <= x < s and 0 <= y < s:
                        self.setf(x, y, max(abs(dx), abs(dy)) not in (2, 4))
        pos = _align_positions(self.ver)
        last = len(pos) - 1
        for i, px in enumerate(pos):
            for j, py in enumerate(pos):
                if (i, j) in ((0, 0), (0, last), (last, 0)):
                    continue
                for dy in range(-2, 3):
                    for dx in range(-2, 3):
                        self.setf(px + dx, py + dy, max(abs(dx), abs(dy)) != 1)
        self.draw_format(0)
        if self.ver >= 7:
            rem = self.ver
            for _ in range(12):
                rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
            bits = self.ver << 12 | rem
            for i in range(18):
                bit = (bits >> i) & 1 == 1
                a, b = s - 11 + i % 3, i // 3
                self.setf(a, b, bit)
                self.setf(b, a, bit)

    def draw_format(self, mask):
        data = FORMAT_BITS_M << 3 | mask
        rem = data
        for _ in range(10):
            rem = (rem << 1) ^ ((rem >> 9) * 0x537)
        bits = (data << 10 | rem) ^ 0x5412
        bit = lambda i: (bits >> i) & 1 == 1
        s = self.size
        for i in range(6):
            self.setf(8, i, bit(i))
        self.setf(8, 7, bit(6))
        self.setf(8, 8, bit(7))
        self.setf(7, 8, bit(8))
        for i in range(9, 15):
            self.setf(14 - i, 8, bit(i))
        for i in range(8):
            self.setf(s - 1 - i, 8, bit(i))
        for i in range(8, 15):
            self.setf(8, s - 15 + i, bit(i))
        self.setf(8, s - 8, True)  # the dark module

    def draw_codewords(self, words):
        s, i, total = self.size, 0, len(words) * 8
        right = s - 1
        while right >= 1:
            if right == 6:
                right = 5
            for vert in range(s):
                for j in range(2):
                    x = right - j
                    upward = ((right + 1) & 2) == 0
                    y = s - 1 - vert if upward else vert
                    if not self.fn[y][x] and i < total:
                        self.m[y][x] = (words[i >> 3] >> (7 - (i & 7))) & 1 == 1
                        i += 1
            right -= 2

    def apply_mask(self, mask):
        f = [
            lambda x, y: (x + y) % 2 == 0,
            lambda x, y: y % 2 == 0,
            lambda x, y: x % 3 == 0,
            lambda x, y: (x + y) % 3 == 0,
            lambda x, y: (x // 3 + y // 2) % 2 == 0,
            lambda x, y: x * y % 2 + x * y % 3 == 0,
            lambda x, y: (x * y % 2 + x * y % 3) % 2 == 0,
            lambda x, y: ((x + y) % 2 + x * y % 3) % 2 == 0,
        ][mask]
        for y in range(self.size):
            for x in range(self.size):
                if not self.fn[y][x] and f(x, y):
                    self.m[y][x] = not self.m[y][x]

    def penalty(self):
        s, m, p = self.size, self.m, 0
        lines = [row for row in m] + [[m[y][x] for y in range(s)] for x in range(s)]
        finder_a = [True, False, True, True, True, False, True, False, False, False, False]
        finder_b = finder_a[::-1]
        for line in lines:
            run = 1
            for i in range(1, s + 1):
                if i < s and line[i] == line[i - 1]:
                    run += 1
                else:
                    if run >= 5:
                        p += 3 + run - 5
                    run = 1
            for i in range(s - 10):
                seg = line[i:i + 11]
                if seg == finder_a or seg == finder_b:
                    p += 40
        for y in range(s - 1):
            for x in range(s - 1):
                c = m[y][x]
                if c == m[y][x + 1] == m[y + 1][x] == m[y + 1][x + 1]:
                    p += 3
        dark = sum(sum(r) for r in m)
        total = s * s
        k = (abs(dark * 20 - total * 10) + total - 1) // total - 1
        return p + k * 10


def make_qr(text):
    """Return the QR code for text as a list of rows of booleans (True = dark)."""
    ver, data = _encode_data(text)
    words = _interleave(ver, data)
    best, best_pen = None, None
    for mask in range(8):
        g = _Grid(ver)
        g.draw_function_patterns()
        g.draw_codewords(words)
        g.apply_mask(mask)
        g.draw_format(mask)
        pen = g.penalty()
        if best_pen is None or pen < best_pen:
            best, best_pen = g, pen
    return best.m


def to_svg(matrix, quiet=4):
    """Crisp black-on-white SVG, one unit per module, with the required quiet zone."""
    n = len(matrix) + 2 * quiet
    path = "".join(f"M{x + quiet},{y + quiet}h1v1h-1z"
                   for y, row in enumerate(matrix) for x, dark in enumerate(row) if dark)
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {n} {n}" '
            f'shape-rendering="crispEdges"><rect width="{n}" height="{n}" fill="#fff"/>'
            f'<path d="{path}" fill="#000"/></svg>')


def to_terminal(matrix, quiet=4):
    """Draw the code in a terminal: each text line holds two module rows ("▀" with
    the top module as foreground, the bottom one as background), so modules come out
    square. Colours are explicit black on white — phone cameras read dark-on-light,
    and a dark terminal theme would otherwise invert the code."""
    n = len(matrix)
    size = n + 2 * quiet

    def dark(x, y):
        x, y = x - quiet, y - quiet
        return 0 <= x < n and 0 <= y < n and matrix[y][x]

    lines = []
    for y in range(0, size, 2):
        row = []
        for x in range(size):
            top = dark(x, y)
            bottom = dark(x, y + 1) if y + 1 < size else False
            fg = "30" if top else "97"          # black / bright white
            bg = "40" if bottom else "107"
            row.append(f"\033[{fg};{bg}m▀")
        lines.append("".join(row) + "\033[0m")
    return "\n".join(lines)


if __name__ == "__main__":
    args = sys.argv[1:]
    terminal = "--terminal" in args
    args = [a for a in args if a != "--terminal"]
    if len(args) != 1:
        sys.exit("usage: qr.py [--terminal] TEXT   (SVG on stdout, or --terminal to draw it)")
    m = make_qr(args[0])
    print(to_terminal(m) if terminal else to_svg(m))
