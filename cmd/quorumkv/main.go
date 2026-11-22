// QuorumKV runs one member of the static, loopback-only three-node MVP.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/fabiotomasoni/distributed_kv_db/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := server.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
