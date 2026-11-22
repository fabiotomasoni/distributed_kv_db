package raft

import (
	"errors"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func TestMetricsDistinguishStorageFailureFromApplicationFailure(t *testing.T) {
	for _, storage := range []bool{false, true} {
		name := "application"
		if storage {
			name = "storage"
		}
		t.Run(name, func(t *testing.T) {
			disk := &memoryStorage{s: initial()}
			s := newAppService(t, disk, statemachine.New(), 1)
			if storage {
				disk.failure = wal.ErrFailed
				effects, err := s.core.Tick(s.core.Status().ElectionDeadline)
				if _, err = s.effects(effects, err); !errors.Is(err, wal.ErrFailed) {
					t.Fatal(err)
				}
			} else {
				_, _ = s.core.fail(applier.ErrApplyFailed)
				s.publish()
			}
			metrics := s.Metrics()
			observed, ok := metrics["storage_error"].(bool)
			if metrics["node_error"] != true {
				t.Fatal("node failure hidden")
			}
			shutdownErr := s.Shutdown(deadline(t))
			if shutdownErr == nil {
				t.Fatal("fatal failure lost")
			}
			if !ok || observed != storage {
				t.Fatalf("storage_error=%v for %s failure", observed, name)
			}
		})
	}
}
