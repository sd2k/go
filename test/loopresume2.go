// runoutput

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Generate a loop with thousands of resume points (calls) and an
// indirect call to a runtime function. On wasm, the callee's table index
// (4096 and up) is then within the loop's resume pcs, so a stale PC_B
// left by the indirect call would make the loop's dispatch resume at the
// wrong call.

package main

import "fmt"

const calls = 6000

func main() {
	fmt.Print(`package main

import "runtime"

//go:noinline
func g(i, k int) int { return i ^ k }

func bigLoop(n int) int {
	f := runtime.NumGoroutine
	t := 0
	for i := 0; i < n; i++ {
`)
	for k := range calls {
		fmt.Printf("\t\tt += g(i, %d)\n", k)
	}
	fmt.Printf(`		t += f()
	}
	return t
}

func main() {
	const n = 50
	want := 0
	for i := 0; i < n; i++ {
		for k := 0; k < %d; k++ {
			want += i ^ k
		}
		want += runtime.NumGoroutine()
	}
	if got := bigLoop(n); got != want {
		println("bigLoop =", got, "want", want)
	}
}
`, calls)
}
