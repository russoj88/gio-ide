package buffer

import "sync/atomic"

// A node is one piece in a persistent treap. Subtree byte length and newline
// counts make line lookup logarithmic. Nodes are never mutated in place.
type node struct {
	left, right *node
	prio        uint32
	src         int // 0 = original buffer, 1 = add buffer
	off         int
	ln          int
	nl          int
	size        int // subtree bytes
	newlines    int // subtree newlines
}

var prioN atomic.Uint32

func init() { prioN.Store(0x12345678) }

func nextPrio() uint32 {
	x := prioN.Add(0x9e3779b9)
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	if x == 0 {
		x = 1
	}
	return x
}

func leaf(src, off, ln, nl int) *node {
	n := &node{prio: nextPrio(), src: src, off: off, ln: ln, nl: nl}
	fix(n)
	return n
}

func fix(n *node) {
	n.size = n.ln
	n.newlines = n.nl
	if n.left != nil {
		n.size += n.left.size
		n.newlines += n.left.newlines
	}
	if n.right != nil {
		n.size += n.right.size
		n.newlines += n.right.newlines
	}
}

func sizeOf(n *node) int {
	if n == nil {
		return 0
	}
	return n.size
}

func nlOf(n *node) int {
	if n == nil {
		return 0
	}
	return n.newlines
}

func (d *Doc) piece(n *node) []byte {
	if n == nil || n.ln == 0 {
		return nil
	}
	if n.src == 0 {
		return d.orig[n.off : n.off+n.ln]
	}
	return d.add[n.off : n.off+n.ln]
}

func (d *Doc) insert(root *node, at int, text string) *node {
	if text == "" {
		return root
	}
	start := len(d.add)
	d.add = append(d.add, text...)
	mid := leaf(1, start, len(text), countNLString(text))
	l, r := d.split(root, at)
	return merge(merge(l, mid), r)
}

func (d *Doc) remove(root *node, start, end int) *node {
	if start >= end {
		return root
	}
	l, rest := d.split(root, start)
	_, r := d.split(rest, end-start)
	return merge(l, r)
}

func (d *Doc) split(n *node, off int) (*node, *node) {
	if n == nil {
		return nil, nil
	}
	ls := sizeOf(n.left)
	if off < ls {
		l, r := d.split(n.left, off)
		cp := *n
		cp.left = r
		fix(&cp)
		return l, &cp
	}
	if off == ls {
		cp := *n
		cp.left = nil
		fix(&cp)
		return n.left, &cp
	}
	if off >= ls+n.ln {
		if off == ls+n.ln {
			cp := *n
			cp.right = nil
			fix(&cp)
			return &cp, n.right
		}
		l, r := d.split(n.right, off-ls-n.ln)
		cp := *n
		cp.right = l
		fix(&cp)
		return &cp, r
	}
	at := off - ls
	b := d.piece(n)
	leftNL := countNL(b[:at])
	a := leaf(n.src, n.off, at, leftNL)
	a.prio = n.prio
	a.left = n.left
	fix(a)
	right := leaf(n.src, n.off+at, n.ln-at, n.nl-leftNL)
	right.right = n.right
	fix(right)
	return a, right
}

func merge(a, b *node) *node {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.prio >= b.prio {
		cp := *a
		cp.right = merge(a.right, b)
		fix(&cp)
		return &cp
	}
	cp := *b
	cp.left = merge(a, b.left)
	fix(&cp)
	return &cp
}

type snap struct {
	version int32
	root    *node
	orig    []byte
	add     []byte
}

func (s *snap) Version() int32 { return s.version }
func (s *snap) Len() int       { return sizeOf(s.root) }

func (s *snap) LineCount() int {
	n := nlOf(s.root) + 1
	if n < 1 {
		return 1
	}
	return n
}

func (s *snap) LineStart(n int) Offset {
	if n <= 0 {
		return 0
	}
	if n >= s.LineCount() {
		return Offset(s.Len())
	}
	return Offset(s.afterNewlines(s.root, n))
}

func (s *snap) Line(n int) []byte {
	if n < 0 || n >= s.LineCount() {
		return nil
	}
	start := int(s.LineStart(n))
	end := s.Len()
	if n+1 < s.LineCount() {
		end = int(s.LineStart(n + 1))
	}
	if end > start && end <= s.Len() {
		b := s.slice(start, end)
		if len(b) > 0 && b[len(b)-1] == '\n' {
			b = b[:len(b)-1]
		}
		return b
	}
	return s.slice(start, end)
}

func (s *snap) OffsetToLineCol(o Offset) (int, int) {
	if o < 0 {
		return 0, 0
	}
	if int(o) > s.Len() {
		o = Offset(s.Len())
	}
	line := s.newlinesBefore(s.root, int(o))
	start := s.LineStart(line)
	return line, int(o - start)
}

func (s *snap) newlinesBefore(n *node, off int) int {
	if n == nil || off <= 0 {
		return 0
	}
	ls := sizeOf(n.left)
	if off <= ls {
		return s.newlinesBefore(n.left, off)
	}
	inPiece := min(off-ls, n.ln)
	nls := 0
	if inPiece > 0 {
		nls = countNL(s.piece(n)[:inPiece])
	}
	if off <= ls+n.ln {
		return nlOf(n.left) + nls
	}
	return nlOf(n.left) + n.nl + s.newlinesBefore(n.right, off-ls-n.ln)
}

func (s *snap) Slice(start, end Offset) []byte {
	if start < 0 {
		start = 0
	}
	if end > Offset(s.Len()) {
		end = Offset(s.Len())
	}
	if end < start {
		return nil
	}
	return s.slice(int(start), int(end))
}

func (s *snap) Bytes() []byte {
	return s.slice(0, s.Len())
}

func (s *snap) piece(n *node) []byte {
	if n == nil || n.ln == 0 {
		return nil
	}
	if n.src == 0 {
		return s.orig[n.off : n.off+n.ln]
	}
	return s.add[n.off : n.off+n.ln]
}

func (s *snap) afterNewlines(n *node, need int) int {
	if n == nil || need <= 0 {
		return 0
	}
	leftNL := nlOf(n.left)
	if need <= leftNL {
		return s.afterNewlines(n.left, need)
	}
	need -= leftNL
	if need <= n.nl {
		b := s.piece(n)
		seen := 0
		for i := range b {
			if b[i] == '\n' {
				seen++
				if seen == need {
					return sizeOf(n.left) + i + 1
				}
			}
		}
		return sizeOf(n.left) + n.ln
	}
	need -= n.nl
	return sizeOf(n.left) + n.ln + s.afterNewlines(n.right, need)
}

func (s *snap) slice(start, end int) []byte {
	if start < 0 {
		start = 0
	}
	if end > s.Len() {
		end = s.Len()
	}
	if start >= end {
		return []byte{}
	}
	out := make([]byte, 0, end-start)
	s.walk(s.root, 0, start, end, &out)
	return out
}

func (s *snap) walk(n *node, base, start, end int, out *[]byte) {
	if n == nil || base >= end {
		return
	}
	ls := sizeOf(n.left)
	if start < base+ls {
		s.walk(n.left, base, start, end, out)
	}
	pb := base + ls
	pe := pb + n.ln
	if pe > start && pb < end {
		b := s.piece(n)
		from := 0
		to := len(b)
		if start > pb {
			from = start - pb
		}
		if end < pe {
			to = end - pb
		}
		if from < to {
			*out = append(*out, b[from:to]...)
		}
	}
	if end > pe {
		s.walk(n.right, pe, start, end, out)
	}
}
