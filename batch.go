package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type batchOutput struct {
	Input   string `json:"input"`
	Output  string `json:"output"`
	Renamed bool   `json:"renamed"`
	Error   string `json:"error,omitempty"`
}

func toPDFName(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name)) + ".pdf"
}

// planBatchOutputs allocates every name before conversion. Queue order breaks
// ties; natural names are reserved before numbered names so report (2).md keeps
// its name. Existing entries (including directories and dangling links) are
// occupied. Name comparisons are deliberately case-insensitive on every OS.
func planBatchOutputs(paths []string, outputDir string) ([]batchOutput, error) {
	type directoryNames struct {
		occupied map[string]bool
		wanted   map[string]bool
	}
	directories := make(map[string]*directoryNames)
	plans := make([]batchOutput, len(paths))
	planDirs := make([]*directoryNames, len(paths))
	for i, input := range paths {
		output := toPDFName(input)
		if outputDir != "" {
			output = filepath.Join(outputDir, filepath.Base(output))
		}
		plans[i] = batchOutput{Input: input, Output: output}
		names, err := func() (*directoryNames, error) {
			dir, err := filepath.Abs(filepath.Dir(output))
			if err != nil {
				return nil, fmt.Errorf("resolving output directory: %w", err)
			}
			dir, err = filepath.EvalSymlinks(dir)
			if err != nil {
				return nil, fmt.Errorf("checking output directory: %w", err)
			}
			key := dir
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			names := directories[key]
			if names == nil {
				entries, err := os.ReadDir(dir)
				if err != nil {
					return nil, fmt.Errorf("reading output directory: %w", err)
				}
				names = &directoryNames{occupied: make(map[string]bool), wanted: make(map[string]bool)}
				for _, entry := range entries {
					names.occupied[strings.ToLower(entry.Name())] = true
				}
				directories[key] = names
			}
			return names, nil
		}()
		if err != nil {
			if outputDir != "" {
				return nil, err
			}
			plans[i].Error = err.Error()
			continue
		}
		names.wanted[strings.ToLower(filepath.Base(output))] = true
		planDirs[i] = names
	}
	for i := range plans {
		plan, names := &plans[i], planDirs[i]
		if plan.Error != "" {
			continue
		}
		name := filepath.Base(plan.Output)
		if names.occupied[strings.ToLower(name)] {
			stem := strings.TrimSuffix(name, filepath.Ext(name))
			for suffix := 2; ; suffix++ {
				candidate := fmt.Sprintf("%s (%d).pdf", stem, suffix)
				key := strings.ToLower(candidate)
				if !names.occupied[key] && !names.wanted[key] {
					name = candidate
					plan.Renamed = true
					break
				}
			}
		}
		names.occupied[strings.ToLower(name)] = true
		plan.Output = filepath.Join(filepath.Dir(plan.Output), name)
	}
	return plans, nil
}

// convertBatch never replaces an existing destination, even if another process
// creates it after preflight. Such a race is reported for that file; the rest of
// the batch continues. A failed conversion does not change later allocations.
func convertBatch(paths []string, outputDir string, opts ConvertOptions, progress func(int, batchOutput)) ([]batchOutput, error) {
	plans, err := planBatchOutputs(paths, outputDir)
	if err != nil {
		return nil, err
	}
	for i := range plans {
		if plans[i].Error != "" {
			continue
		}
		if progress != nil {
			progress(i, plans[i])
		}
		if err := convertFile(plans[i].Input, plans[i].Output, opts, false); err != nil {
			plans[i].Error = err.Error()
		}
	}
	return plans, nil
}
