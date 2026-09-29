//go:build mtile_mode0 && mtile_mode0_obj && !mtile_mode0_pspr

package runtime

type mode0PoseStreamState struct{}

func (c *Context) mode0PoseStageWords() []uint32 { return nil }
