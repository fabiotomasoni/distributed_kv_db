// Package server owns the bounded process lifecycle. It adds no client protocol
// or recovery algorithm: StartAPI verifies storage and replays before listening.
package server

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/diagnostics"
	"github.com/fabiotomasoni/distributed_kv_db/internal/node"
	"path/filepath"
	"strings"
)

const ShutdownTimeout = 5 * time.Second

// Run returns 0 on a clean signal/context stop, 2 on usage/configuration failure,
// and 1 on startup, runtime or shutdown failure. Diagnostics never include user
// values, raw configuration or filesystem error contents. No implicit bootstrap.
func Run(ctx context.Context, args []string, out, errOut io.Writer) int {
	if ctx == nil {
		fmt.Fprintln(errOut, "invalid_context")
		return 2
	}
	if len(args) > 0 && args[0] == "node" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("quorumkv", flag.ContinueOnError)
	// Flag's default errors can echo arbitrary arguments. Emit fixed diagnostics.
	fs.SetOutput(io.Discard)
	path := fs.String("config", "", "path to static configuration JSON")
	logDir := fs.String("log-dir", "", "optional separate private log directory")
	bootstrap := fs.Bool("bootstrap", false, "initialize a pre-existing private empty data directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(out, "Usage: quorumkv node -config FILE [-bootstrap]")
			return 0
		}
		fmt.Fprintln(errOut, "invalid_arguments")
		return 2
	}
	if *path == "" || fs.NArg() != 0 {
		fmt.Fprintln(errOut, "config_required")
		return 2
	}
	c, err := readConfig(*path)
	if err != nil {
		fmt.Fprintln(errOut, "invalid_configuration")
		return 2
	}
	if ctx.Err() != nil {
		return 0
	}
	var rotation *diagnostics.Rotation
	if *logDir != "" {
		logAbs, e1 := filepath.EvalSymlinks(*logDir)
		dataAbs, e2 := filepath.EvalSymlinks(c.DataDir())
		if e1 == nil {
			logAbs, e1 = filepath.Abs(logAbs)
		}
		if e2 == nil {
			dataAbs, e2 = filepath.Abs(dataAbs)
		}
		if e1 != nil || e2 != nil || logAbs == dataAbs || within(dataAbs, logAbs) || within(logAbs, dataAbs) {
			fmt.Fprintln(errOut, "invalid_log_directory")
			return 2
		}
		rotation, err = diagnostics.OpenRotation(logAbs)
		if err != nil {
			fmt.Fprintln(errOut, "invalid_log_directory")
			return 2
		}
		defer rotation.Close()
	}
	n, err := node.StartAPI(c, *bootstrap)
	if err != nil {
		fmt.Fprintln(errOut, "startup_failed")
		return 1
	}
	if _, err = fmt.Fprintln(out, "node_started", c.NodeID()); err != nil {
		shutdown, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		_ = n.Shutdown(shutdown)
		return 1
	}
	d := n.Diagnostics()
	d.Log(diagnostics.Record{Event: diagnostics.Startup})
	flush := func() {
		for _, record := range d.Drain() {
			line := diagnostics.Encode(record)
			if rotation != nil {
				if _, e := rotation.Write(line); e != nil {
					fmt.Fprintln(errOut, "diagnostic_write_failed")
				}
			} else {
				_, _ = errOut.Write(line)
			}
			if record.Event == diagnostics.StorageError && rotation != nil {
				_, _ = errOut.Write(line)
			}
		}
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
waiting:
	for {
		select {
		case <-ctx.Done():
			break waiting
		case <-n.Done():
			break waiting
		case <-ticker.C:
			flush()
		}
	}
	defer func() { d.Log(diagnostics.Record{Event: diagnostics.Shutdown}); flush() }()
	shutdown, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	if err = n.Shutdown(shutdown); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintln(errOut, "shutdown_timeout_recovery_required")
		} else {
			fmt.Fprintln(errOut, "node_failed_recovery_required")
		}
		return 1
	}
	fmt.Fprintln(out, "node_stopped", c.NodeID())
	return 0
}
func readConfig(path string) (config.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return config.Config{}, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, config.MaxBytes+1))
	if err != nil {
		return config.Config{}, err
	}
	return config.Parse(raw)
}

func within(parent, child string) bool {
	rel, e := filepath.Rel(parent, child)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
