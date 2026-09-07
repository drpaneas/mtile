//go:build mtile_mode0 && mtile_mode0_obj && !mtile_mode0_objpack

package runtime

type mode0ObjectPackState struct{}

func (c *Context) hasMode0PackedUploads() bool { return false }
func (c *Context) prepareMode0PackedObjects()  {}

func mode0PackedObjectReservedSource(id int) bool                  { return false }
func (c *Context) mode0PackedObjectTile(id, cells int) (int, bool) { return 0, false }
func (c *Context) dirtyMode0PackedObject(id int)                   {}
func (c *Context) presentMode0PackedObjects() uint32               { return 0 }
