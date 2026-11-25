// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip3

package signal

import (
	"os"
	"path/filepath"
	"testing"
)

// Notify starts a receiver that waits for the life of the program. It used to
// keep the scheduler from ever polling, so the read below never completed.
func TestNotifyThenReadFile(t *testing.T) {
	c := make(chan os.Signal, 1)
	Notify(c, os.Interrupt)
	defer Stop(c)

	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "hello" {
		t.Fatalf("ReadFile = %q, %v", b, err)
	}
}
