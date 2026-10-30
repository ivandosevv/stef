//go:build !darwin || !cgo

package companion

import (
	"context"
	"fmt"
)

func Available() bool { return false }
func Open(context.Context, string, string, func(string) (string, error)) error {
	return fmt.Errorf("this build uses the IDE or browser view; start the local engine with --view ide")
}
