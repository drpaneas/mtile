//go:build mtile_mode0 && (!mtile_mode0_text || !mtile_mode0_obj)

package runtime

type mode0TextState struct{}

func mode0TextObjectTileReserved(id int) bool            { return false }
func mode0TextObjectWindowReserved(base, count int) bool { return false }
func (c *Context) beginMode0Text()                       {}
func (c *Context) presentMode0Text() uint32              { return 0 }
func (c *Context) hardwarePrint(s, x, y, color Value) (int, bool) {
	return 0, false
}
func (c *Context) hardwarePrintStatic(s, x, y, color Value) (int, bool) {
	return 0, false
}
