package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPlanBatchOutputs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		inputs   []string
		existing []string
		want     []string
	}{
		{"same basenames", []string{"a/report.md", "b/report.md"}, nil, []string{"report.pdf", "report (2).pdf"}},
		{"existing PDFs", []string{"a/report.md", "b/report.md"}, []string{"report.pdf", "report (2).pdf"}, []string{"report (3).pdf", "report (4).pdf"}},
		{"reserve natural names", []string{"a/report.md", "b/report.md", "c/report (2).md"}, nil, []string{"report.pdf", "report (3).pdf", "report (2).pdf"}},
		{"case and extensions", []string{"a/Report.md", "b/report.txt", "c/REPORT.markdown"}, []string{"REPORT.PDF"}, []string{"Report (2).pdf", "report (3).pdf", "REPORT (4).pdf"}},
		{"duplicate input", []string{"a/report.md", "a/report.md"}, nil, []string{"report.pdf", "report (2).pdf"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tc.existing {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			first, err := planBatchOutputs(tc.inputs, dir)
			if err != nil {
				t.Fatal(err)
			}
			second, err := planBatchOutputs(tc.inputs, dir)
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("planning is not deterministic: %v, %v", second, err)
			}
			for i, plan := range first {
				if got := filepath.Base(plan.Output); got != tc.want[i] {
					t.Errorf("output %d = %q, want %q", i, got, tc.want[i])
				}
				if want := filepath.Base(toPDFName(tc.inputs[i])) != tc.want[i]; plan.Renamed != want {
					t.Errorf("renamed %d = %v, want %v", i, plan.Renamed, want)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != len(tc.existing) {
				t.Fatalf("preflight changed directory: %v, %v", entries, err)
			}
		})
	}
}

func TestPlanBatchSiblingOutputsAndOccupiedDirectory(t *testing.T) {
	dir := t.TempDir()
	var inputs []string
	for _, folder := range []string{"a", "b"} {
		sourceDir := filepath.Join(dir, folder)
		if err := os.Mkdir(sourceDir, 0755); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, filepath.Join(sourceDir, "report.md"))
	}
	if err := os.Mkdir(filepath.Join(dir, "a", "report.pdf"), 0755); err != nil {
		t.Fatal(err)
	}
	plans, err := planBatchOutputs(inputs, "")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"a/report (2).pdf", "b/report.pdf"} {
		if plans[i].Output != filepath.Join(dir, filepath.FromSlash(want)) {
			t.Fatalf("output %d = %q, want %q", i, plans[i].Output, want)
		}
	}
}

func TestPlanBatchPreservesDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "report.pdf")
	if err := os.Symlink("missing-target", link); err != nil {
		t.Skip(err)
	}
	plans, err := planBatchOutputs([]string{"report.md"}, dir)
	if err != nil || filepath.Base(plans[0].Output) != "report (2).pdf" {
		t.Fatalf("plan = %v, error = %v", plans, err)
	}
	if got, err := os.Readlink(link); err != nil || got != "missing-target" {
		t.Fatalf("link changed: %q, %v", got, err)
	}
}

func TestPlanBatchResolvesDirectoryAliases(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip(err)
	}
	plans, err := planBatchOutputs([]string{filepath.Join(dir, "report.md"), filepath.Join(link, "report.md")}, "")
	if err != nil || filepath.Base(plans[1].Output) != "report (2).pdf" {
		t.Fatalf("alias plan = %v, error = %v", plans, err)
	}
}

func TestConvertBatchPreservesPDFsAndContinuesAfterFailure(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	existing := filepath.Join(out, "report.pdf")
	original := []byte("existing PDF sentinel")
	if err := os.WriteFile(existing, original, 0644); err != nil {
		t.Fatal(err)
	}
	var inputs []string
	for _, folder := range []string{"a", "missing", "b"} {
		sourceDir := filepath.Join(dir, folder)
		if err := os.Mkdir(sourceDir, 0755); err != nil {
			t.Fatal(err)
		}
		input := filepath.Join(sourceDir, "report.md")
		inputs = append(inputs, input)
		if folder != "missing" {
			if err := os.WriteFile(input, []byte("# Unique "+folder+" document"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	results, err := convertBatch(inputs, out, ConvertOptions{Mermaid: MermaidSkip}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var pdfs [][]byte
	for i, result := range results {
		if (result.Error != "") != (i == 1) {
			t.Fatalf("unexpected result %d: %+v", i, result)
		}
		if i == 1 {
			if _, err := os.Stat(result.Output); !os.IsNotExist(err) {
				t.Fatalf("failed conversion left output: %v", err)
			}
			continue
		}
		pdf, err := os.ReadFile(result.Output)
		if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
			t.Fatalf("invalid PDF: %v", err)
		}
		pdfs = append(pdfs, pdf)
	}
	if bytes.Equal(pdfs[0], pdfs[1]) {
		t.Fatal("different input documents produced the same output")
	}
	got, err := os.ReadFile(existing)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("existing PDF changed: %q, %v", got, err)
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 3 {
		t.Fatalf("unexpected output files: %v, %v", entries, err)
	}
	// Repeating the same batch also preserves the previous successful PDFs.
	again, err := planBatchOutputs(inputs, out)
	if err != nil || again[0].Output == results[0].Output || again[2].Output == results[2].Output {
		t.Fatalf("repeat reused a completed output: %v, %v", again, err)
	}
}

func TestConvertBatchPreflightFailureDoesNotStartConversion(t *testing.T) {
	_, err := convertBatch([]string{"input.md"}, filepath.Join(t.TempDir(), "missing"), ConvertOptions{}, func(int, batchOutput) {
		t.Fatal("conversion started before preflight succeeded")
	})
	if err == nil {
		t.Fatal("expected preflight failure")
	}
}

func TestConvertBatchRejectsDestinationCreatedAfterPreflight(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.md")
	if err := os.WriteFile(input, []byte("# Source"), 0644); err != nil {
		t.Fatal(err)
	}
	results, err := convertBatch([]string{input}, "", ConvertOptions{Mermaid: MermaidSkip}, func(_ int, plan batchOutput) {
		if err := os.WriteFile(plan.Output, []byte("another writer"), 0644); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil || results[0].Error == "" {
		t.Fatalf("expected raced destination error: %v, %v", results, err)
	}
	got, err := os.ReadFile(results[0].Output)
	if err != nil || string(got) != "another writer" {
		t.Fatalf("raced destination changed: %q, %v", got, err)
	}
}

func TestConvertBatchInvalidSiblingDirectoryDoesNotBlockValidInput(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.md")
	if err := os.WriteFile(good, []byte("# Good source"), 0644); err != nil {
		t.Fatal(err)
	}
	results, err := convertBatch([]string{good, filepath.Join(dir, "missing", "report.md")}, "", ConvertOptions{Mermaid: MermaidSkip}, nil)
	if err != nil || results[0].Error != "" || results[1].Error == "" {
		t.Fatalf("unexpected results: %v, %v", results, err)
	}
	got, err := os.ReadFile(results[0].Output)
	if err != nil || !bytes.HasPrefix(got, []byte("%PDF-")) {
		t.Fatalf("valid input was not converted: %v", err)
	}
}
