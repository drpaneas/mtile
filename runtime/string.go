package runtime

func (c *Context) heapStringBytes(ref Ref) []byte {
	data, err := c.objectBytes(ref)
	if err != HeapOK || c.heap.handles[ref].Kind() != ObjString || len(data) < 2 {
		return nil
	}
	n := int(readU16(data, 0))
	if 2+n > len(data) {
		return nil
	}
	if n == 0 {
		return data[2:2]
	}
	return data[2 : 2+n]
}

func (c *Context) NewString(b []byte) (Value, HeapError) {
	c.ensureHeap()
	if len(b) > MaxStringBytes {
		return NilVal(), HeapOOM
	}
	size := uint16(2 + len(b))
	ref, err := c.heap.allocObject(ObjString, size)
	if err != HeapOK {
		return NilVal(), err
	}
	data, err := c.objectBytes(ref)
	if err != HeapOK {
		return NilVal(), err
	}
	writeU16(data, 0, uint16(len(b)))
	for i := 0; i < len(b); i++ {
		data[2+i] = b[i]
	}
	return Value{Kind: KindString, Ref: ref}, HeapOK
}

func (c *Context) copyStringInto(v Value, dst []byte) int {
	src := c.stringBytes(v)
	n := len(src)
	if n > len(dst) {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		dst[i] = src[i]
	}
	return n
}

func (c *Context) Concat(a, b Value) (Value, HeapError) {
	if !c.PushTemp(a) {
		return NilVal(), HeapOOM
	}
	if !c.PushTemp(b) {
		c.PopTemp()
		return NilVal(), HeapOOM
	}
	if err := c.ensureString(&a); err != HeapOK {
		c.PopTemps(2)
		return NilVal(), err
	}
	c.heap.roots.temps[c.heap.roots.tempCount-2] = a
	if err := c.ensureString(&b); err != HeapOK {
		c.PopTemps(2)
		return NilVal(), err
	}
	c.heap.roots.temps[c.heap.roots.tempCount-1] = b
	total := c.stringLen(a) + c.stringLen(b)
	if total > MaxStringBytes {
		c.PopTemps(2)
		return NilVal(), HeapOOM
	}
	size := uint16(2 + total)
	ref, err := c.heap.allocObject(ObjString, size)
	if err != HeapOK {
		c.PopTemps(2)
		return NilVal(), err
	}
	data, err := c.objectBytes(ref)
	if err != HeapOK {
		c.PopTemps(2)
		return NilVal(), err
	}
	writeU16(data, 0, uint16(total))
	na := c.copyStringInto(a, data[2:])
	c.copyStringInto(b, data[2+na:])
	c.PopTemps(2)
	return Value{Kind: KindString, Ref: ref}, HeapOK
}

func (c *Context) ensureString(v *Value) HeapError {
	if isStringKind(v.Kind) {
		return HeapOK
	}
	s, err := c.Tostr(*v, false)
	if err != HeapOK {
		return err
	}
	*v = s
	return HeapOK
}

func (c *Context) Sub(s Value, i, j Value) (Value, HeapError) {
	if !c.PushTemp(s) {
		return NilVal(), HeapOOM
	}
	n := c.stringLen(s)
	start := luaIndex(i.IntOr(1), n)
	end := luaIndex(j.IntOr(n), n)
	if start < 1 {
		start = 1
	}
	if end > n {
		end = n
	}
	if start > end || n == 0 {
		c.PopTemp()
		return c.NewString(nil)
	}
	size := uint16(2 + (end - start + 1))
	ref, err := c.heap.allocObject(ObjString, size)
	if err != HeapOK {
		c.PopTemp()
		return NilVal(), err
	}
	data, err := c.objectBytes(ref)
	if err != HeapOK {
		c.PopTemp()
		return NilVal(), err
	}
	src := c.stringBytes(s)
	outN := end - start + 1
	writeU16(data, 0, uint16(outN))
	for k := 0; k < outN; k++ {
		data[2+k] = src[start-1+k]
	}
	c.PopTemp()
	return Value{Kind: KindString, Ref: ref}, HeapOK
}

func luaIndex(i, n int) int {
	if i < 0 {
		return n + 1 + i
	}
	return i
}

func (c *Context) Chr(n Value) (Value, HeapError) {
	var b [1]byte
	b[0] = byte(n.Int())
	return c.NewString(b[:])
}

func (c *Context) Ord(s, index Value) Value {
	i := index.IntOr(1)
	b := c.stringBytes(s)
	if i < 1 || i > len(b) {
		return NilVal()
	}
	return IntVal(int(b[i-1]))
}

func (c *Context) StrLen(s Value) int {
	return c.stringLen(s)
}

func (c *Context) Tostr(v Value, hex bool) (Value, HeapError) {
	if isStringKind(v.Kind) {
		if v.Kind == KindString {
			return v, HeapOK
		}
		n := c.stringLen(v)
		if !c.PushTemp(v) {
			return NilVal(), HeapOOM
		}
		ref, err := c.heap.allocObject(ObjString, uint16(2+n))
		if err != HeapOK {
			c.PopTemp()
			return NilVal(), err
		}
		data, err := c.objectBytes(ref)
		if err != HeapOK {
			c.PopTemp()
			return NilVal(), err
		}
		writeU16(data, 0, uint16(n))
		c.copyStringInto(v, data[2:])
		c.PopTemp()
		return Value{Kind: KindString, Ref: ref}, HeapOK
	}
	var rooted bool
	if v.Kind == KindTable {
		if !c.PushTemp(v) {
			return NilVal(), HeapOOM
		}
		rooted = true
	}
	var buf [64]byte
	n := 0
	switch v.Kind {
	case KindNil:
		n = copy(buf[:], "[nil]")
	case KindBool:
		if v.Num != 0 {
			n = copy(buf[:], "true")
		} else {
			n = copy(buf[:], "false")
		}
	case KindTable:
		n = copy(buf[:], "[table]")
	case KindNumber:
		n = formatNumber(buf[:], v.Num, hex)
	default:
		n = copy(buf[:], "[nil]")
	}
	result, err := c.NewString(buf[:n])
	if rooted {
		c.PopTemp()
	}
	return result, err
}

func formatNumber(dst []byte, n Number, hex bool) int {
	if hex {
		u := uint32(n)
		dst[0] = '0'
		dst[1] = 'x'
		putHex4(dst[2:], uint16(u>>16))
		dst[6] = '.'
		putHex4(dst[7:], uint16(u))
		return 11
	}
	neg := n < 0
	if neg {
		n = -n
	}
	ip := uint32(n) >> 16
	frac := uint32(n) & 0xFFFF
	i := 0
	if neg {
		dst[i] = '-'
		i++
	}
	i += uitoa(dst[i:], ip)
	if frac == 0 {
		return i
	}
	dst[i] = '.'
	i++
	scaled := frac * 10000 / 65536
	var digits [4]byte
	for d := 3; d >= 0; d-- {
		digits[d] = byte('0' + scaled%10)
		scaled /= 10
	}
	end := 4
	for end > 1 && digits[end-1] == '0' {
		end--
	}
	for d := 0; d < end; d++ {
		dst[i] = digits[d]
		i++
	}
	return i
}

func putHex4(dst []byte, v uint16) {
	const hexdigits = "0123456789abcdef"
	dst[0] = hexdigits[(v>>12)&0xF]
	dst[1] = hexdigits[(v>>8)&0xF]
	dst[2] = hexdigits[(v>>4)&0xF]
	dst[3] = hexdigits[v&0xF]
}

func uitoa(dst []byte, v uint32) int {
	if v == 0 {
		dst[0] = '0'
		return 1
	}
	var tmp [10]byte
	n := 0
	for v > 0 {
		tmp[n] = byte('0' + v%10)
		v /= 10
		n++
	}
	for i := 0; i < n; i++ {
		dst[i] = tmp[n-1-i]
	}
	return n
}

func (c *Context) Tonum(v Value) Value {
	if v.Kind == KindNumber {
		return v
	}
	if !isStringKind(v.Kind) {
		return NilVal()
	}
	b := c.stringBytes(v)
	n, ok := parsePicoNumber(b)
	if !ok {
		return NilVal()
	}
	return NumVal(n)
}

func parsePicoNumber(b []byte) (Number, bool) {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	if i >= len(b) {
		return 0, false
	}
	neg := false
	if b[i] == '-' {
		neg = true
		i++
	}
	if i+1 < len(b) && b[i] == '0' && (b[i+1] == 'x' || b[i+1] == 'X') {
		n, ok := parseHexNumber(b[i+2:])
		if !ok {
			return 0, false
		}
		if neg {
			n = -n
		}
		return n, true
	}
	ip := uint32(0)
	got := false
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		got = true
		ip = ip*10 + uint32(b[i]-'0')
		i++
	}
	frac := uint32(0)
	scale := uint32(1)
	if i < len(b) && b[i] == '.' {
		i++
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			got = true
			if scale <= 100000 {
				frac = frac*10 + uint32(b[i]-'0')
				scale *= 10
			}
			i++
		}
	}
	if !got {
		return 0, false
	}
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	if i != len(b) {
		return 0, false
	}
	n := Number(ip << 16)
	if scale > 1 {
		n += Number((uint64(frac) << 16) / uint64(scale))
	}
	if neg {
		n = -n
	}
	return n, true
}

func parseHexNumber(b []byte) (Number, bool) {
	i := 0
	ip := uint32(0)
	got := false
	for i < len(b) {
		d, ok := hexDigit(b[i])
		if !ok {
			break
		}
		got = true
		ip = (ip << 4) | uint32(d)
		i++
	}
	frac := uint32(0)
	fracDigits := 0
	if i < len(b) && b[i] == '.' {
		i++
		for i < len(b) {
			d, ok := hexDigit(b[i])
			if !ok {
				break
			}
			got = true
			if fracDigits < 4 {
				frac = (frac << 4) | uint32(d)
				fracDigits++
			}
			i++
		}
	}
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	if !got || i != len(b) {
		return 0, false
	}
	for fracDigits < 4 {
		frac <<= 4
		fracDigits++
	}
	return Number((ip << 16) | (frac & 0xFFFF)), true
}

func hexDigit(ch byte) (byte, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return ch - '0', true
	case ch >= 'a' && ch <= 'f':
		return ch - 'a' + 10, true
	case ch >= 'A' && ch <= 'F':
		return ch - 'A' + 10, true
	default:
		return 0, false
	}
}
