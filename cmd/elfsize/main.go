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
	defer f.Close()
	size, err := analyzeELF(f, path)
	if err != nil {
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
	if r.Before != nil {
		fmt.Fprintf(tw, "%s -> %s\n", r.Before.Path, r.After.Path)
		fmt.Fprintln(tw, "BYTES\tBEFORE\tAFTER\tDELTA")
	} else {
		fmt.Fprintln(tw, r.After.Path)
		fmt.Fprintln(tw, "BYTES\tVALUE")
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
			fmt.Fprintf(tw, "%s\t%d\t%d\t%+d\n", row.name, row.before, row.after, int64(row.after)-int64(row.before))
		} else {
			fmt.Fprintf(tw, "%s\t%d\n", row.name, row.after)
		}
	}
	if r.Comparison != nil {
		for _, table := range []struct {
			name string
			rows []sizeDelta
		}{{"SECTION CHANGES", r.Comparison.Sections}, {"SYMBOL CHANGES (aliases grouped)", r.Comparison.Symbols}} {
			fmt.Fprintf(tw, "\n%s\nNAME\tSECTION\tBEFORE\tAFTER\tDELTA\tROM\tIWRAM\tEWRAM\tUNATTRIBUTED\n", table.name)
			for _, row := range table.rows {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%+d\t%+d\t%+d\t%+d\t%+d\n", row.Name, row.Section, row.Before, row.After, row.Delta, row.Cost.ROM, row.Cost.IWRAM, row.Cost.EWRAM, row.Unattributed)
			}
		}
		if !r.Comparison.SymbolsComparable {
			fmt.Fprintln(tw, "Symbol comparison unavailable: at least one ELF has no symbol table.")
		}
	} else {
		fmt.Fprintln(tw, "\nALLOCATED SECTIONS\nNAME\tTYPE\tVMA\tLMA\tSIZE\tROM\tIWRAM\tEWRAM\tUNATTRIBUTED")
		for _, s := range r.After.Sections {
			lma := "-"
			if s.LMA != nil {
				lma = fmt.Sprintf("%#x", *s.LMA)
			}
			fmt.Fprintf(tw, "%s\t%s\t%#x\t%s\t%d\t%d\t%d\t%d\t%d\n", s.Name, s.Type, s.VMA, lma, s.Size, s.Cost.ROM, s.Cost.IWRAM, s.Cost.EWRAM, s.Unattributed)
		}
		fmt.Fprintln(tw, "\nLARGEST SYMBOLS (aliases grouped)\nNAME\tSECTION\tSIZE\tROM\tIWRAM\tEWRAM")
		for _, s := range r.After.Symbols {
			fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\n", strings.Join(s.Names, " = "), s.Section, s.Size, s.Cost.ROM, s.Cost.IWRAM, s.Cost.EWRAM)
		}
		if !r.After.SymbolsAvailable {
			fmt.Fprintln(tw, "No symbol table; section totals remain available.")
		}
	}
	fmt.Fprintln(tw, "\nROM and RAM are separate costs: initialized IWRAM occupies both. NOBITS occupies RAM only.")
	fmt.Fprintln(tw, "Totals use range unions. Section/symbol rows can overlap and are not additive; symbols omit padding and unsized assembly.")
	return tw.Flush()
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "elfsize:", err)
		os.Exit(1)
	}
}
