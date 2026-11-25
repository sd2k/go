// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip3

package runtime

// sleep and wakeup on one-time events, like note_other.go, plus the
// goroutine parked in an untimed notetsleepg, which notewakeup readies.
type note struct {
	key uintptr
	gp  guintptr // not *g: notewakeup runs where write barriers are prohibited
}
