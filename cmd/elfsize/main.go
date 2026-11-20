// elfsize explains the section and symbol costs of linked mTile GBA ELFs.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

type report struct {
	Before     *imageSize  `json:"before,omitempty"`
	After      imageSize   `json:"after"`
	Comparison *comparison `json:"comparison,omitempty"`
}

func readImage(path string) (imageSize, error) {
	f, err := os.Open(path)
	if err != nil {
		return imageSize{}, err
	}
	size, err := analyzeELF(f, path)
	if err := errors.Join(err, f.Close()); err != nil {
		return imageSize{}, fmt.Errorf("%s: %w", path, err)
	}
	return size, nil
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("elfsize", flag.ContinueOnError)
	flags.SetOutput(stderr)
	beforePath := flags.String("before", "", "optional baseline ELF")
	afterPath := flags.String("after", "", "candidate ELF (or use one positional path)")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	top := flags.Int("top", 10, "maximum symbol rows, 0 for all")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *top < 0 {
		return fmt.Errorf("-top must be nonnegative")
	}
	if flags.NArg() == 1 && *afterPath == "" {
		*afterPath = flags.Arg(0)
	} else if flags.NArg() != 0 {
		return fmt.Errorf("use one ELF path or -after, not both")
	}
	if *afterPath == "" {
		return fmt.Errorf("usage: elfsize [-before baseline.elf] [-json] [-top 10] [-after] candidate.elf")
	}
	after, err := readImage(*afterPath)
	if err != nil {
		return err
	}
	out := report{After: after}
	if *beforePath != "" {
		before, err := readImage(*beforePath)
		if err != nil {
			return err
		}
		delta := compareImages(before, after)
		if *top > 0 && len(delta.Symbols) > *top {
			delta.Symbols = delta.Symbols[:*top]
		}
		before.Symbols = nil
		out.Before = &before
		out.After.Symbols = nil
		out.Comparison = &delta
	} else if *top > 0 && len(out.After.Symbols) > *top {
		out.After.Symbols = out.After.Symbols[:*top]
	}
	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	}
	return writeText(stdout, out)
}

func writeText(w io.Writer, r report) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	var writeErr error
	linef := func(format string, args ...any) {
		if writeErr == nil {
			_, writeErr = fmt.Fprintf(tw, format+"\n", args...)
		}
	}
	if r.Before != nil {
		linef("%s -> %s", r.Before.Path, r.After.Path)
		linef("BYTES\tBEFORE\tAFTER\tDELTA")
	} else {
		linef("%s", r.After.Path)
		linef("BYTES\tVALUE")
	}
	after := r.After
	before := imageSize{}
	if r.Before != nil {
		before = *r.Before
	}
	for _, row := range []struct {
		name          string
		before, after uint64
	}{
		{"ROM load (union)", before.Totals.ROM, after.Totals.ROM},
		{"ROM image span from origin", before.ROMImageBytes, after.ROMImageBytes},
		{"ROM segment padding outside sections", before.ROMSegmentPadding, after.ROMSegmentPadding},
		{"IWRAM allocated", before.Totals.IWRAM, after.Totals.IWRAM},
		{"EWRAM application allocated", before.Totals.EWRAM, after.Totals.EWRAM},
		{"EWRAM debug mailbox", before.Totals.Mailbox, after.Totals.Mailbox},
		{"Other allocated", before.Totals.Other, after.Totals.Other},
	} {
		if r.Before != nil {
			linef("%s\t%d\t%d\t%+d", row.name, row.before, row.after, int64(row.after)-int64(row.before))
		} else {
			linef("%s\t%d", row.name, row.after)
		}
	}
	if r.Comparison != nil {
		for _, table := range []struct {
			name string
			rows []sizeDelta
		}{{"SECTION CHANGES", r.Comparison.Sections}, {"SYMBOL CHANGES (aliases grouped)", r.Comparison.Symbols}} {
			linef("\n%s\nNAME\tSECTION\tBEFORE\tAFTER\tDELTA\tROM\tIWRAM\tEWRAM\tUNATTRIBUTED", table.name)
			for _, row := range table.rows {
				linef("%s\t%s\t%d\t%d\t%+d\t%+d\t%+d\t%+d\t%+d", row.Name, row.Section, row.Before, row.After, row.Delta, row.Cost.ROM, row.Cost.IWRAM, row.Cost.EWRAM, row.Unattributed)
			}
		}
		if !r.Comparison.SymbolsComparable {
			linef("Symbol comparison unavailable: at least one ELF has no symbol table.")
		}
	} else {
		linef("\nALLOCATED SECTIONS\nNAME\tTYPE\tVMA\tLMA\tSIZE\tROM\tIWRAM\tEWRAM\tUNATTRIBUTED")
		for _, s := range r.After.Sections {
			lma := "-"
			if s.LMA != nil {
				lma = fmt.Sprintf("%#x", *s.LMA)
			}
			linef("%s\t%s\t%#x\t%s\t%d\t%d\t%d\t%d\t%d", s.Name, s.Type, s.VMA, lma, s.Size, s.Cost.ROM, s.Cost.IWRAM, s.Cost.EWRAM, s.Unattributed)
		}
		linef("\nLARGEST SYMBOLS (aliases grouped)\nNAME\tSECTION\tSIZE\tROM\tIWRAM\tEWRAM")
		for _, s := range r.After.Symbols {
			linef("%s\t%s\t%d\t%d\t%d\t%d", strings.Join(s.Names, " = "), s.Section, s.Size, s.Cost.ROM, s.Cost.IWRAM, s.Cost.EWRAM)
		}
		if !r.After.SymbolsAvailable {
			linef("No symbol table; section totals remain available.")
		}
	}
	linef("\nROM and RAM are separate costs: initialized IWRAM occupies both. NOBITS occupies RAM only.")
	linef("Totals use range unions. Section/symbol rows can overlap and are not additive; symbols omit padding and unsized assembly.")
	if writeErr != nil {
		return writeErr
	}
	return tw.Flush()
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "elfsize:", err)
		os.Exit(1)
	}
}
