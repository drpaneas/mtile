//go:build mtile_mode0 && mtile_mode0_obj && mtile_mode0_pspr

package runtime

// The compiler supplies exactly the staging capacity its windows require.
// Cartridges without streamed poses allocate no staging storage.
type mode0PoseStreamState struct{}

func (c *Context) mode0PoseStageWords() []uint32 {
	if c.cart == nil {
		return nil
	}
	return c.cart.PoseOBJStage
}
