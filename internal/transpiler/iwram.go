package transpiler

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func (g *generator) validateIWRAMFunctions() error {
	if err := assetsync.ValidateIWRAMFunctions(g.opts.IWRAMFunctions); err != nil {
		return fmt.Errorf("transpiler: iwramFunctions: %w", err)
	}
	for _, name := range g.opts.IWRAMFunctions {
		if g.funcs[name] == nil {
			return fmt.Errorf("transpiler: iwramFunctions: unknown top-level function %q", name)
		}
	}
	return nil
}
