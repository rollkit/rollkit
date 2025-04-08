package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	// Find the project root directory
	projectRoot, err := findProjectRoot()
	if err != nil {
		log.Fatalf("Failed to find project root: %v", err)
	}

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Start local-da in a goroutine
	daPath := filepath.Join(projectRoot, "build", "local-da")
	if _, err := os.Stat(daPath); os.IsNotExist(err) {
		log.Println("local-da binary not found, building...")
		buildCmd := exec.Command("make", "build-da")
		buildCmd.Dir = projectRoot
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			log.Fatalf("Failed to build local-da: %v", err)
		}
	}

	log.Println("Starting local-da...")
	daCmd := exec.CommandContext(ctx, daPath)
	daCmd.Stdout = os.Stdout
	daCmd.Stderr = os.Stderr
	if err := daCmd.Start(); err != nil {
		log.Fatalf("Failed to start local-da: %v", err)
	}

	// Ensure DA is properly initialized before starting testapp
	log.Println("Waiting for local-da to initialize...")
	time.Sleep(2 * time.Second)

	// Start testapp with proper configuration
	appPath := filepath.Join(projectRoot, "build", "testapp")
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		log.Println("testapp binary not found, building...")
		buildCmd := exec.Command("make", "build")
		buildCmd.Dir = projectRoot
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			log.Fatalf("Failed to build testapp: %v", err)
		}
	}

	log.Println("Initializing testapp with local DA...")
	initArgs := []string{
		"init",
		"--da.type=local",
		"--da.config.url=http://localhost:7980",
		"--rollkit.node.aggregator=true",       // Set to true if you want to run as an aggregator
		"--rollkit.signer.passphrase=12345678", // Replace with your passphrase
		// Add any other required flags for initialization
	}
	initCmd := exec.CommandContext(ctx, appPath, initArgs...)
	initCmd.Stdout = os.Stdout
	initCmd.Stderr = os.Stderr
	if err := initCmd.Run(); err != nil {
		log.Fatalf("Failed to initialize testapp: %v", err)
	}

	log.Println("Starting testapp with local DA...")
	appArgs := []string{
		"run",
		"--da.type=local",
		"--da.config.url=http://localhost:7980",
		"--rollkit.node.aggregator=true",       // Set to true if you want to run as an aggregator
		"--rollkit.signer.passphrase=12345678", // Replace with your passphrase
		// Add any other required flags for testapp
	}

	appCmd := exec.CommandContext(ctx, appPath, appArgs...)
	appCmd.Stdout = os.Stdout
	appCmd.Stderr = os.Stderr
	if err := appCmd.Start(); err != nil {
		log.Fatalf("Failed to start testapp: %v", err)
	}

	// Wait for either process to finish or context cancellation
	errCh := make(chan error, 2)
	go func() {
		errCh <- daCmd.Wait()
	}()
	go func() {
		errCh <- appCmd.Wait()
	}()

	select {
	case err := <-errCh:
		if ctx.Err() == nil { // If context was not canceled, it's an unexpected error
			log.Fatalf("One of the processes exited unexpectedly: %v", err)
		}
	case <-ctx.Done():
		// Context was canceled, we're shutting down gracefully
		log.Println("Shutting down processes...")

		// Give processes some time to gracefully terminate
		time.Sleep(1 * time.Second)
	}

	log.Println("Cleanup complete")
}

// findProjectRoot attempts to locate the project root directory
func findProjectRoot() (string, error) {
	// Try current working directory first
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if we're already in the project root
	if isProjectRoot(cwd) {
		return cwd, nil
	}

	// Try to find it by walking up from the current directory
	dir := cwd
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			// We've reached the filesystem root
			break
		}
		dir = parent
		if isProjectRoot(dir) {
			return dir, nil
		}
	}

	return "", fmt.Errorf("could not find project root")
}

// isProjectRoot checks if the given directory is the project root
func isProjectRoot(dir string) bool {
	// Check for key directories/files that would indicate the project root
	markers := []string{
		"da",
		"rollups",
		"scripts",
		"go.mod",
	}

	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(dir, marker)); os.IsNotExist(err) {
			return false
		}
	}
	return true
}
