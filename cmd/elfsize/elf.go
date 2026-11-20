package main

import (
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
)

type span struct{ start, end uint64 }

var (
	romRegion     = span{0x08000000, 0x0a000000}
	iwramRegion   = span{0x03000000, 0x03007fa0}
	ewramRegion   = span{0x02000000, 0x0203fe00}
	mailboxRegion = span{0x0203fe00, 0x02040000}
)

type cost struct {
	ROM     uint64 `json:"romLoadBytes"`
	IWRAM   uint64 `json:"iwramBytes"`
	EWRAM   uint64 `json:"ewramBytes"`
	Mailbox uint64 `json:"debugMailboxBytes"`
	Other   uint64 `json:"otherAllocatedBytes"`
}

type sectionSize struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	VMA          uint64  `json:"vma"`
	LMA          *uint64 `json:"lma,omitempty"`
	Size         uint64  `json:"size"`
	Cost         cost    `json:"cost"`
	Unattributed uint64  `json:"unattributedBytes"`
}

type symbolSize struct {
	Names   []string `json:"names"`
	Section string   `json:"section"`
	VMA     uint64   `json:"vma"`
	LMA     *uint64  `json:"lma,omitempty"`
	Size    uint64   `json:"size"`
	Cost    cost     `json:"cost"`
}

type imageSize struct {
	Path              string        `json:"path"`
	Totals            cost          `json:"totals"`
	ROMImageBytes     uint64        `json:"romImageBytes"`
	ROMSegmentPadding uint64        `json:"romSegmentPaddingBytes"`
	SymbolsAvailable  bool          `json:"symbolsAvailable"`
	Sections          []sectionSize `json:"sections"`
	Symbols           []symbolSize  `json:"symbols,omitempty"`
}

func checkedSpan(start, size uint64) (span, error) {
	if size > ^uint64(0)-start {
		return span{}, fmt.Errorf("address range overflows: %#x + %#x", start, size)
	}
	return span{start, start + size}, nil
}

func intersection(a, b span) span {
	start, end := max(a.start, b.start), min(a.end, b.end)
	if end < start {
		end = start
	}
	return span{start, end}
}

func unionBytes(ranges []span) uint64 {
	if len(ranges) == 0 {
		return 0
	}
	ranges = slices.Clone(ranges)
	slices.SortFunc(ranges, func(a, b span) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		return 0
	})
	current := ranges[0]
	var total uint64
	for _, r := range ranges[1:] {
		if r.start <= current.end {
			current.end = max(current.end, r.end)
		} else {
			total += current.end - current.start
			current = r
		}
	}
	return total + current.end - current.start
}

func regionBytes(ranges []span, region span) uint64 {
	clipped := make([]span, 0, len(ranges))
	for _, r := range ranges {
		s := intersection(r, region)
		if s.end > s.start {
			clipped = append(clipped, s)
		}
	}
	return unionBytes(clipped)
}

func memoryCost(ranges []span) cost {
	c := cost{IWRAM: regionBytes(ranges, iwramRegion), EWRAM: regionBytes(ranges, ewramRegion), Mailbox: regionBytes(ranges, mailboxRegion)}
	c.Other = unionBytes(ranges) - regionBytes(ranges, romRegion) - c.IWRAM - c.EWRAM - c.Mailbox
	return c
}

// A section's load address comes from its file-backed PT_LOAD mapping, not
// its VMA or its name. In mTile .data contains executable IWRAM functions.
func sectionLMA(s *elf.Section, programs []*elf.Prog) (*uint64, error) {
	if s.Size == 0 || s.Type == elf.SHT_NOBITS {
		return nil, nil
	}
	var lma *uint64
	for _, p := range programs {
		if p.Type != elf.PT_LOAD || s.Offset < p.Off || s.Addr < p.Vaddr {
			continue
		}
		delta := s.Offset - p.Off
		if delta > p.Filesz || s.Size > p.Filesz-delta || s.Addr-p.Vaddr != delta {
			continue
		}
		if delta > p.Memsz || s.Size > p.Memsz-delta {
			continue
		}
		address := p.Paddr + delta
		if lma != nil && *lma != address {
			return nil, fmt.Errorf("section %s has conflicting PT_LOAD addresses", s.Name)
		}
		lma = &address
	}
	if lma == nil {
		return nil, fmt.Errorf("allocated section %s has no complete file-backed PT_LOAD mapping", s.Name)
	}
	return lma, nil
}

func analyzeELF(r io.ReaderAt, path string) (imageSize, error) {
	f, err := elf.NewFile(r)
	if err != nil {
		return imageSize{}, fmt.Errorf("read ELF: %w", err)
	}
	for _, p := range f.Progs {
		if p.Type != elf.PT_LOAD || p.Filesz == 0 {
			continue
		}
		backing, err := checkedSpan(p.Off, p.Filesz)
		if err != nil {
			return imageSize{}, err
		}
		if backing.end > math.MaxInt64 {
			return imageSize{}, fmt.Errorf("PT_LOAD file range exceeds supported file offsets")
		}
		var last [1]byte
		if _, err := r.ReadAt(last[:], int64(backing.end-1)); err != nil {
			return imageSize{}, fmt.Errorf("truncated PT_LOAD file backing: %w", err)
		}
	}
	return analyzeFile(f, path)
}

func analyzeFile(f *elf.File, path string) (imageSize, error) {
	out := imageSize{Path: path, Sections: []sectionSize{}}
	if f.Class != elf.ELFCLASS32 || f.Machine != elf.EM_ARM || f.Type != elf.ET_EXEC || f.Data != elf.ELFDATA2LSB {
		return out, fmt.Errorf("expected a linked little-endian ELF32 ARM GBA executable")
	}
	var romRanges, vmRanges, sectionLoadRanges []span
	for _, p := range f.Progs {
		if p.Type != elf.PT_LOAD {
			continue
		}
		if p.Filesz > p.Memsz {
			return out, fmt.Errorf("PT_LOAD file size exceeds memory size")
		}
		if _, err := checkedSpan(p.Off, p.Filesz); err != nil {
			return out, err
		}
		if _, err := checkedSpan(p.Vaddr, p.Memsz); err != nil {
			return out, err
		}
		load, err := checkedSpan(p.Paddr, p.Filesz)
		if err != nil {
			return out, err
		}
		if p.Filesz == 0 {
			continue
		}
		if load.start < romRegion.start || load.end > romRegion.end {
			return out, fmt.Errorf("file-backed PT_LOAD %#x..%#x is outside GBA ROM", load.start, load.end)
		}
		romRanges = append(romRanges, load)
		out.ROMImageBytes = max(out.ROMImageBytes, load.end-romRegion.start)
	}
	sectionRows := make(map[elf.SectionIndex]sectionSize)
	sectionOrdinals := make(map[elf.SectionIndex]int)
	for index, s := range f.Sections {
		if s.Flags&elf.SHF_ALLOC == 0 {
			continue
		}
		vm, err := checkedSpan(s.Addr, s.Size)
		if err != nil {
			return out, err
		}
		if s.Size > 0 {
			vmRanges = append(vmRanges, vm)
		}
		lma, err := sectionLMA(s, f.Progs)
		if err != nil {
			return out, err
		}
		c := memoryCost([]span{vm})
		if lma != nil {
			c.ROM = s.Size
			sectionLoadRanges = append(sectionLoadRanges, span{*lma, *lma + s.Size})
		}
		row := sectionSize{Name: s.Name, Type: s.Type.String(), VMA: s.Addr, LMA: lma, Size: s.Size, Cost: c, Unattributed: s.Size}
		sectionOrdinals[elf.SectionIndex(index)] = len(out.Sections)
		out.Sections = append(out.Sections, row)
		sectionRows[elf.SectionIndex(index)] = row
	}
	out.Totals = memoryCost(vmRanges)
	out.Totals.ROM = unionBytes(romRanges)
	out.ROMSegmentPadding = out.Totals.ROM - unionBytes(sectionLoadRanges)
	syms, err := f.Symbols()
	if errors.Is(err, elf.ErrNoSymbols) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read symbols: %w", err)
	}
	out.SymbolsAvailable = true
	type aliasKey struct {
		section       elf.SectionIndex
		address, size uint64
	}
	groups := make(map[aliasKey]int)
	coverage := make(map[elf.SectionIndex][]span)
	for _, s := range syms {
		kind := elf.ST_TYPE(s.Info)
		if s.Size == 0 || s.Name == "" || (kind != elf.STT_FUNC && kind != elf.STT_OBJECT && kind != elf.STT_NOTYPE) || s.Section == elf.SHN_UNDEF || s.Section >= elf.SHN_LORESERVE {
			continue
		}
		if int(s.Section) >= len(f.Sections) {
			return out, fmt.Errorf("symbol %s references missing section %d", s.Name, s.Section)
		}
		section, ok := sectionRows[s.Section]
		if !ok {
			continue
		}
		address := s.Value
		if kind == elf.STT_FUNC {
			address &^= 1
		} // ARM ELF encodes Thumb state in bit zero.
		vm, err := checkedSpan(address, s.Size)
		if err != nil {
			return out, err
		}
		if address < section.VMA || vm.end > section.VMA+section.Size {
			return out, fmt.Errorf("symbol %s extends outside section %s", s.Name, section.Name)
		}
		key := aliasKey{s.Section, address, s.Size}
		coverage[s.Section] = append(coverage[s.Section], vm)
		if existing, ok := groups[key]; ok {
			out.Symbols[existing].Names = append(out.Symbols[existing].Names, s.Name)
			continue
		}
		row := symbolSize{Names: []string{s.Name}, Section: section.Name, VMA: address, Size: s.Size, Cost: memoryCost([]span{vm})}
		if section.LMA != nil {
			lma := *section.LMA + address - section.VMA
			row.LMA = &lma
			row.Cost.ROM = s.Size
		}
		groups[key] = len(out.Symbols)
		out.Symbols = append(out.Symbols, row)
	}
	for index, ranges := range coverage {
		row := &out.Sections[sectionOrdinals[index]]
		row.Unattributed = row.Size - unionBytes(ranges)
	}
	for i := range out.Symbols {
		slices.Sort(out.Symbols[i].Names)
		out.Symbols[i].Names = slices.Compact(out.Symbols[i].Names)
	}
	slices.SortFunc(out.Symbols, func(a, b symbolSize) int {
		if a.Size > b.Size {
			return -1
		}
		if a.Size < b.Size {
			return 1
		}
		return strings.Compare(symbolKey(a), symbolKey(b))
	})
	return out, nil
}

type signedCost struct {
	ROM     int64 `json:"romLoadBytes"`
	IWRAM   int64 `json:"iwramBytes"`
	EWRAM   int64 `json:"ewramBytes"`
	Mailbox int64 `json:"debugMailboxBytes"`
	Other   int64 `json:"otherAllocatedBytes"`
}

func subtract(a, b cost) signedCost {
	return signedCost{int64(a.ROM) - int64(b.ROM), int64(a.IWRAM) - int64(b.IWRAM), int64(a.EWRAM) - int64(b.EWRAM), int64(a.Mailbox) - int64(b.Mailbox), int64(a.Other) - int64(b.Other)}
}

type sizeDelta struct {
	Name         string     `json:"name"`
	Section      string     `json:"section,omitempty"`
	Before       uint64     `json:"beforeBytes"`
	After        uint64     `json:"afterBytes"`
	Delta        int64      `json:"deltaBytes"`
	Cost         signedCost `json:"costDelta"`
	Unattributed int64      `json:"unattributedBytesDelta,omitempty"`
}

type comparison struct {
	Totals            signedCost  `json:"totalsDelta"`
	ROMImageBytes     int64       `json:"romImageBytesDelta"`
	ROMSegmentPadding int64       `json:"romSegmentPaddingBytesDelta"`
	Sections          []sizeDelta `json:"sections"`
	Symbols           []sizeDelta `json:"symbols,omitempty"`
	SymbolsComparable bool        `json:"symbolsComparable"`
}

func symbolKey(s symbolSize) string { return s.Section + "\x00" + strings.Join(s.Names, "\x00") }

func addCost(a, b cost) cost {
	return cost{a.ROM + b.ROM, a.IWRAM + b.IWRAM, a.EWRAM + b.EWRAM, a.Mailbox + b.Mailbox, a.Other + b.Other}
}

// Deltas match section + sorted alias names, never addresses that shift after
// linking. Repeated local names are aggregated. Renamed alias groups appear as
// removals/additions. Symbol rows explain contributors; they are not totals.
func compareImages(before, after imageSize) comparison {
	out := comparison{Totals: subtract(after.Totals, before.Totals), ROMImageBytes: int64(after.ROMImageBytes) - int64(before.ROMImageBytes), ROMSegmentPadding: int64(after.ROMSegmentPadding) - int64(before.ROMSegmentPadding), Sections: []sizeDelta{}, SymbolsComparable: before.SymbolsAvailable && after.SymbolsAvailable}
	type entry struct {
		name, section string
		size          uint64
		cost          cost
		unattributed  uint64
	}
	diff := func(left, right map[string]entry) []sizeDelta {
		keys := make(map[string]bool)
		for key := range left {
			keys[key] = true
		}
		for key := range right {
			keys[key] = true
		}
		var rows []sizeDelta
		for key := range keys {
			a, b := right[key], left[key]
			name, section := a.name, a.section
			if name == "" {
				name, section = b.name, b.section
			}
			row := sizeDelta{Name: name, Section: section, Before: b.size, After: a.size, Delta: int64(a.size) - int64(b.size), Cost: subtract(a.cost, b.cost), Unattributed: int64(a.unattributed) - int64(b.unattributed)}
			if row.Delta != 0 || row.Cost != (signedCost{}) || row.Unattributed != 0 {
				rows = append(rows, row)
			}
		}
		abs := func(v int64) int64 {
			if v < 0 {
				return -v
			}
			return v
		}
		slices.SortFunc(rows, func(a, b sizeDelta) int {
			if abs(a.Delta) > abs(b.Delta) {
				return -1
			}
			if abs(a.Delta) < abs(b.Delta) {
				return 1
			}
			return strings.Compare(a.Section+"\x00"+a.Name, b.Section+"\x00"+b.Name)
		})
		return rows
	}
	sections := func(image imageSize) map[string]entry {
		m := make(map[string]entry)
		for _, s := range image.Sections {
			old := m[s.Name]
			m[s.Name] = entry{name: s.Name, size: old.size + s.Size, cost: addCost(old.cost, s.Cost), unattributed: old.unattributed + s.Unattributed}
		}
		return m
	}
	symbols := func(image imageSize) map[string]entry {
		m := make(map[string]entry)
		for _, s := range image.Symbols {
			key := symbolKey(s)
			old := m[key]
			m[key] = entry{name: strings.Join(s.Names, " = "), section: s.Section, size: old.size + s.Size, cost: addCost(old.cost, s.Cost)}
		}
		return m
	}
	out.Sections = diff(sections(before), sections(after))
	if out.SymbolsComparable {
		out.Symbols = diff(symbols(before), symbols(after))
	}
	return out
}
