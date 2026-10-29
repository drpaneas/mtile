// Font typography evidence runner. Production mode measures the shipped runtime;
// historical patched-runtime probes are reproduced from their recorded commit.
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
	production := flag.Bool("production", false, "measure the shipped B renderer without runtime patches")
	flag.Parse()
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(*out, 0755))
	if *production {
		must(runProduction(root, *out))
		return
	}
	font, err := assetsync.LoadFont("", "international")
	must(err)
	if font.Version != 1 {
		must(fmt.Errorf("historical experiment requires commit a1e0fad; use -production for current B"))
	}
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
