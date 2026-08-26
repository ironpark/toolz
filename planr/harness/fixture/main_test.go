package main

import "testing"

func TestFormatGreeting(t *testing.T) {
	if got, want := formatGreeting("Ada"), "Hello, Ada!"; got != want {
		t.Fatalf("formatGreeting() = %q, want %q", got, want)
	}
}
