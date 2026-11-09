// Package logic finds a minimal sum of products for a truth table (Quine-McCluskey + exact cover).
package logic

import (
	"math/bits"
	"sort"
	"strings"
)

// Term is a product term: bits set in Mask are eliminated variables; Val holds the
// values of the remaining variables (masked bits are zero).
type Term struct{ Val, Mask uint32 }

func (t Term) covers(m uint32) bool { return m&^t.Mask == t.Val }

func (t Term) literals(n int) int { return n - bits.OnesCount32(t.Mask) }

// Minimize returns a minimal SOP for a table of 2^n rows with '0', '1' or 'x' (don't care).
// Row index bit n-1 is variable A.
func Minimize(n int, table string) []Term {
	var ones, all []uint32
	for i := 0; i < len(table) && i < 1<<n; i++ {
		switch table[i] {
		case '1':
			ones = append(ones, uint32(i))
			all = append(all, uint32(i))
		case 'x', 'X':
			all = append(all, uint32(i))
		}
	}
	if len(ones) == 0 {
		return nil
	}
	primes := primeImplicants(all)
	useful := primes[:0]
	for _, p := range primes {
		for _, m := range ones {
			if p.covers(m) {
				useful = append(useful, p)
				break
			}
		}
	}
	return cover(n, useful, ones)
}

func primeImplicants(ms []uint32) []Term {
	cur := map[Term]bool{}
	for _, m := range ms {
		cur[Term{Val: m}] = true
	}
	var primes []Term
	for len(cur) > 0 {
		list := make([]Term, 0, len(cur))
		for t := range cur {
			list = append(list, t)
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Mask != list[j].Mask {
				return list[i].Mask < list[j].Mask
			}
			return list[i].Val < list[j].Val
		})
		next := map[Term]bool{}
		used := make([]bool, len(list))
		for i := range list {
			for j := i + 1; j < len(list); j++ {
				a, b := list[i], list[j]
				if a.Mask != b.Mask {
					continue
				}
				d := a.Val ^ b.Val
				if bits.OnesCount32(d) == 1 {
					next[Term{Val: a.Val &^ d, Mask: a.Mask | d}] = true
					used[i], used[j] = true, true
				}
			}
		}
		for i, t := range list {
			if !used[i] {
				primes = append(primes, t)
			}
		}
		cur = next
	}
	return primes
}

// cover picks the fewest terms (then fewest literals) covering all minterms.
func cover(n int, primes []Term, ones []uint32) []Term {
	var best []Term
	bestCost := [2]int{1 << 30, 1 << 30}
	cost := func(ts []Term) [2]int {
		l := 0
		for _, t := range ts {
			l += t.literals(n)
		}
		return [2]int{len(ts), l}
	}
	less := func(a, b [2]int) bool { return a[0] < b[0] || (a[0] == b[0] && a[1] < b[1]) }
	budget := 200000
	var rec func(chosen []Term, left []uint32)
	rec = func(chosen []Term, left []uint32) {
		if budget--; budget < 0 {
			return
		}
		if len(left) == 0 {
			if c := cost(chosen); less(c, bestCost) {
				bestCost = c
				best = append([]Term(nil), chosen...)
			}
			return
		}
		if len(chosen)+1 > bestCost[0] {
			return
		}
		// Branch on the minterm with the fewest covering primes.
		var cands []Term
		for _, m := range left {
			var c []Term
			for _, p := range primes {
				if p.covers(m) {
					c = append(c, p)
				}
			}
			if cands == nil || len(c) < len(cands) {
				cands = c
			}
		}
		sort.Slice(cands, func(i, j int) bool { return cands[i].literals(n) < cands[j].literals(n) })
		for _, p := range cands {
			rest := make([]uint32, 0, len(left))
			for _, m := range left {
				if !p.covers(m) {
					rest = append(rest, m)
				}
			}
			rec(append(chosen, p), rest)
		}
	}
	rec(nil, ones)
	sort.Slice(best, func(i, j int) bool {
		if li, lj := best[i].literals(n), best[j].literals(n); li != lj {
			return li < lj
		}
		return best[i].Val > best[j].Val
	})
	return best
}

// Format prints terms the way Multisim does: AB'+C. A is the most significant variable.
func Format(n int, terms []Term) string {
	if len(terms) == 0 {
		return "0"
	}
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		var b strings.Builder
		for i := 0; i < n; i++ {
			bit := uint32(1) << (n - 1 - i)
			if t.Mask&bit != 0 {
				continue
			}
			b.WriteByte(byte('A' + i))
			if t.Val&bit == 0 {
				b.WriteByte('\'')
			}
		}
		if b.Len() == 0 {
			return "1"
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "+")
}
