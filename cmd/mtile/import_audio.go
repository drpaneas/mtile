package main

import (
	"flag"
	"fmt"
	"github.com/drpaneas/mtile/internal/audioimport"
	"io"
	"strings"
)

func runImportAudio(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("import-audio", flag.ContinueOnError)
	project := flags.String("project", ".", "project directory receiving the imported audio")
	converter := flags.String("mmutil", "", "Maxmod converter executable (default: MMUTIL or PATH)")
	help, err := parseFlags(flags, args, errOut)
	if err != nil || help {
		return err
	}
	result, err := audioimport.Import(*project, flags.Args(), *converter)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(result.ConverterOutput, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "warning") || strings.Contains(lower, "unsupported") || strings.Contains(lower, "truncat") {
			fmt.Fprintln(errOut, line)
		}
	}
	fmt.Fprintln(out, "Imported Direct Sound audio (Maxmod, 16 kHz stereo output).")
	for id, item := range result.Manifest.Music {
		if item.Deleted {
			continue
		}
		fmt.Fprintf(out, "  pcm_music(%d)  %s\n", id, item.Name)
	}
	for id, item := range result.Manifest.Samples {
		if item.Deleted {
			continue
		}
		fmt.Fprintf(out, "  sample(%d)     %s\n", id, item.Name)
	}
	return nil
}
