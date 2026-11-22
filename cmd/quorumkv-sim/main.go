// quorumkv-sim is a separate, explicit synthetic fault profile. No sockets,
// active data directories or ordinary node fault switches are used.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/fabiotomasoni/distributed_kv_db/internal/faultsim"
	"io"
	"os"
	"time"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("quorumkv-sim", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	scenario := flags.String("scenario", "", "synthetic scenario JSON")
	replay := flags.String("replay", "", "saved manifest JSON")
	output := flags.String("output", "", "new manifest path")
	if flags.Parse(args) != nil || flags.NArg() != 0 || (*scenario == "") == (*replay == "") || (*scenario != "" && *output == "") {
		fmt.Fprintln(errOut, "invalid_simulation_arguments")
		return 2
	}
	path := *scenario
	if *replay != "" {
		path = *replay
	}
	f, e := os.Open(path)
	if e != nil {
		fmt.Fprintln(errOut, "input_failed")
		return 2
	}
	b, e := io.ReadAll(io.LimitReader(f, faultsim.MaxEvidence+1))
	_ = f.Close()
	if e != nil || len(b) > faultsim.MaxEvidence {
		fmt.Fprintln(errOut, "input_limit")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var m faultsim.Manifest
	if *replay != "" {
		original, err := faultsim.ReadManifest(b)
		if err != nil {
			fmt.Fprintln(errOut, "invalid_manifest")
			return 2
		}
		m, e = faultsim.Replay(ctx, original)
	} else {
		s, err := faultsim.Parse(b)
		if err != nil {
			fmt.Fprintln(errOut, "invalid_scenario")
			return 2
		}
		m, e = faultsim.Run(ctx, s)
	}
	if *output != "" {
		raw, err := m.Bytes()
		if err != nil {
			fmt.Fprintln(errOut, "evidence_limit")
			return 1
		}
		f, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintln(errOut, "output_failed")
			return 1
		}
		n, err := f.Write(raw)
		closeErr := f.Close()
		if err != nil || closeErr != nil || n != len(raw) {
			fmt.Fprintln(errOut, "output_failed")
			return 1
		}
	}
	if e != nil {
		fmt.Fprintln(errOut, "simulation_"+m.Outcome)
		return 1
	}
	fmt.Fprintln(out, "simulation_passed", len(m.Trace), "events")
	return 0
}
