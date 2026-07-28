package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

var version = "0.1.0"

func main() {
	var (
		minSize    int64
		maxSize    int64
		showSize   bool
		jsonOutput bool
		delete     bool
		dryRun     bool
		workers    int
		showVer    bool
	)

	flag.Int64Var(&minSize, "min-size", 0, "minimum file size in bytes to consider")
	flag.Int64Var(&maxSize, "max-size", 0, "maximum file size in bytes (0 = no limit)")
	flag.BoolVar(&showSize, "show-size", false, "show file sizes in output")
	flag.BoolVar(&jsonOutput, "json", false, "output results as JSON")
	flag.BoolVar(&delete, "delete", false, "interactively delete duplicates")
	flag.BoolVar(&dryRun, "dry-run", false, "show what would be deleted without deleting")
	flag.IntVar(&workers, "workers", 0, "number of parallel hash workers (0 = auto)")
	flag.BoolVar(&showVer, "version", false, "print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "dedup - find duplicate files fast\n\n")
		fmt.Fprintf(os.Stderr, "Usage: dedup [options] <directory> [directory...]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  dedup ~/Photos\n")
		fmt.Fprintf(os.Stderr, "  dedup --min-size 1048576 --json /data\n")
		fmt.Fprintf(os.Stderr, "  dedup --delete --dry-run ~/Downloads\n")
	}

	flag.Parse()

	if showVer {
		fmt.Printf("dedup v%s\n", version)
		os.Exit(0)
	}

	dirs := flag.Args()
	if len(dirs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one directory is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate directories
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot access %q: %v\n", dir, err)
			os.Exit(1)
		}
		if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "Error: %q is not a directory\n", dir)
			os.Exit(1)
		}
	}

	cfg := &Config{
		Dirs:       dirs,
		MinSize:    minSize,
		MaxSize:    maxSize,
		ShowSize:   showSize,
		JSONOutput: jsonOutput,
		Delete:     delete,
		DryRun:     dryRun,
		Workers:    workers,
	}

	dupes, err := FindDuplicates(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(dupes) == 0 {
		if !jsonOutput {
			fmt.Println("No duplicates found.")
		} else {
			fmt.Println("[]")
		}
		os.Exit(0)
	}

	if jsonOutput {
		printJSON(dupes)
	} else {
		printHuman(dupes, showSize)
	}

	if delete || dryRun {
		handleDeletion(dupes, dryRun)
	}
}

func printHuman(groups []DuplicateGroup, showSize bool) {
	var totalWasted int64
	for i, g := range groups {
		if i > 0 {
			fmt.Println()
		}
		sizeStr := ""
		if showSize {
			sizeStr = fmt.Sprintf(" (%s)", formatBytes(g.Size))
		}
		fmt.Printf("Group %d%s:\n", i+1, sizeStr)
		for _, f := range g.Paths {
			fmt.Printf("  %s\n", f)
		}
		totalWasted += g.Size * int64(len(g.Paths)-1)
	}
	fmt.Printf("\n%d duplicate groups found. %s wasted.\n",
		len(groups), formatBytes(totalWasted))
}

func printJSON(groups []DuplicateGroup) {
	fmt.Println("[")
	for i, g := range groups {
		fmt.Printf("  {\"hash\": %q, \"size\": %d, \"paths\": [", g.Hash, g.Size)
		for j, p := range g.Paths {
			fmt.Printf("%q", p)
			if j < len(g.Paths)-1 {
				fmt.Print(", ")
			}
		}
		fmt.Print("]}")
		if i < len(groups)-1 {
			fmt.Println(",")
		} else {
			fmt.Println()
		}
	}
	fmt.Println("]")
}

func handleDeletion(groups []DuplicateGroup, dryRun bool) {
	var totalFreed int64
	var filesDeleted int

	for _, g := range groups {
		// Keep the first file, mark rest for deletion
		for _, path := range g.Paths[1:] {
			if dryRun {
				fmt.Printf("[dry-run] would delete: %s\n", path)
			} else {
				if err := os.Remove(path); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not delete %s: %v\n", path, err)
					continue
				}
				fmt.Printf("Deleted: %s\n", path)
			}
			filesDeleted++
			totalFreed += g.Size
		}
	}

	verb := "Deleted"
	if dryRun {
		verb = "Would delete"
	}
	fmt.Printf("\n%s %d files, freeing %s\n", verb, filesDeleted, formatBytes(totalFreed))
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s",
		float64(b)/float64(div),
		[]string{"KB", "MB", "GB", "TB"}[exp])
}

func init() {
	// Ensure we handle pipe broken gracefully
	_ = strings.NewReplacer()
}
