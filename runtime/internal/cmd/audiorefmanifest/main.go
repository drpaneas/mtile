package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/drpaneas/mtile/runtime/internal/audioref"
)

type caseGroup struct {
	Kind  string   `json:"kind"`
	Title string   `json:"title"`
	Cases []string `json:"cases"`
}

type caseManifest struct {
	Version int         `json:"version"`
	Groups  []caseGroup `json:"groups"`
}

func activePSGCases() caseManifest {
	manifest := caseManifest{Version: 2}
	for kind := audioref.CH1; kind <= audioref.CH4; kind++ {
		group := caseGroup{Kind: fmt.Sprintf("ch%d", int(kind)+1), Title: audioref.KindName(kind)}
		for index := 0; index < audioref.CaseCount(kind); index++ {
			group.Cases = append(group.Cases, audioref.CaseName(kind, index))
		}
		manifest.Groups = append(manifest.Groups, group)
	}
	return manifest
}

func renderMarkdown(manifest caseManifest) string {
	var out strings.Builder
	out.WriteString("# Mtile Audio v2 physical-GBA results\n\n")
	out.WriteString("Fill this file while running the ROMs from the same bundle. A checked box means the named case was both audible and visually selected on the GBA. For every case, replace `PASS / FAIL` with exactly `PASS` or `FAIL`; leaving both words is an incomplete result.\n\n")
	out.WriteString("- Date/time:\n- Listener:\n- GBA model/serial:\n- Flash cartridge:\n- Output (speaker/headphones/capture):\n- Power source:\n- TinyGo version: see `BUILD-INFO.txt`\n- ROM hashes: see `SHA256SUMS`\n\n")
	for _, group := range manifest.Groups {
		fmt.Fprintf(&out, "## %s\n\n", group.Title)
		for index, name := range group.Cases {
			fmt.Fprintf(&out, "- [ ] %02d — %s: PASS / FAIL — notes:\n", index, name)
		}
		out.WriteString("- [ ] SELECT stops cleanly; START restarts the selected case.\n\n")
	}
	out.WriteString("## Managed controller\n\n")
	out.WriteString("- [ ] `audiopsg.gba`: the first note sounds after a cold boot.\n")
	out.WriteString("- [ ] `audiopsg.gba`: loop timing is stable and every restart begins cleanly.\n")
	out.WriteString("- [ ] `audiopsgcoexist.gba`: screen reaches `SELF TEST: PASS`.\n")
	out.WriteString("- [ ] `audiopsgcoexist.gba`: `HW CHANNELS SEEN` reaches 15.\n")
	out.WriteString("- [ ] All four channel roles remain separately audible for ten minutes.\n")
	out.WriteString("- [ ] No missing first note, drops, stuck notes, clicks, drift, or graphics interference.\n\n")
	out.WriteString("## Final decision\n\n- [ ] PASS\n- [ ] FAIL\n\nFailure notes / recording paths:\n")
	return out.String()
}

func main() {
	format := flag.String("format", "json", "output format: json or markdown")
	output := flag.String("output", "", "output file")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "-output is required")
		os.Exit(2)
	}

	manifest := activePSGCases()
	var data []byte
	var err error
	switch *format {
	case "json":
		data, err = json.MarshalIndent(manifest, "", "  ")
		data = append(data, '\n')
	case "markdown":
		data = []byte(renderMarkdown(manifest))
	default:
		fmt.Fprintf(os.Stderr, "unsupported format %q\n", *format)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
