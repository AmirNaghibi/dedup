package main

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create duplicate files
	content1 := []byte("hello world, this is duplicate content")
	content2 := []byte("unique file content here")
	content3 := []byte("another unique file")

	// Duplicate pair
	os.WriteFile(filepath.Join(dir, "file1.txt"), content1, 0644)
	os.WriteFile(filepath.Join(dir, "file2.txt"), content1, 0644)

	// Unique files
	os.WriteFile(filepath.Join(dir, "unique1.txt"), content2, 0644)
	os.WriteFile(filepath.Join(dir, "unique2.txt"), content3, 0644)

	// Subdirectory with another duplicate
	sub := filepath.Join(dir, "subdir")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "file3.txt"), content1, 0644)

	return dir
}

func TestFindDuplicates(t *testing.T) {
	dir := setupTestDir(t)

	cfg := &Config{
		Dirs: []string{dir},
	}

	dupes, err := FindDuplicates(cfg)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	if len(dupes) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(dupes))
	}

	if len(dupes[0].Paths) != 3 {
		t.Fatalf("expected 3 files in duplicate group, got %d", len(dupes[0].Paths))
	}
}

func TestFindDuplicates_MinSize(t *testing.T) {
	dir := setupTestDir(t)

	cfg := &Config{
		Dirs:    []string{dir},
		MinSize: 1000, // All test files are smaller than this
	}

	dupes, err := FindDuplicates(cfg)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	if len(dupes) != 0 {
		t.Fatalf("expected 0 duplicate groups with min-size filter, got %d", len(dupes))
	}
}

func TestFindDuplicates_MaxSize(t *testing.T) {
	dir := setupTestDir(t)

	cfg := &Config{
		Dirs:    []string{dir},
		MaxSize: 5, // All test files are larger than this
	}

	dupes, err := FindDuplicates(cfg)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	if len(dupes) != 0 {
		t.Fatalf("expected 0 duplicate groups with max-size filter, got %d", len(dupes))
	}
}

func TestFindDuplicates_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Dirs: []string{dir},
	}

	dupes, err := FindDuplicates(cfg)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	if len(dupes) != 0 {
		t.Fatalf("expected 0 duplicate groups for empty dir, got %d", len(dupes))
	}
}

func TestFindDuplicates_NoDuplicates(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("ccc"), 0644)

	cfg := &Config{
		Dirs: []string{dir},
	}

	dupes, err := FindDuplicates(cfg)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	if len(dupes) != 0 {
		t.Fatalf("expected 0 duplicate groups, got %d", len(dupes))
	}
}

func TestFindDuplicates_MultipleDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	content := []byte("cross-directory duplicate")
	os.WriteFile(filepath.Join(dir1, "file.txt"), content, 0644)
	os.WriteFile(filepath.Join(dir2, "copy.txt"), content, 0644)

	cfg := &Config{
		Dirs: []string{dir1, dir2},
	}

	dupes, err := FindDuplicates(cfg)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	if len(dupes) != 1 {
		t.Fatalf("expected 1 duplicate group across dirs, got %d", len(dupes))
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("deterministic hash test"), 0644)

	hash1, err := computeHash(path)
	if err != nil {
		t.Fatal(err)
	}

	hash2, err := computeHash(path)
	if err != nil {
		t.Fatal(err)
	}

	if hash1 != hash2 {
		t.Fatalf("hash not deterministic: %s != %s", hash1, hash2)
	}
}

func TestComputeHash_DifferentContent(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "a.txt")
	path2 := filepath.Join(dir, "b.txt")
	os.WriteFile(path1, []byte("content A"), 0644)
	os.WriteFile(path2, []byte("content B"), 0644)

	hash1, _ := computeHash(path1)
	hash2, _ := computeHash(path2)

	if hash1 == hash2 {
		t.Fatal("different content should produce different hashes")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
