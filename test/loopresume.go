// run

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test loops that resume: goroutine switches, stack growth and panics
// inside loops, rotated and irreducible loops, and indirect calls in
// loops. On wasm, these loops are wasm loops that resume points have to
// dispatch into.

package main

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
)

var sink int

// pingpong switches goroutines inside nested loops.
func pingpong(n int) int {
	ping, pong := make(chan int), make(chan int)
	go func() {
		for v := range ping {
			pong <- v * 3
		}
		close(pong)
	}()
	t := 0
	for i := 0; i < n; i++ {
		for j := 0; j < 3; j++ {
			ping <- i + j
			t += <-pong
			if j == 1 {
				continue
			}
			t ^= i
		}
	}
	close(ping)
	return t
}

// grow forces stack growth (morestack, copystack) from inside a loop in
// each frame.
//
//go:noinline
func grow(depth int) int {
	var buf [64]int
	t := 0
	for i := range buf {
		buf[i] = i * depth
		if depth > 0 && i%32 == 0 {
			t += grow(depth - 1)
		}
		t += buf[i]
	}
	return t
}

// recovering panics and recovers inside a loop, through deferreturn.
func recovering(n int) (t int) {
	for i := 0; i < n; i++ {
		t += func() (r int) {
			defer func() {
				if e := recover(); e != nil {
					r = len(fmt.Sprint(e))
				}
			}()
			if i%3 == 0 {
				panic(fmt.Sprintf("boom %d", i))
			}
			var m map[int]int
			if i%3 == 1 {
				m[i] = i // nil map write
			}
			return i
		}()
	}
	return t
}

// deferInLoop has a defer in a function with a loop that switches.
func deferInLoop(n int) (t int) {
	defer func() { t *= 2 }()
	for i := 0; i < n; i++ {
		runtime.Gosched()
		t += i
	}
	return t
}

type shape interface{ area() int }
type sq int
type rect struct{ w, h int }

func (s sq) area() int   { return int(s) * int(s) }
func (r rect) area() int { return r.w * r.h }

// indirect makes interface and closure calls in a loop that switches.
func indirect(n int) int {
	shapes := []shape{sq(2), rect{3, 4}, sq(5)}
	fs := []func(int) int{func(x int) int { return x + 1 }, func(x int) int { return x * 2 }}
	t := 0
	for i := 0; i < n; i++ {
		t += shapes[i%3].area()
		t = fs[i%2](t) % 1000003
		if i%50 == 0 {
			runtime.Gosched()
		}
	}
	return t
}

// irreducible jumps into the middle of a loop.
func irreducible(n int) int {
	t, i := 0, 0
	if n%2 == 0 {
		goto mid
	}
top:
	t += i
	runtime.Gosched()
mid:
	t ^= i * 7
	i++
	if i < n {
		goto top
	}
	return t
}

// labeled uses labeled break and continue across nested loops with calls.
func labeled(n int) int {
	t := 0
outer:
	for i := 0; i < n; i++ {
		for j := 0; ; j++ {
			switch {
			case j > i:
				continue outer
			case i*j > 50:
				break outer
			case (i+j)%4 == 0:
				runtime.Gosched()
			}
			t += i * j
		}
	}
	return t
}

// workers runs goroutines that loop over a shared counter.
func workers(n int) int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	total := 0
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				mu.Lock()
				total += w*i + 1
				mu.Unlock()
				if i%7 == 0 {
					runtime.Gosched()
				}
			}
		}()
	}
	wg.Wait()
	return total
}

// selectLoop selects in a loop over several channels.
func selectLoop(n int) int {
	a, b, done := make(chan int), make(chan int), make(chan bool)
	go func() {
		for i := 0; i < n; i++ {
			if i%2 == 0 {
				a <- i
			} else {
				b <- i
			}
		}
		done <- true
	}()
	t := 0
	for {
		select {
		case v := <-a:
			t += v
		case v := <-b:
			t -= v * 2
		case <-done:
			return t
		}
	}
}

// recurse recurses deeply, with a loop in every frame.
func recurse(d int) int {
	if d == 0 {
		return 1
	}
	t := 0
	for i := 0; i < 2; i++ {
		if i == 0 {
			t += recurse(d - 1)
		} else {
			t += d % 7
		}
	}
	return t % 1000007
}

// panicDeep panics many frames down and recovers in a loop.
func panicDeep(n int) int {
	t := 0
	for i := 0; i < n; i++ {
		func() {
			defer func() {
				if e, ok := recover().(error); ok {
					t += len(e.Error())
				}
			}()
			var f func(int)
			f = func(d int) {
				if d == 0 {
					panic(errors.New(strings.Repeat("x", i%5+1)))
				}
				f(d - 1)
			}
			f(i * 10)
		}()
	}
	return t
}

// recoverInLoop panics in a loop with returns inside it, and recovers in
// its own defer, which resumes at its deferreturn call.
func recoverInLoop(n int) (r int) {
	defer func() {
		if e := recover(); e != nil {
			r = -r
		}
	}()
	for i := 0; i < n; i++ {
		for j := 0; j < 3; j++ {
			if i == n-1 && j == 2 {
				panic("x")
			}
			if i > 10*n {
				return i
			}
			r += i * j
			if j == 1 {
				runtime.Gosched()
			}
		}
	}
	return r
}

func main() {
	results := []struct {
		name string
		f    func() int
	}{
		{"pingpong", func() int { return pingpong(2000) }},
		{"grow", func() int { return grow(12) }},
		{"recovering", func() int { return recovering(300) }},
		{"deferInLoop", func() int { return deferInLoop(500) }},
		{"indirect", func() int { return indirect(5000) }},
		{"irreducible-even", func() int { return irreducible(1000) }},
		{"irreducible-odd", func() int { return irreducible(1001) }},
		{"labeled", func() int { return labeled(40) }},
		{"workers", func() int { return workers(3000) }},
		{"selectLoop", func() int { return selectLoop(3000) }},
		{"recurse", func() int { return recurse(100000) }},
		{"panicDeep", func() int { return panicDeep(200) }},
		{"recoverInLoop", func() int { return recoverInLoop(300) }},
	}
	for _, r := range results {
		fmt.Printf("%s %d\n", r.name, r.f())
	}
}
