//go:build run
// +build run

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	baseP2PPort = 26656
	baseRPCPort = 7331
)

func main() {
	// Parse command line arguments
	numNodes := flag.Int("nodes", 1, "Number of nodes to spin up")
	flag.Parse()

	if *numNodes < 1 {
		log.Fatal("Number of nodes must be at least 1")
	}

	// Setup variables to track resources for cleanup
	var (
		daCmd        *exec.Cmd
		nodeCommands = make([]*exec.Cmd, 0, *numNodes)
		nodeHomes    = make([]string, 0, *numNodes)
		ctx, cancel  = context.WithCancel(context.Background())
	)

	// Ensure cleanup happens even on early failure
	defer func() {
		// Cancel context to stop any running processes
		cancel()

		// Give processes some time to gracefully terminate
		time.Sleep(1 * time.Second)

		// More robust process termination
		killProcess := func(cmd *exec.Cmd) {
			if cmd == nil || cmd.Process == nil {
				return
			}

			// First try SIGTERM for graceful shutdown
			log.Printf("Sending SIGTERM to process %d", cmd.Process.Pid)
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				log.Printf("Failed to send SIGTERM to process %d: %v", cmd.Process.Pid, err)
			}

			// Give it a moment to terminate gracefully
			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			select {
			case <-done:
				// Process terminated gracefully
				return
			case <-time.After(2 * time.Second):
				// Process didn't terminate, force kill
				log.Printf("Process %d didn't respond to SIGTERM, sending SIGKILL", cmd.Process.Pid)
				if err := cmd.Process.Kill(); err != nil {
					log.Printf("Failed to kill process %d: %v", cmd.Process.Pid, err)
				}
			}
		}

		// Kill DA process
		if daCmd != nil {
			killProcess(daCmd)
		}

		// Kill all node processes
		for _, cmd := range nodeCommands {
			killProcess(cmd)
		}

		// Remove node home directories
		for _, nodeHome := range nodeHomes {
			if nodeHome != "" {
				if err := os.RemoveAll(nodeHome); err != nil {
					log.Printf("Failed to remove node home directory %s: %v", nodeHome, err)
				} else {
					log.Printf("Removed node home directory %s", nodeHome)
				}
			}
		}

		log.Println("Cleanup complete")
	}()

	// Find the project root directory
	projectRoot, err := findProjectRoot()
	if err != nil {
		log.Printf("Error finding project root: %v", err)
		return
	}

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
			log.Printf("Error building local-da: %v", err)
			return
		}
	}

	log.Println("Starting local-da...")
	daCmd = exec.CommandContext(ctx, daPath)
	daCmd.Stdout = os.Stdout
	daCmd.Stderr = os.Stderr
	if err := daCmd.Start(); err != nil {
		log.Printf("Error starting local-da: %v", err)
		return
	}

	// Ensure DA is properly initialized before starting nodes
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
			log.Printf("Error building testapp: %v", err)
			return
		}
	}

	// Start multiple nodes
	var aggregatorAddress string

	for i := 0; i < *numNodes; i++ {
		nodeHome := fmt.Sprintf("./testapp_home_node%d", i)
		// Track the node home for cleanup
		nodeHomes = append(nodeHomes, nodeHome)
		p2pPort := baseP2PPort + i
		rpcPort := baseRPCPort + i
		isAggregator := i == 0 // First node is the aggregator

		// Initialize this node
		log.Printf("Initializing node %d (aggregator: %v)...", i, isAggregator)

		// Create init command with unique home directory
		initArgs := []string{
			"init",
			fmt.Sprintf("--home=%s", nodeHome),
			fmt.Sprintf("--rollkit.node.aggregator=%t", isAggregator),
			"--rollkit.signer.passphrase=12345678",
			fmt.Sprintf("--rollkit.rpc.address=0.0.0.0:%d", rpcPort),
		}

		initCmd := exec.CommandContext(ctx, appPath, initArgs...)
		initCmd.Stdout = os.Stdout
		initCmd.Stderr = os.Stderr
		if err := initCmd.Run(); err != nil {
			log.Printf("Error initializing node %d: %v", i, err)
			return
		}

		// If this is not the first node, copy the genesis file from the first node
		if i > 0 {
			log.Printf("Copying genesis file from node 0 to node %d...", i)
			sourceGenesisPath := filepath.Join("./testapp_home_node0", "config", "genesis.json")
			destGenesisPath := filepath.Join(nodeHome, "config", "genesis.json")

			sourceFile, err := os.Open(sourceGenesisPath)
			if err != nil {
				log.Printf("Error opening source genesis file (%s) for node %d: %v", sourceGenesisPath, i, err)
				return
			}
			defer sourceFile.Close()

			destFile, err := os.Create(destGenesisPath) // Create or truncate
			if err != nil {
				log.Printf("Error creating destination genesis file (%s) for node %d: %v", destGenesisPath, i, err)
				return
			}
			defer destFile.Close()

			_, err = io.Copy(destFile, sourceFile)
			if err != nil {
				log.Printf("Error copying genesis file for node %d: %v", i, err)
				return
			}
			log.Printf("Successfully copied genesis file to node %d", i)
		}

		// Build run arguments
		runArgs := []string{
			"run",
			fmt.Sprintf("--home=%s", nodeHome),
			"--rollkit.da.address=http://localhost:7980",
			fmt.Sprintf("--rollkit.node.aggregator=%t", isAggregator),
			"--rollkit.signer.passphrase=12345678",
			fmt.Sprintf("--rollkit.p2p.listen_address=/ip4/0.0.0.0/tcp/%d", p2pPort),
			fmt.Sprintf("--rollkit.rpc.address=0.0.0.0:%d", rpcPort),
		}

		// Add peer list for non-aggregator nodes
		if i > 0 && aggregatorAddress != "" {
			runArgs = append(runArgs, fmt.Sprintf("--rollkit.p2p.peers=%s", aggregatorAddress))
		}

		log.Printf("Starting node %d with P2P port %d and RPC port %d...", i, p2pPort, rpcPort)
		nodeCmd := exec.CommandContext(ctx, appPath, runArgs...)
		nodeCmd.Stdout = os.NewFile(0, fmt.Sprintf("node%d-stdout", i))
		nodeCmd.Stderr = os.NewFile(0, fmt.Sprintf("node%d-stderr", i))

		if err := nodeCmd.Start(); err != nil {
			log.Printf("Error starting node %d: %v", i, err)
			return
		}

		nodeCommands = append(nodeCommands, nodeCmd)

		// If this is the aggregator and we have more than one node,
		// get the node address after it's started
		if i == 0 && *numNodes > 1 {
			// Wait a bit for the node to initialize
			time.Sleep(3 * time.Second)

			// Get node ID of the first node to use for peer connections
			nodeIdCmd := exec.Command(appPath, "node-info", fmt.Sprintf("--home=%s", nodeHome))
			nodeInfoOutput, err := nodeIdCmd.CombinedOutput()
			if err != nil {
				log.Printf("Error getting node info for node %d: %v, output: %s", i, err, string(nodeInfoOutput))
				return
			}

			// Parse the output to extract the full address
			nodeInfoStr := string(nodeInfoOutput)
			lines := strings.Split(nodeInfoStr, "\n")

			for _, line := range lines {
				if strings.Contains(line, "🔗 Full Address:") {
					// Extract the address part (after the color code)
					parts := strings.Split(line, "Full Address:")
					if len(parts) >= 2 {
						// Clean up ANSI color codes and whitespace
						cleanAddr := strings.TrimSpace(parts[1])
						// Remove potential ANSI color codes
						cleanAddr = strings.TrimPrefix(cleanAddr, "\033[1;32m")
						cleanAddr = strings.TrimSuffix(cleanAddr, "\033[0m")
						cleanAddr = strings.TrimSpace(cleanAddr)
						aggregatorAddress = cleanAddr
						break
					}
				}
			}

			if aggregatorAddress == "" {
				log.Printf("Could not extract node address from output: %s", nodeInfoStr)
				return
			}

			log.Printf("Aggregator Full Address: %s", aggregatorAddress)
		}

		// Wait a bit before starting the next node
		if i < *numNodes-1 {
			time.Sleep(2 * time.Second)
		}
	}

	// Wait for processes to finish or context cancellation
	errCh := make(chan error, 1+len(nodeCommands))
	go func() {
		errCh <- daCmd.Wait()
	}()

	for i, cmd := range nodeCommands {
		i, cmd := i, cmd // Create local copy for goroutine
		go func() {
			err := cmd.Wait()
			log.Printf("Node %d exited: %v", i, err)
			errCh <- err
		}()
	}

	select {
	case err := <-errCh:
		if ctx.Err() == nil { // If context was not canceled, it's an unexpected error
			log.Printf("One of the processes exited unexpectedly: %v", err)
			// Don't need to call cancel() here as the defer will handle cleanup
		}
	case <-ctx.Done():
		// Context was canceled, we're shutting down gracefully
		log.Println("Shutting down processes...")
	}
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
		"apps",
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
