package okf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"okf/worker/internal/converter"
)

func TestBuildProducesValidBundle(t *testing.T) {
	workDir := t.TempDir()
	segments := []converter.Segment{
		{Title: "Inicio", Order: 1, File: filepath.Join(workDir, "capitulo-01.md")},
		{Title: "Intro", Order: 2, File: filepath.Join(workDir, "capitulo-02.md")},
	}
	if err := os.WriteFile(segments[0].File, []byte("# Inicio\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(segments[1].File, []byte("# Intro\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(workDir, "original.pdf")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Build(context.Background(), workDir, segments, Meta{
		JobID:        "job-1",
		UserID:       "user-1",
		OriginalName: "doc.pdf",
		Format:       "pdf",
		SourcePath:   src,
		ConvertedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if res.BundlePath != filepath.Join("bundles", "user-1", "job-1") {
		t.Errorf("BundlePath = %q", res.BundlePath)
	}
	if err := Validate(res.Dir); err != nil {
		t.Fatalf("bundle no válido tras Build: %v", err)
	}

	index, err := os.ReadFile(filepath.Join(res.Dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "conceptos/capitulo-01.md") {
		t.Errorf("index.md sin link al primer concepto: %s", index)
	}

	logFile, err := os.ReadFile(filepath.Join(res.Dir, "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logFile), "SHA-256") {
		t.Errorf("log.md sin metadatos: %s", logFile)
	}
}