//go:build !darwin || !cgo

package companion

import (
	"context"
	"fmt"
)

func Available() bool { return false }
func Open(context.Context, string, string, func(string) (string, error)) error {
	return fmt.Errorf("native companion currently requires macOS with CGO; use --headless for the local engine")
}
