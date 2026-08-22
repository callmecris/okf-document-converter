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
		{Title: "Inicio", Order: 1, File: filepath.Join(workDir, "fragmento-01.md")},
		{Title: "Intro", Order: 2, File: filepath.Join(workDir, "fragmento-02.md")},
	}
	if err := os.WriteFile(segments[0].File, []byte("# Inicio\n\nContenido del primer concepto.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(segments[1].File, []byte("# Intro\n\nContenido del segundo concepto.\n"), 0o644); err != nil {
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
	report := Validate(res.Dir)
	if !report.Publishable() {
		t.Fatalf("bundle no válido tras Build: %v", report.Err)
	}
	if report.Level != LevelValid {
		t.Errorf("nivel = %q, advertencias = %v; se esperaba valid", report.Level, report.Warnings)
	}

	index, err := os.ReadFile(filepath.Join(res.Dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "conceptos/fragmento-01.md") {
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