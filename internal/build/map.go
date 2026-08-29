package build

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const EWRAMReserve uint64 = 4096

const (
	gbaEWRAMOrigin uint64 = 0x02000000
	// The final 512 bytes are the editor-only debug mailbox in preview builds.
	// The application EWRAM region in gba.ld therefore ends at 0x0203FE00.
	gbaEWRAMEnd    uint64 = 0x0203fe00
	gbaIWRAMOrigin uint64 = 0x03000000
	gbaIWRAMEnd    uint64 = 0x03007fa0

	// Export capacities so build clients can present the exact hardware budget
	// used by the linker rather than copying magic GBA limits into the UI.
	EWRAMCapacity = gbaEWRAMEnd - gbaEWRAMOrigin
	IWRAMCapacity = gbaIWRAMEnd - gbaIWRAMOrigin
)

// LinkerSymbols are the linker-defined EWRAM boundary symbols needed by the
// post-link heap gate.
type LinkerSymbols struct {
	EWRAMEnd uint64
	HeapEnd  uint64
	IWRAMEnd uint64
}

// ParseLinkerMap reads the lld map format emitted by the GBA linker. lld
// prints addresses without a 0x prefix and assignment symbols as the final
// field on their line; both normal and assignment forms are accepted.
func ParseLinkerMap(path string) (LinkerSymbols, error) {
	file, err := os.Open(path)
	if err != nil {
		return LinkerSymbols{}, fmt.Errorf("open linker map: %w", err)
	}
	defer func() { _ = file.Close() }()

	var symbols LinkerSymbols
	var foundEWRAM, foundHeap bool
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		address, ok := parseMapAddress(fields[0])
		if !ok {
			continue
		}
		for _, field := range fields[1:] {
			switch field {
			case "_ewram_end":
				symbols.EWRAMEnd = address
				foundEWRAM = true
			case "_heap_end":
				// lld leaves an assignment such as
				// `_heap_end = ORIGIN(ewram) + LENGTH(ewram)` in the
				// map. Its first column is the current location, not the
				// value of the expression. The GBA linker script reserves
				// the final 512 bytes for preview telemetry.
				if strings.Contains(line, "ORIGIN(ewram)") && strings.Contains(line, "LENGTH(ewram)") {
					symbols.HeapEnd = gbaEWRAMEnd
				} else {
					symbols.HeapEnd = address
				}
				foundHeap = true
			case "_ebss":
				// IWRAM is optional for the heap gate but useful build
				// telemetry. Older/fabricated map fixtures may omit it.
				symbols.IWRAMEnd = address
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return LinkerSymbols{}, fmt.Errorf("read linker map: %w", err)
	}
	if !foundEWRAM || !foundHeap {
		missing := make([]string, 0, 2)
		if !foundEWRAM {
			missing = append(missing, "_ewram_end")
		}
		if !foundHeap {
			missing = append(missing, "_heap_end")
		}
		return LinkerSymbols{}, fmt.Errorf("linker map is missing %s", strings.Join(missing, ", "))
	}
	return symbols, nil
}

func parseMapAddress(value string) (uint64, bool) {
	value = strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
	if value == "" {
		return 0, false
	}
	address, err := strconv.ParseUint(value, 16, 64)
	return address, err == nil
}

func validateHeapBudget(symbols LinkerSymbols, reserve uint64) error {
	if symbols.EWRAMEnd < gbaEWRAMOrigin || symbols.EWRAMEnd > gbaEWRAMEnd {
		return fmt.Errorf("linker map EWRAM end 0x%x is outside application EWRAM", symbols.EWRAMEnd)
	}
	if symbols.HeapEnd > gbaEWRAMEnd {
		return fmt.Errorf("linker map heap end 0x%x exceeds application EWRAM", symbols.HeapEnd)
	}
	if symbols.HeapEnd < symbols.EWRAMEnd {
		return fmt.Errorf("linker map has heap end 0x%x before EWRAM end 0x%x", symbols.HeapEnd, symbols.EWRAMEnd)
	}
	free := symbols.HeapEnd - symbols.EWRAMEnd
	if free < reserve {
		return fmt.Errorf("EWRAM heap leftover is %d bytes, need at least %d", free, reserve)
	}
	return nil
}

// ValidateHeapBudget exposes the post-link policy to callers and tests.
func ValidateHeapBudget(symbols LinkerSymbols) error {
	return validateHeapBudget(symbols, EWRAMReserve)
}
