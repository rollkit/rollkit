package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func main() {
	serverAddr := flag.String("addr", "http://localhost:40042", "KV executor HTTP server address")
	key := flag.String("key", "", "Key for the transaction")
	value := flag.String("value", "", "Value for the transaction")
	rawTx := flag.String("raw", "", "Raw transaction data (instead of key/value)")
	listStore := flag.Bool("list", false, "List all key-value pairs in the store")
	flag.Parse()

	// List store contents
	if *listStore {
		resp, err := http.Get(*serverAddr + "/store")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to server: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		buffer := new(bytes.Buffer)
		_, err = buffer.ReadFrom(resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(buffer.String())
		return
	}

	// Get transaction data
	var txData string
	if *rawTx != "" {
		txData = *rawTx
	} else if *key != "" {
		txData = fmt.Sprintf("%s=%s", *key, *value)
	} else {
		fmt.Println("Please provide either a raw transaction with -raw or a key/value pair with -key and -value")
		flag.Usage()
		os.Exit(1)
	}

	// Send transaction
	url := *serverAddr + "/tx"
	resp, err := http.Post(url, "text/plain", strings.NewReader(txData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending transaction: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		buffer := new(bytes.Buffer)
		_, err = buffer.ReadFrom(resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Server returned error: %s\n", buffer.String())
		os.Exit(1)
	}

	fmt.Println("Transaction submitted successfully")
}
