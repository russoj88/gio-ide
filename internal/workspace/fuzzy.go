package workspace

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Match is one fuzzy hit. Score is higher for a better match.
type Match struct {
	Path  string
	Score int
}

const fuzzyMiss = -1 << 30

// FuzzyScore scores query as a case-insensitive subsequence of path.
// It returns false when query is not a subsequence.
// Bonuses: filename, path separators, word starts, and consecutive runs.
func FuzzyScore(query, filePath string) (int, bool) {
	if query == "" {
		return 0, true
	}
	q := strings.ToLower(query)
	p := filePath
	pl := strings.ToLower(p)
	m, n := len(q), len(pl)
	if m > n {
		return 0, false
	}
	// Bonuses use the lowered string. ToLower can be longer than p,
	// and an index into p at that offset panics.
	base := path.Base(pl)
	baseAt := len(pl) - len(base)

	best := make([]int, n+1)
	end := make([]int, n+1)
	nextBest := make([]int, n+1)
	nextEnd := make([]int, n+1)
	for j := range best {
		best[j] = 0
		end[j] = fuzzyMiss
	}
	for i := 1; i <= m; i++ {
		for j := range nextBest {
			nextBest[j] = fuzzyMiss
			nextEnd[j] = fuzzyMiss
		}
		for j := 1; j <= n; j++ {
			skip := nextBest[j-1]
			got := fuzzyMiss
			if pl[j-1] == q[i-1] && best[j-1] != fuzzyMiss {
				got = best[j-1] + 1
				if j-1 >= baseAt {
					got += 16
				}
				if j == 1 || isSep(pl[j-2]) {
					got += 10
				} else if wordStart(p, j-1) {
					got += 8
				}
				if end[j-1] != fuzzyMiss && end[j-1] == best[j-1] {
					got += 7
				}
			}
			nextBest[j] = max(got, skip)
			nextEnd[j] = got
		}
		best, nextBest = nextBest, best
		end, nextEnd = nextEnd, end
	}
	if best[n] == fuzzyMiss {
		return 0, false
	}
	return best[n], true
}

func isSep(b byte) bool { return b == '/' || b == '\\' }

func wordStart(p string, i int) bool {
	if i <= 0 || i >= len(p) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(p[i:])
	prev, _ := utf8.DecodeLastRuneInString(p[:i])
	if prev == '_' || prev == '-' || prev == '.' {
		return true
	}
	return unicode.IsUpper(r) && unicode.IsLower(prev)
}
