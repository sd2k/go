// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wasm

import (
	"cmd/internal/obj"
	"slices"
	"sort"
)

// A function's code is a sequence of resume segments: the code between
// two resume points, which the entry br_table can jump to. A jump between
// basic blocks always lands at the start of its target's segment.
//
// Jumps to a later segment are plain branches out of the nested blocks
// whose ends start the segments. Jumps to the same or an earlier segment
// (back edges) need a wasm loop. A region is the function's top level, or
// a run of segments [first, last] compiled as one wasm loop, which back
// edges to first branch to. Regions nest, and a region's children are the
// segments and loops directly in it, each started by the end of a block,
// as at the top level.
//
// Resuming inside a loop has to enter it at its top, since wasm can't
// branch into a loop, and so does a jump into a loop other than to its
// first segment (the entry of a rotated loop, or an irreducible jump),
// which sets PC_B to its target and branches to the loop's top. Such a
// loop dispatches: if PC_B is 0 it starts its first segment, and
// otherwise it compares PC_B with the pcs of its targets, the children
// that are resumed or jumped into. A target that is a nested loop which
// dispatches takes PC_B on; otherwise the loop clears PC_B and branches
// to the child. It compares the first child's pcs first: a jump to a
// loop's first segment from outside the loop around it reaches it this
// way, as the entry of a rotated loop whose header is a loop does.
//
// So normal control flow reaches a loop with PC_B 0, or with the stale pc
// of a resume at the top level, which is outside every loop's targets:
// the loop clears it and starts its first segment. The top level doesn't
// clear PC_B, to keep its br_table, which has an entry for every pc and
// is the code the old dispatch had.
//
// Compares rather than a second br_table per loop keep the code small:
// a br_table has 4 bytes per pc, and loops have few targets.
type region struct {
	first, last int
	parent      *region
	// starts[i] is the first segment of the i-th child.
	starts []int
	// targets are the children, other than the first, that the loop's
	// dispatch goes to.
	targets []int

	// Set when the code is laid out.
	loop   *obj.Prog   // the Loop instruction
	blocks []*obj.Prog // blocks[i] is the Block whose end starts child i
}

type regions struct {
	top    *region
	loops  []*region // sorted by first
	inner  []*region // the innermost region of each segment
	loopAt []*region // the loop starting at each segment, if any
	endsAt []int     // the number of loops ending at each segment
}

// findRegions computes the loops of a function with nseg resume segments
// from its jumps, as (from, to) segment pairs.
func findRegions(nseg int, jumps [][2]int) *regions {
	rs := &regions{
		top:    &region{first: 0, last: nseg - 1},
		inner:  make([]*region, nseg),
		loopAt: make([]*region, nseg),
		endsAt: make([]int, nseg),
	}
	for _, j := range jumps {
		from, to := j[0], j[1]
		if to > from {
			continue
		}
		if l := rs.loopAt[to]; l != nil {
			l.last = max(l.last, from)
			continue
		}
		l := &region{first: to, last: from}
		rs.loopAt[to] = l
		rs.loops = append(rs.loops, l)
	}
	slices.SortFunc(rs.loops, func(a, b *region) int { return a.first - b.first })

	// Wasm loops must nest. A loop that overlaps a later one without
	// containing it is extended to contain it, which is always valid:
	// code in a loop's body doesn't have to loop. The stack holds the
	// loops containing the current one, outermost first.
	stack := []*region{rs.top}
	for _, l := range rs.loops {
		for stack[len(stack)-1].last < l.first {
			stack = stack[:len(stack)-1]
		}
		for _, o := range stack[1:] {
			o.last = max(o.last, l.last)
		}
		l.parent = stack[len(stack)-1]
		stack = append(stack, l)
	}

	for i := range rs.inner {
		rs.inner[i] = rs.top
	}
	for _, l := range rs.loops { // parents before children
		for i := l.first; i <= l.last; i++ {
			rs.inner[i] = l
		}
		rs.endsAt[l.last]++
	}
	for i, r := range rs.inner {
		if r.first == i && r != rs.top {
			// The first segment of a loop is its first child, and
			// the loop is a child of its parent.
			r.parent.starts = append(r.parent.starts, i)
		}
		r.starts = append(r.starts, i)
	}
	return rs
}

// child returns the index of r's child that contains segment seg.
func (r *region) child(seg int) int {
	return sort.SearchInts(r.starts, seg+1) - 1
}

// target returns the region and the child in it that a jump from segment
// from to the later segment to branches to the start of.
func (rs *regions) target(from, to int) (*region, int) {
	r := rs.inner[from]
	for to > r.last {
		r = r.parent
	}
	return r, r.child(to)
}

// jumpInto records a jump from segment from to the later segment to: the
// loops it enters other than at their first segment dispatch to it.
func (rs *regions) jumpInto(from, to int) {
	r, _ := rs.target(from, to)
	for l := rs.inner[to]; l != r; l = l.parent {
		if l.first != to {
			l.mark(to)
		}
	}
}

// resumeAt records a resume point in segment seg.
func (rs *regions) resumeAt(seg int) {
	for l := rs.inner[seg]; l != rs.top; l = l.parent {
		if l.first != seg {
			l.mark(seg)
		}
	}
}

// mark makes the child of loop r that contains segment seg a target.
func (r *region) mark(seg int) {
	if c := r.child(seg); !slices.Contains(r.targets, c) {
		r.targets = append(r.targets, c)
	}
}
