package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/cespare/xxhash/v2"
)

// Config holds all scan configuration.
type Config struct {
	Dirs       []string
	MinSize    int64
	MaxSize    int64
	ShowSize   bool
	JSONOutput bool
	Delete     bool
	DryRun     bool
	Workers    int
}

// DuplicateGroup represents a set of files with identical content.
type DuplicateGroup struct {
	Hash  string   `json:"hash"`
	Size  int64    `json:"size"`
	Paths []string `json:"paths"`
}

// fileEntry is an internal representation of a discovered file.
type fileEntry struct {
	Path string
	Size int64
}

// hashResult is returned by hash workers.
type hashResult struct {
	Path string
	Hash string
	Err  error
}

// FindDuplicates scans directories and returns groups of duplicate files.
func FindDuplicates(cfg *Config) ([]DuplicateGroup, error) {
	// Phase 1: Walk and collect files, group by size
	sizeGroups, err := collectFiles(cfg)
	if err != nil {
		return nil, err
	}

	// Phase 2: For size groups with >1 file, compute content hashes
	var candidates []fileEntry
	for _, files := range sizeGroups {
		if len(files) > 1 {
			candidates = append(candidates, files...)
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Phase 3: Hash files in parallel
	workers := cfg.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
		if workers > 8 {
			workers = 8
		}
	}

	hashGroups := hashFiles(candidates, workers)

	// Phase 4: Build duplicate groups (only groups with 2+ files)
	var results []DuplicateGroup
	for hash, files := range hashGroups {
		if len(files) < 2 {
			continue
		}
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.Path
		}
		sort.Strings(paths)
		results = append(results, DuplicateGroup{
			Hash:  hash,
			Size:  files[0].Size,
			Paths: paths,
		})
	}

	// Sort by wasted space (largest first)
	sort.Slice(results, func(i, j int) bool {
		wastedI := results[i].Size * int64(len(results[i].Paths)-1)
		wastedJ := results[j].Size * int64(len(results[j].Paths)-1)
		return wastedI > wastedJ
	})

	return results, nil
}

// collectFiles walks all directories and groups files by size.
func collectFiles(cfg *Config) (map[int64][]fileEntry, error) {
	sizeGroups := make(map[int64][]fileEntry)

	for _, dir := range cfg.Dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// Skip permission errors
				return nil
			}
			if info.IsDir() {
				return nil
			}
			// Skip symlinks
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			// Skip empty files
			if info.Size() == 0 {
				return nil
			}
			// Apply size filters
			if cfg.MinSize > 0 && info.Size() < cfg.MinSize {
				return nil
			}
			if cfg.MaxSize > 0 && info.Size() > cfg.MaxSize {
				return nil
			}

			sizeGroups[info.Size()] = append(sizeGroups[info.Size()], fileEntry{
				Path: path,
				Size: info.Size(),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return sizeGroups, nil
}

// hashFiles computes content hashes for all candidate files using a worker pool.
// Uses xxhash for fast initial hashing, then SHA-256 to confirm matches.
func hashFiles(files []fileEntry, numWorkers int) map[string][]fileEntry {
	jobs := make(chan fileEntry, len(files))
	results := make(chan hashResult, len(files))

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				hash, err := computeHash(f.Path)
				results <- hashResult{Path: f.Path, Hash: hash, Err: err}
			}
		}()
	}

	for _, f := range files {
		jobs <- f
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results grouped by hash
	hashMap := make(map[string][]fileEntry)
	sizeMap := make(map[string]int64)
	for _, f := range files {
		sizeMap[f.Path] = f.Size
	}

	for r := range results {
		if r.Err != nil {
			continue // Skip files we can't read
		}
		hashMap[r.Hash] = append(hashMap[r.Hash], fileEntry{
			Path: r.Path,
			Size: sizeMap[r.Path],
		})
	}

	return hashMap
}

// computeHash produces a combined xxhash+sha256 fingerprint for a file.
// xxhash is used for the first pass (fast), SHA-256 confirms the match.
func computeHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	xxh := xxhash.New()
	sha := sha256.New()
	w := io.MultiWriter(xxh, sha)

	buf := make([]byte, 64*1024) // 64KB buffer
	if _, err := io.CopyBuffer(w, f, buf); err != nil {
		return "", err
	}

	// Use SHA-256 as the canonical hash (xxhash is computed but SHA is the key)
	return hex.EncodeToString(sha.Sum(nil)), nil
}
