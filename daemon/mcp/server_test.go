package mcp_test

import (
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"testing"
)

func TestBridgeOnlyAcceptsLoopbackOrigins(t *testing.T) {
	for _, endpoint := range []string{"https://127.0.0.1:4820", "http://example.com", "http://user:pass@localhost:4820", "http://localhost:4820/private", "http://localhost:4820?redirect=1", "http://localhost:4820#fragment"} {
		t.Run(endpoint, func(t *testing.T) {
			if _, err := bridge.NewServer(endpoint); err == nil {
				t.Fatal("expected URL rejection")
			}
		})
	}
	for _, endpoint := range []string{"http://localhost:4820", "http://127.0.0.1:4820", "http://[::1]:4820"} {
		if _, err := bridge.NewServer(endpoint); err != nil {
			t.Fatal(err)
		}
	}
}
