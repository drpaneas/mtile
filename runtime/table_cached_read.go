package runtime

// TableGetCachedNumber is a compiler-only warm read. The caller has checked a
// nonzero offset and retains TableGetStableNumber's numeric/layout proof. The
// stable handle is resolved each time so a previous collection may move it.
// Compact arguments avoid passing unused table/key Value payloads on ARM.
//
//go:noinline
//go:section .iwram
func (c *Context) TableGetCachedNumber(ref Ref, cache TableFieldCache) Number {
	header := &c.heap.handles[ref]
	return readValue(c.activeSpace(), header.Offset+uint32(cache)-1).Num
}
