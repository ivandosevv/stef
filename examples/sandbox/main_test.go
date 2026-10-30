package main

import "testing"

func TestGreetingMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{"stef", "Hello, stef!"},
		{"  stef  ", "Hello, stef!"},
		{"", "Hello, friend!"},
		{" \t\n ", "Hello, friend!"},
		{"  Иване  ", "Hello, Иване!"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Greeting{Name: tc.name}).Message(); got != tc.want {
				t.Fatalf("Message() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGreetArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"default", nil, "Hello, stef!"},
		{"padded Unicode", []string{"  Иване  "}, "Hello, Иване!"},
		{"blank", []string{" \t\n "}, "Hello, friend!"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := greet(tc.args); got != tc.want {
				t.Fatalf("greet(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}
