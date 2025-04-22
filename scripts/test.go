package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	rootDir := "." // Start from the current directory
	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Prevent panic if a directory is not accessible
			log.Printf("Warning: Error accessing path %q: %v\n", path, err)
			return err
		}

		// Check if the entry is a go.mod file
		if !d.IsDir() && d.Name() == "go.mod" {
			modDir := filepath.Dir(path)
			fmt.Printf("--> Found go.mod in: %s\n", modDir)

			// Skip the root go.mod if it exists and we only want submodules,
			// or adjust logic if root tests are also desired.
			// For this example, we'll run tests in all directories with go.mod.

			fmt.Printf("--> Running tests in: %s\n", modDir)
			cmd := exec.Command("go", "test", "./...")
			cmd.Dir = modDir // Set the working directory for the command
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			err := cmd.Run()
			if err != nil {
				// Log the error but continue checking other modules
				log.Printf("Error running tests in %s: %v\n", modDir, err)
				// Optionally, return the error here if you want to stop the walk on the first failure
				// return fmt.Errorf("tests failed in %s: %w", modDir, err)
			} else {
				fmt.Printf("--> Tests finished successfully in: %s\n", modDir)
			}
			fmt.Println(strings.Repeat("-", 40)) // Separator
		}
		return nil // Continue walking
	})

	if err != nil {
		log.Fatalf("Error walking the path %q: %v\n", rootDir, err)
	}

	fmt.Println("--> Finished running tests for all modules.")
}
