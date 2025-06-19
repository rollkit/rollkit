//go:build cover
// +build cover

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	rootDir := "."

	var coverFiles []string
	var testFailures bool

	// Phase 1: Find go.mod files, run tests, and collect cover.out file paths
	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("Warning: Error accessing path %q: %v\n", path, err)
			return err // Skip this path
		}

		if d.IsDir() && (d.Name() == "vendor" || d.Name() == ".git" || strings.Contains(path, "testdata")) {
			log.Printf("Skipping directory: %s\n", path)
			return filepath.SkipDir
		}

		if !d.IsDir() && d.Name() == "go.mod" {
			modDir := filepath.Dir(path)
			fmt.Printf("--> Found go.mod in: %s\n", modDir)

			fullCoverProfilePath := filepath.Join(modDir, "cover.out")
			relativeCoverProfileArg := "cover.out"

			fmt.Printf("--> Running tests with coverage in: %s (profile: %s)\n", modDir, relativeCoverProfileArg)
			cmd := exec.Command("go", "test", "./...", "-race", "-coverprofile="+relativeCoverProfileArg, "-covermode=atomic")
			cmd.Dir = modDir
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			runErr := cmd.Run()
			if runErr != nil {
				log.Printf("Error running tests in %s: %v\n", modDir, runErr)
				testFailures = true
			} else {
				fmt.Printf("--> Tests finished successfully in: %s\n", modDir)
				// Check for the existence of the cover file using its full path
				if _, statErr := os.Stat(fullCoverProfilePath); statErr == nil {
					coverFiles = append(coverFiles, fullCoverProfilePath)
				} else {
					log.Printf("Warning: %s not found in %s after tests ran. This module might not have any tests or testable code.\n", relativeCoverProfileArg, modDir)
				}
			}
			fmt.Println(strings.Repeat("-", 40))
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Error walking the path %q: %v\n", rootDir, err)
	}

	// Phase 2: Merge coverage files
	if len(coverFiles) > 0 {
		mergedCoverageFile := filepath.Join(rootDir, "coverage.txt") // Place in root directory
		fmt.Printf("--> Merging %d coverage files into %s\n", len(coverFiles), mergedCoverageFile)

		out, err := os.Create(mergedCoverageFile)
		if err != nil {
			log.Fatalf("Failed to create merged coverage file %s: %v\n", mergedCoverageFile, err)
		}
		defer out.Close()

		firstFile := true
		for _, coverFile := range coverFiles {
			fmt.Printf("    Processing %s\n", coverFile)
			f, err := os.Open(coverFile)
			if err != nil {
				log.Printf("Warning: Failed to open coverage file %s: %v. Skipping.\n", coverFile, err)
				continue
			}

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "mode: ") {
					if firstFile {
						if _, err := out.WriteString(line + "\n"); err != nil {
							log.Fatalf("Failed to write mode line to merged coverage file: %v", err)
						}
						firstFile = false
					}
				} else {
					if _, err := out.WriteString(line + "\n"); err != nil {
						log.Fatalf("Failed to write to merged coverage file: %v", err)
					}
				}
			}
			if err := scanner.Err(); err != nil {
				log.Printf("Warning: Error reading coverage file %s: %v\n", coverFile, err)
			}
			f.Close()

			// Phase 3: Clean up individual cover.out files
			fmt.Printf("    Deleting %s\n", coverFile)
			if err := os.Remove(coverFile); err != nil {
				log.Printf("Warning: Failed to delete coverage file %s: %v\n", coverFile, err)
			}
		}
		fmt.Printf("--> Merged coverage data written to %s\n", mergedCoverageFile)
	} else {
		fmt.Println("--> No coverage files found to merge. This could be because no tests were found or all tests failed before producing coverage.")
	}

	fmt.Println("--> Finished processing coverage for all modules.")
	if testFailures {
		fmt.Println("Error: Some tests failed. Please check the logs above.")
		os.Exit(1)
	}
}
