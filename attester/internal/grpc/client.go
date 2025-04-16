package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure" // Use insecure for now, TODO: Add TLS

	// "google.golang.org/grpc/keepalive"

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
)

// Client provides methods to interact with the leader's AttesterService.
type Client struct {
	conn   *grpc.ClientConn
	client attesterv1.AttesterServiceClient
	logger *slog.Logger
}

// NewClient creates a new client connected to the given target address.
func NewClient(ctx context.Context, target string, logger *slog.Logger) (*Client, error) {
	if target == "" {
		return nil, fmt.Errorf("signature client target address cannot be empty")
	}

	logger = logger.With("component", "grpc-client", "target", target)

	// TODO: Add options for TLS, keepalive, interceptors etc.
	// Example keepalive options:
	// kacp := keepalive.ClientParameters{
	// 	Time:                10 * time.Second, // send pings every 10 seconds if there is no activity
	// 	Timeout:             time.Second,      // wait 1 second for ping ack before considering the connection dead
	// 	PermitWithoutStream: true,             // send pings even without active streams
	// }

	connCtx, cancel := context.WithTimeout(ctx, 5*time.Second) // Timeout for initial connection
	defer cancel()

	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // Use insecure for now
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			// Use connCtx for the dialer context to respect the initial connection timeout
			deadline, _ := connCtx.Deadline() // ok is always true for context.WithTimeout
			timeout := deadline.Sub(time.Now())
			if timeout < 0 {
				timeout = 0 // Ensure timeout is non-negative
			}
			d := net.Dialer{Timeout: timeout}
			return d.DialContext(ctx, "tcp", addr)
		}),
	)
	if err != nil {
		logger.Error("Failed to dial leader gRPC service", "error", err)
		return nil, fmt.Errorf("failed to connect to signature service at %s: %w", target, err)
	}

	logger.Info("Successfully connected to leader gRPC service")

	client := attesterv1.NewAttesterServiceClient(conn)

	return &Client{
		conn:   conn,
		client: client,
		logger: logger,
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		c.logger.Info("Closing connection to leader gRPC service")
		return c.conn.Close()
	}
	return nil
}

// SubmitSignature sends the signature to the leader.
func (c *Client) SubmitSignature(ctx context.Context, height uint64, hash []byte, attesterID string, signature []byte) error {
	req := &attesterv1.SubmitSignatureRequest{
		BlockHeight: height,
		BlockHash:   hash,
		AttesterId:  attesterID,
		Signature:   signature,
	}

	// Add a timeout for the RPC call itself
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second) // Example timeout
	defer cancel()

	// Basic retry logic could be added here if needed
	c.logger.Debug("Submitting signature to leader", "block_height", height, "attester_id", attesterID)
	resp, err := c.client.SubmitSignature(callCtx, req)

	if err != nil {
		c.logger.Error("Failed to submit signature via RPC", "error", err, "block_height", height, "attester_id", attesterID)
		return fmt.Errorf("rpc SubmitSignature failed: %w", err)
	}

	if !resp.Success {
		c.logger.Error("Leader rejected signature submission", "error_message", resp.ErrorMessage, "block_height", height, "attester_id", attesterID)
		return fmt.Errorf("signature submission rejected by leader: %s", resp.ErrorMessage)
	}

	c.logger.Info("Successfully submitted signature to leader", "block_height", height, "attester_id", attesterID)
	return nil
}
