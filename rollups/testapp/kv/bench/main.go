package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

// checkServerStatus attempts to connect to the server's /store endpoint.
func checkServerStatus(serverAddr string) error {
	checkURL := serverAddr + "/store"
	resp, err := http.Get(checkURL)
	if err != nil {
		return fmt.Errorf("server not reachable at %s: %w", checkURL, err)
	}
	defer resp.Body.Close()
	// We expect OK for the store endpoint, even if empty
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned non-OK status (%d) at %s", resp.StatusCode, checkURL)
	}
	fmt.Printf("Server check successful at %s\n", checkURL)
	return nil
}

func main() {
	serverAddr := flag.String("addr", "http://localhost:9090", "KV executor HTTP server address")
	listStore := flag.Bool("list", false, "List all key-value pairs in the store")

	// Benchmarking parameters
	duration := flag.Duration("duration", 30*time.Second, "Total duration for the benchmark")
	interval := flag.Duration("interval", 1*time.Second, "Interval between batches of transactions")
	txPerInterval := flag.Int("tx-per-interval", 10, "Number of transactions to send in each interval")

	flag.Parse()

	// Seed random number generator
	rand.Seed(time.Now().UnixNano())

	// Validate the server URL format first
	parsedBaseURL, err := url.Parse(*serverAddr)
	if err != nil || (parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") {
		fmt.Fprintf(os.Stderr, "Invalid server base URL format: %s\n", *serverAddr)
		os.Exit(1)
	}

	// Check if the server is reachable before proceeding
	if err := checkServerStatus(*serverAddr); err != nil {
		fmt.Fprintf(os.Stderr, "Server status check failed: %v\n", err)
		os.Exit(1)
	}

	// List store contents
	if *listStore {
		resp, err := http.Get(*serverAddr + "/store")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to server: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "Error closing response body: %v\n", err)
			}
		}()

		buffer := new(bytes.Buffer)
		_, err = buffer.ReadFrom(resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(buffer.String())
		return
	}

	// No need to re-parse, but create the final URL object
	safeURL := url.URL{
		Scheme: parsedBaseURL.Scheme,
		Host:   parsedBaseURL.Host,
		Path:   "/tx",
	}

	// Run benchmark
	runBenchmark(safeURL.String(), *duration, *interval, *txPerInterval)
}

// runBenchmark sends transactions to the server at the specified interval
func runBenchmark(url string, duration, interval time.Duration, txPerInterval int) {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var (
		successCount uint64
		failureCount uint64
		wg           sync.WaitGroup
	)

	startTime := time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Printf("Starting benchmark for %s with %d tx every %s\n",
		duration.String(), txPerInterval, interval.String())

	for {
		select {
		case <-ctx.Done():
			wg.Wait() // Wait for any in-progress transactions to complete
			endTime := time.Now()
			elapsed := endTime.Sub(startTime)
			totalTx := successCount + failureCount

			fmt.Println("\nBenchmark complete")
			fmt.Printf("Duration: %s\n", elapsed.String())
			fmt.Printf("Total transactions: %d\n", totalTx)
			fmt.Printf("Successful transactions: %d\n", successCount)
			fmt.Printf("Failed transactions: %d\n", failureCount)

			if elapsed.Seconds() > 0 {
				txPerSec := float64(totalTx) / elapsed.Seconds()
				fmt.Printf("Throughput: %.2f tx/sec\n", txPerSec)
			}

			return

		case <-ticker.C:
			// Send a batch of transactions
			for i := 0; i < txPerInterval; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					var currentTxData string
					// Generate random key-value pair
					key := randomString(8)
					value := randomString(16)
					currentTxData = fmt.Sprintf("%s=%s", key, value)
					success := sendTransaction(url, currentTxData)
					if success {
						atomic.AddUint64(&successCount, 1)
					} else {
						atomic.AddUint64(&failureCount, 1)
					}
				}()
			}

			// Print progress
			current := atomic.LoadUint64(&successCount) + atomic.LoadUint64(&failureCount)
			fmt.Printf("\rTransactions sent: %d (success: %d, failed: %d)",
				current, atomic.LoadUint64(&successCount), atomic.LoadUint64(&failureCount))
		}
	}
}

// sendTransaction sends a single transaction and returns true if successful
func sendTransaction(url, txData string) bool {
	resp, err := http.Post(url, "text/plain", strings.NewReader(txData))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusAccepted
}
