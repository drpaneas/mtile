//go:build mtile_mode0 && !mtile_mode0_text

package runtime

type mode0TextState struct{}

func (c *Context) beginMode0Text()          {}
func (c *Context) presentMode0Text() uint32 { return 0 }
func (c *Context) hardwarePrint(s, x, y, color Value) (int, bool) {
	return 0, false
}
