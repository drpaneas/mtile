// Font geometry experiment. Production code and project font requirements are
// deliberately unchanged; candidate runtimes exist only in a temp directory.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func main() {
	out := flag.String("out", "/tmp/mtile-font-geometry", "artifact directory")
	variants := flag.String("variants", "", "comma-separated research variant names")
	export := flag.Bool("export", false, "export baseline and candidate subsets only")
	flag.Parse()
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(*out, 0755))
	font, err := assetsync.LoadFont("", "international")
	must(err)
	raw, err := json.MarshalIndent(font, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(*out, "baseline.json"), raw, 0644))
	if *export {
		fmt.Println(filepath.Join(*out, "baseline.json"))
		return
	}
	must(runExperiment(root, *out, *variants))
}
func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
