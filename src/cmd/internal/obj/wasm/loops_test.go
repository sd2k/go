// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wasm

import (
	"fmt"
	"slices"
	"testing"
)

// layout describes regions as a string: a loop is "(" its children ")",
// a segment is its number, and a target of its loop's dispatch is marked
// with "*".
func layout(rs *regions, r *region) string {
	s := ""
	for i, seg := range r.starts {
		if i > 0 {
			s += " "
		}
		if l := rs.loopAt[seg]; l != nil && l != r {
			s += "(" + layout(rs, l) + ")"
		} else {
			s += fmt.Sprint(seg)
		}
		if slices.Contains(r.targets, i) {
			s += "*"
		}
	}
	return s
}

func TestFindRegions(t *testing.T) {
	tests := []struct {
		name   string
		nseg   int
		jumps  [][2]int
		resume []int // segments with resume points
		want   string
	}{
		{"none", 4, [][2]int{{0, 2}, {1, 3}}, nil, "0 1 2 3"},
		{"self", 3, [][2]int{{1, 1}}, nil, "0 (1) 2"},
		{"loop", 5, [][2]int{{3, 1}, {2, 1}}, nil, "0 (1 2 3) 4"},
		{"nested", 6, [][2]int{{4, 1}, {3, 2}}, nil, "0 (1 (2 3) 4) 5"},
		// Loops that overlap without nesting: the first is extended.
		{"crossing", 7, [][2]int{{3, 1}, {5, 2}}, nil, "0 (1 (2 3 4 5)) 6"},
		{"crossing3", 8, [][2]int{{3, 1}, {5, 2}, {6, 4}}, nil, "0 (1 (2 3 (4 5 6))) 7"},
		// A rotated loop: its entry jumps to its header, segment 2.
		{"rotated", 5, [][2]int{{0, 2}, {2, 1}}, nil, "0 (1 2*) 3 4"},
		// A jump from outside into a nested loop: both loops dispatch.
		{"deep entry", 7, [][2]int{{4, 1}, {3, 2}, {0, 3}}, nil, "0 (1 (2 3*)* 4) 5 6"},
		// A jump to a nested loop's first segment enters it at its top.
		{"inner first", 7, [][2]int{{4, 1}, {3, 2}, {0, 2}}, nil, "0 (1 (2 3)* 4) 5 6"},
		// A jump out of a loop needs no dispatch.
		{"exit", 6, [][2]int{{3, 1}, {2, 5}}, nil, "0 (1 2 3) 4 5"},
		// Resume points inside loops make every loop around them
		// dispatch to them, except at a loop's first segment.
		{"resume", 7, [][2]int{{4, 1}, {3, 2}}, []int{3, 2, 0}, "0 (1 (2 3*)* 4) 5 6"},
		{"resume outer", 7, [][2]int{{4, 1}, {3, 2}}, []int{4, 5}, "0 (1 (2 3) 4*) 5 6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := findRegions(tt.nseg, tt.jumps)
			for _, j := range tt.jumps {
				if j[1] > j[0] {
					rs.jumpInto(j[0], j[1])
				}
			}
			for _, seg := range tt.resume {
				rs.resumeAt(seg)
			}
			if got := layout(rs, rs.top); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			// Every back edge is in its loop, and the loops
			// ending at each segment are counted.
			ends := make([]int, tt.nseg)
			for _, l := range rs.loops {
				ends[l.last]++
			}
			if !slices.Equal(ends, rs.endsAt) {
				t.Errorf("endsAt = %v, want %v", rs.endsAt, ends)
			}
			for _, j := range tt.jumps {
				if l := rs.loopAt[j[1]]; j[1] <= j[0] && (l == nil || j[0] > l.last) {
					t.Errorf("back edge %v is not in a loop", j)
				}
			}
		})
	}
}

func TestRegionTarget(t *testing.T) {
	// 0 (1 (2 3) 4) 5
	rs := findRegions(6, [][2]int{{4, 1}, {3, 2}})
	for _, tt := range []struct {
		from, to int
		first    int // the region's first segment
		child    int
	}{
		{0, 5, 0, 2}, // over the loop
		{0, 1, 0, 1}, // to the loop's top
		{0, 3, 0, 1}, // into it, through its top
		{3, 5, 0, 2}, // out of both loops
		{3, 4, 1, 2}, // out of the inner loop
		{1, 3, 1, 1}, // into the inner loop, through its top
	} {
		r, c := rs.target(tt.from, tt.to)
		if r.first != tt.first || c != tt.child {
			t.Errorf("target(%d, %d) = region at %d, child %d; want region at %d, child %d", tt.from, tt.to, r.first, c, tt.first, tt.child)
		}
	}
}
