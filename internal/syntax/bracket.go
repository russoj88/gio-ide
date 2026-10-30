package syntax

import sitter "github.com/tree-sitter/go-tree-sitter"

func matchBracket(root *sitter.Node, text []byte, at int) (open, close int, ok bool) {
	if root == nil || len(text) == 0 {
		return 0, 0, false
	}
	var cands []int
	if at >= 0 && at < len(text) && isBracket(text[at]) {
		cands = append(cands, at)
	}
	if at > 0 && at-1 < len(text) && isBracket(text[at-1]) {
		cands = append(cands, at-1)
	}
	for _, pos := range cands {
		n := root.DescendantForByteRange(uint(pos), uint(pos+1))
		for n != nil {
			s := int(n.StartByte())
			e := int(n.EndByte())
			if e > s && e <= len(text) && isOpen(text[s]) && isClose(text[e-1]) && pairs(text[s], text[e-1]) {
				if s == pos || e-1 == pos {
					return s, e - 1, true
				}
			}
			n = n.Parent()
		}
	}
	return 0, 0, false
}

func isBracket(b byte) bool { return isOpen(b) || isClose(b) }

func isOpen(b byte) bool { return b == '(' || b == '{' || b == '[' }

func isClose(b byte) bool { return b == ')' || b == '}' || b == ']' }

func pairs(a, b byte) bool {
	return (a == '(' && b == ')') || (a == '{' && b == '}') || (a == '[' && b == ']')
}
