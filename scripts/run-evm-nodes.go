//go:build run_evm
// +build run_evm

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const (
	// Sequencer ports
	sequencerRPCPort   = 7331
	sequencerP2PPort   = 7676
	sequencerEVMRPC    = 8545
	sequencerEVMEngine = 8551
	sequencerEVMWS     = 8546

	// Full node ports
	fullNodeRPCPort   = 7332
	fullNodeP2PPort   = 7677
	fullNodeEVMRPC    = 8555
	fullNodeEVMEngine = 8561
	fullNodeEVMWS     = 8556

	// Common ports
	daPort = 7980

	// Genesis hash for EVM
	genesisHash = "0x2b8bbb1ea1e04f9c9809b4b278a8687806edc061a356c7dbc491930d8e922503"
)

type nodeManager struct {
	ctx           context.Context
	cancel        context.CancelFunc
	projectRoot   string
	jwtPath       string
	cleanOnExit   bool
	logLevel      string
	processes     []*exec.Cmd
	nodeDirs      []string
	dockerCleanup []string
}

func main() {
	// Parse command line arguments
	cleanOnExit := flag.Bool("clean-on-exit", true, "Remove node directories on exit")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Create node manager
	nm := &nodeManager{
		cleanOnExit: *cleanOnExit,
		logLevel:    *logLevel,
		processes:   make([]*exec.Cmd, 0),
		nodeDirs:    make([]string, 0),
	}

	// Setup context for cancellation
	nm.ctx, nm.cancel = context.WithCancel(context.Background())

	// Setup cleanup on exit
	defer nm.cleanup()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down...", sig)
		nm.cancel()
	}()

	// Find project root
	var err error
	nm.projectRoot, err = findProjectRoot()
	if err != nil {
		log.Printf("Failed to find project root: %v", err)
		return
	}
	log.Printf("Project root: %s", nm.projectRoot)

	// Execute the setup sequence
	if err := nm.run(); err != nil {
		log.Printf("Failed to run nodes: %v", err)
		return // cleanup will run via defer
	}

	// Wait for cancellation
	<-nm.ctx.Done()
	log.Println("Shutting down...")
}

func (nm *nodeManager) run() error {
	// 1. Setup JWT token
	if err := nm.setupJWT(); err != nil {
		return fmt.Errorf("failed to setup JWT: %w", err)
	}

	// 3. Start Local DA
	if err := nm.startLocalDA(); err != nil {
		return fmt.Errorf("failed to start local DA: %w", err)
	}

	// 4. Start EVM execution layers
	if err := nm.startEVMExecutionLayers(); err != nil {
		return fmt.Errorf("failed to start EVM execution layers: %w", err)
	}

	// 5. Initialize and start sequencer
	sequencerP2PAddr, err := nm.startSequencer()
	if err != nil {
		return fmt.Errorf("failed to start sequencer: %w", err)
	}

	// 6. Initialize and start full node
	if err := nm.startFullNode(sequencerP2PAddr); err != nil {
		return fmt.Errorf("failed to start full node: %w", err)
	}

	log.Println("All nodes started successfully!")
	log.Printf("Sequencer RPC: http://localhost:%d", sequencerRPCPort)
	log.Printf("Full Node RPC: http://localhost:%d", fullNodeRPCPort)

	// Monitor processes
	return nm.monitorProcesses()
}

func (nm *nodeManager) setupJWT() error {
	// Create temporary directory for JWT
	tmpDir, err := os.MkdirTemp("", "rollkit-evm-jwt-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	nm.jwtPath = filepath.Join(tmpDir, "jwt.hex")

	// Generate JWT token using crypto/rand (same as test helpers)
	jwtSecret := make([]byte, 32)
	if _, err := rand.Read(jwtSecret); err != nil {
		return fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Convert to hex string (no newline)
	jwtToken := hex.EncodeToString(jwtSecret)

	// Write JWT to file without newline
	if err := os.WriteFile(nm.jwtPath, []byte(jwtToken), 0600); err != nil {
		return fmt.Errorf("failed to write JWT: %w", err)
	}

	log.Printf("Generated JWT token at: %s", nm.jwtPath)
	return nil
}

func (nm *nodeManager) startLocalDA() error {
	log.Println("Starting local-da...")
	daPath := filepath.Join(nm.projectRoot, "build", "local-da")

	cmd := exec.CommandContext(nm.ctx, daPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start local-da: %w", err)
	}

	nm.processes = append(nm.processes, cmd)

	// Wait for DA to initialize
	log.Println("Waiting for local-da to initialize...")
	time.Sleep(3 * time.Second)

	return nil
}

func (nm *nodeManager) startEVMExecutionLayers() error {
	// Start sequencer's EVM layer
	log.Println("Starting sequencer's EVM execution layer...")
	if err := nm.startEVMDocker("sequencer", sequencerEVMRPC, sequencerEVMEngine, sequencerEVMWS); err != nil {
		return fmt.Errorf("failed to start sequencer EVM: %w", err)
	}

	// Start full node's EVM layer
	log.Println("Starting full node's EVM execution layer...")
	if err := nm.startEVMDocker("fullnode", fullNodeEVMRPC, fullNodeEVMEngine, fullNodeEVMWS); err != nil {
		return fmt.Errorf("failed to start full node EVM: %w", err)
	}

	// Wait for EVM layers to initialize
	log.Println("Waiting for EVM layers to initialize...")
	time.Sleep(5 * time.Second)

	return nil
}

func (nm *nodeManager) startEVMDocker(name string, rpcPort, enginePort, wsPort int) error {
	containerName := fmt.Sprintf("rollkit-evm-%s", name)

	// Stop any existing container
	stopCmd := exec.Command("docker", "stop", containerName)
	stopCmd.Run() // Ignore error if container doesn't exist

	removeCmd := exec.Command("docker", "rm", containerName)
	removeCmd.Run() // Ignore error if container doesn't exist

	// Get chain genesis path
	chainPath := filepath.Join(nm.projectRoot, "execution", "evm", "docker", "chain", "genesis.json")

	// Run new container
	args := []string{
		"run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("%d:8545", rpcPort),
		"-p", fmt.Sprintf("%d:8551", enginePort),
		"-p", fmt.Sprintf("%d:8546", wsPort),
		"-v", fmt.Sprintf("%s:/jwt/jwt.hex:ro", nm.jwtPath),
		"-v", fmt.Sprintf("%s:/chain/genesis.json:ro", chainPath),
		"ghcr.io/rollkit/lumen:latest",
		"node",
		"--chain", "/chain/genesis.json",
		"--authrpc.addr", "0.0.0.0",
		"--authrpc.port", "8551",
		"--authrpc.jwtsecret", "/jwt/jwt.hex",
		"--http", "--http.addr", "0.0.0.0", "--http.port", "8545",
		"--http.api", "eth,net,web3,txpool",
		"--ws", "--ws.addr", "0.0.0.0", "--ws.port", "8546",
		"--ws.api", "eth,net,web3",
		"--engine.persistence-threshold", "0",
		"--engine.memory-block-buffer-target", "0",
		"--disable-discovery",
		"--rollkit.enable",
	}

	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start container: %w, output: %s", err, output)
	}

	nm.dockerCleanup = append(nm.dockerCleanup, containerName)
	log.Printf("Started EVM container: %s", containerName)

	return nil
}

func (nm *nodeManager) startSequencer() (string, error) {
	log.Println("Initializing sequencer node...")

	sequencerHome := filepath.Join(nm.projectRoot, ".evm-single-sequencer")
	nm.nodeDirs = append(nm.nodeDirs, sequencerHome)

	// Remove existing directory
	os.RemoveAll(sequencerHome)

	evmSinglePath := filepath.Join(nm.projectRoot, "build", "evm-single")

	// Initialize sequencer
	initArgs := []string{
		"init",
		fmt.Sprintf("--home=%s", sequencerHome),
		"--rollkit.node.aggregator=true",
		"--rollkit.signer.passphrase=secret",
	}

	initCmd := exec.Command(evmSinglePath, initArgs...)
	initCmd.Stdout = os.Stdout
	initCmd.Stderr = os.Stderr
	if err := initCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to initialize sequencer: %w", err)
	}

	// Read JWT secret from file
	jwtContent, err := os.ReadFile(nm.jwtPath)
	if err != nil {
		return "", fmt.Errorf("failed to read JWT secret: %w", err)
	}

	// Start sequencer
	log.Println("Starting sequencer node...")
	runArgs := []string{
		"start",
		fmt.Sprintf("--home=%s", sequencerHome),
		fmt.Sprintf("--evm.jwt-secret=%s", string(jwtContent)),
		fmt.Sprintf("--evm.genesis-hash=%s", genesisHash),
		"--rollkit.node.block_time=1s",
		"--rollkit.node.aggregator=true",
		"--rollkit.signer.passphrase=secret",
		fmt.Sprintf("--rollkit.rpc.address=127.0.0.1:%d", sequencerRPCPort),
		fmt.Sprintf("--rollkit.p2p.listen_address=/ip4/127.0.0.1/tcp/%d", sequencerP2PPort),
		fmt.Sprintf("--rollkit.da.address=http://localhost:%d", daPort),
		fmt.Sprintf("--evm.eth-url=http://localhost:%d", sequencerEVMRPC),
		fmt.Sprintf("--evm.engine-url=http://localhost:%d", sequencerEVMEngine),
	}

	runCmd := exec.CommandContext(nm.ctx, evmSinglePath, runArgs...)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr

	if err := runCmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start sequencer: %w", err)
	}

	nm.processes = append(nm.processes, runCmd)

	// Wait for node to initialize
	log.Println("Waiting for sequencer to initialize...")
	time.Sleep(5 * time.Second)

	// Get node info to extract P2P address
	netInfoCmd := exec.Command(evmSinglePath, "net-info",
		fmt.Sprintf("--home=%s", sequencerHome))
	netInfoOutput, err := netInfoCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get net-info: %w, output: %s", err, string(netInfoOutput))
	}

	// Parse the output to extract the full address
	netInfoStr := string(netInfoOutput)
	lines := strings.Split(netInfoStr, "\n")

	var p2pAddress string
	for _, line := range lines {
		// Look for the listen address line with the full P2P address
		if strings.Contains(line, "Addr:") && strings.Contains(line, "/p2p/") {
			// Extract everything after "Addr: "
			addrIdx := strings.Index(line, "Addr:")
			if addrIdx != -1 {
				addrPart := line[addrIdx+5:] // Skip "Addr:"
				addrPart = strings.TrimSpace(addrPart)

				// If this line contains the full P2P address format
				if strings.Contains(addrPart, "/ip4/") && strings.Contains(addrPart, "/tcp/") && strings.Contains(addrPart, "/p2p/") {
					// Remove ANSI color codes
					p2pAddress = stripANSI(addrPart)
					break
				}
			}
		}
	}

	if p2pAddress == "" {
		return "", fmt.Errorf("could not extract P2P address from netinfo output: %s", netInfoStr)
	}

	log.Printf("Sequencer P2P address: %s", p2pAddress)
	return p2pAddress, nil
}

func (nm *nodeManager) startFullNode(sequencerP2PAddr string) error {
	log.Println("Initializing full node...")

	fullNodeHome := filepath.Join(nm.projectRoot, ".evm-single-fullnode")
	nm.nodeDirs = append(nm.nodeDirs, fullNodeHome)

	// Remove existing directory
	os.RemoveAll(fullNodeHome)

	evmSinglePath := filepath.Join(nm.projectRoot, "build", "evm-single")

	// Initialize full node
	initArgs := []string{
		"init",
		fmt.Sprintf("--home=%s", fullNodeHome),
	}

	initCmd := exec.Command(evmSinglePath, initArgs...)
	initCmd.Stdout = os.Stdout
	initCmd.Stderr = os.Stderr
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("failed to initialize full node: %w", err)
	}

	// Copy genesis from sequencer
	log.Println("Copying genesis from sequencer to full node...")
	sequencerGenesis := filepath.Join(nm.projectRoot, ".evm-single-sequencer", "config", "genesis.json")
	fullNodeGenesis := filepath.Join(fullNodeHome, "config", "genesis.json")

	srcFile, err := os.Open(sequencerGenesis)
	if err != nil {
		return fmt.Errorf("failed to open sequencer genesis: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(fullNodeGenesis)
	if err != nil {
		return fmt.Errorf("failed to create full node genesis: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy genesis: %w", err)
	}

	// Read JWT secret from file
	jwtContent, err := os.ReadFile(nm.jwtPath)
	if err != nil {
		return fmt.Errorf("failed to read JWT secret: %w", err)
	}

	// Start full node
	log.Println("Starting full node...")
	runArgs := []string{
		"start",
		fmt.Sprintf("--home=%s", fullNodeHome),
		fmt.Sprintf("--evm.jwt-secret=%s", string(jwtContent)),
		fmt.Sprintf("--evm.genesis-hash=%s", genesisHash),
		fmt.Sprintf("--rollkit.rpc.address=127.0.0.1:%d", fullNodeRPCPort),
		fmt.Sprintf("--rollkit.p2p.listen_address=/ip4/127.0.0.1/tcp/%d", fullNodeP2PPort),
		fmt.Sprintf("--rollkit.p2p.peers=%s", sequencerP2PAddr),
		fmt.Sprintf("--rollkit.da.address=http://localhost:%d", daPort),
		fmt.Sprintf("--evm.eth-url=http://localhost:%d", fullNodeEVMRPC),
		fmt.Sprintf("--evm.engine-url=http://localhost:%d", fullNodeEVMEngine),
	}

	runCmd := exec.CommandContext(nm.ctx, evmSinglePath, runArgs...)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr

	if err := runCmd.Start(); err != nil {
		return fmt.Errorf("failed to start full node: %w", err)
	}

	nm.processes = append(nm.processes, runCmd)

	// Wait a bit for the node to start
	time.Sleep(3 * time.Second)

	return nil
}

func (nm *nodeManager) monitorProcesses() error {
	// Monitor all processes
	errCh := make(chan error, len(nm.processes))

	for i, cmd := range nm.processes {
		go func(idx int, c *exec.Cmd) {
			err := c.Wait()
			if nm.ctx.Err() == nil {
				log.Printf("Process %d exited unexpectedly: %v", idx, err)
			}
			errCh <- err
		}(i, cmd)
	}

	// Wait for context cancellation or process exit
	select {
	case err := <-errCh:
		if nm.ctx.Err() == nil {
			return fmt.Errorf("process exited unexpectedly: %w", err)
		}
	case <-nm.ctx.Done():
		// Normal shutdown
	}

	return nil
}

func (nm *nodeManager) cleanup() {
	log.Println("Starting cleanup...")

	// Cancel context
	nm.cancel()

	// Give processes time to shutdown gracefully
	time.Sleep(2 * time.Second)

	// Kill processes
	for _, cmd := range nm.processes {
		if cmd.Process != nil {
			log.Printf("Terminating process %d", cmd.Process.Pid)
			cmd.Process.Signal(syscall.SIGTERM)

			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			select {
			case <-done:
				// Process terminated
			case <-time.After(5 * time.Second):
				// Force kill
				log.Printf("Force killing process %d", cmd.Process.Pid)
				cmd.Process.Kill()
			}
		}
	}

	// Stop Docker containers
	for _, container := range nm.dockerCleanup {
		log.Printf("Stopping Docker container: %s", container)
		stopCmd := exec.Command("docker", "stop", container)
		stopCmd.Run()

		removeCmd := exec.Command("docker", "rm", container)
		removeCmd.Run()
	}

	// Remove node directories if requested
	if nm.cleanOnExit {
		for _, dir := range nm.nodeDirs {
			log.Printf("Removing directory: %s", dir)
			os.RemoveAll(dir)
		}
	}

	// Remove JWT directory
	if nm.jwtPath != "" {
		jwtDir := filepath.Dir(nm.jwtPath)
		log.Printf("Removing JWT directory: %s", jwtDir)
		os.RemoveAll(jwtDir)
	}

	log.Println("Cleanup complete")
}

func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if we're already in the project root
	if isProjectRoot(cwd) {
		return cwd, nil
	}

	// Walk up to find project root
	dir := cwd
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
		if isProjectRoot(dir) {
			return dir, nil
		}
	}

	return "", fmt.Errorf("could not find project root")
}

func isProjectRoot(dir string) bool {
	markers := []string{
		"apps/evm/single",
		"execution/evm",
		"da",
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

// stripANSI removes ANSI escape sequences from a string
func stripANSI(str string) string {
	// Regular expression to match ANSI escape sequences
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiRegex.ReplaceAllString(str, "")
}
