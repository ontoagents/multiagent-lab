//go:build !windows

package oxigraph

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/runtime-manager/internal/engine"
)

func TestDockerProcessStopReportsFailure(t *testing.T) {
	for _, companion := range []bool{true, false} {
		t.Run(map[bool]string{true: "companion", false: "regular"}[companion], func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "docker")
			// Startup succeeds; the external CLI reports a daemon failure on stop.
			if err := os.WriteFile(bin, []byte("#!/bin/sh\nif [ \"$1\" = rm ]; then echo 'daemon unavailable' >&2; exit 7; fi\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("DOCKER_BIN", bin)
			r := &Runtime{DataDir: dir}
			proc, err := r.DockerStartOpts(context.Background(), "p1", 9201, nil, engine.GraphIsolationOpts{CompanionMode: companion})
			if err != nil {
				t.Fatal(err)
			}
			if err := proc.Stop(); err == nil || !strings.Contains(err.Error(), "daemon unavailable") {
				t.Fatalf("docker removal failure must reach Manager: %v", err)
			}
		})
	}
}
