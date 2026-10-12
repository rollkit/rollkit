// Package gcp implements a signer.Signer backed by Google Cloud KMS.
// It delegates signing to a remote KMS key and caches the public key locally.
package gcp

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"sync"
	"time"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/libp2p/go-libp2p/core/crypto"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var castagnoliTable = crc32.MakeTable(crc32.Castagnoli)

const (
	baseRetryBackoff = 100 * time.Millisecond
	maxRetryBackoff  = 5 * time.Second
)

// KMSClient is the subset of the Google Cloud KMS client API that KmsSigner needs.
// This allows mocking in tests.
type KMSClient interface {
	AsymmetricSign(ctx context.Context, req *kmspb.AsymmetricSignRequest) (*kmspb.AsymmetricSignResponse, error)
	GetPublicKey(ctx context.Context, req *kmspb.GetPublicKeyRequest) (*kmspb.PublicKey, error)
}

type cloudKMSClient struct {
	client *kms.KeyManagementClient
}

func (c *cloudKMSClient) AsymmetricSign(ctx context.Context, req *kmspb.AsymmetricSignRequest) (*kmspb.AsymmetricSignResponse, error) {
	return c.client.AsymmetricSign(ctx, req)
}

func (c *cloudKMSClient) GetPublicKey(ctx context.Context, req *kmspb.GetPublicKeyRequest) (*kmspb.PublicKey, error) {
	return c.client.GetPublicKey(ctx, req)
}

// Options configures optional KmsSigner behavior.
type Options struct {
	// CredentialsFile is an optional path to a Google credentials JSON file.
	// If empty, Application Default Credentials are used.
	CredentialsFile string
	// Timeout for individual KMS Sign API calls. Default: 1s.
	Timeout time.Duration
	// MaxRetries for transient KMS failures during Sign. Default: 3.
	MaxRetries int
}

func (o *Options) timeout() time.Duration { return o.Timeout }

func (o *Options) maxRetries() int { return o.MaxRetries }

// KmsSigner implements the signer.Signer interface using Google Cloud KMS.
type KmsSigner struct {
	client  KMSClient
	keyName string
	opts    Options
	mu      sync.RWMutex
	pubKey  crypto.PubKey
	address []byte
}

// NewKmsSigner creates a new Signer backed by a Google Cloud KMS Ed25519 key version.
// It uses Application Default Credentials unless opts.CredentialsFile is provided.
func NewKmsSigner(ctx context.Context, keyName string, opts *Options) (*KmsSigner, error) {
	if keyName == "" {
		return nil, fmt.Errorf("gcp kms key name is required")
	}

	var clientOpts []option.ClientOption
	if opts != nil && opts.CredentialsFile != "" {
		clientOpts = append(clientOpts, option.WithAuthCredentialsFile(option.ServiceAccount, opts.CredentialsFile))
	}

	client, err := kms.NewKeyManagementClient(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Google Cloud KMS client: %w", err)
	}

	return kmsSignerFromClient(ctx, &cloudKMSClient{client: client}, keyName, opts)
}

// kmsSignerFromClient creates a KmsSigner from an existing KMS client.
// Useful for testing with a mock client.
func kmsSignerFromClient(ctx context.Context, client KMSClient, keyName string, opts *Options) (*KmsSigner, error) {
	if keyName == "" {
		return nil, fmt.Errorf("gcp kms key name is required")
	}
	if client == nil {
		return nil, fmt.Errorf("gcp kms client is required")
	}

	o := Options{Timeout: 1 * time.Second, MaxRetries: 3}
	if opts != nil {
		if opts.Timeout > 0 {
			o.Timeout = opts.Timeout
		}
		if opts.MaxRetries >= 0 {
			o.MaxRetries = opts.MaxRetries
		}
	}

	s := &KmsSigner{
		client:  client,
		keyName: keyName,
		opts:    o,
	}

	// Fetch and cache the public key eagerly so we fail fast on misconfiguration.
	if err := s.fetchPublicKey(ctx); err != nil {
		return nil, fmt.Errorf("failed to fetch public key from Google Cloud KMS: %w", err)
	}

	return s, nil
}

// fetchPublicKey retrieves the public key from KMS and caches it.
func (s *KmsSigner) fetchPublicKey(ctx context.Context) error {
	out, err := s.client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{Name: s.keyName})
	if err != nil {
		return fmt.Errorf("KMS GetPublicKey failed: %w", err)
	}

	if out.GetName() != s.keyName {
		return fmt.Errorf("KMS GetPublicKey integrity check failed: unexpected key name %q", out.GetName())
	}
	if out.GetPemCrc32C() == nil {
		return fmt.Errorf("KMS GetPublicKey integrity check failed: pem_crc32c is missing")
	}
	if got, want := out.GetPemCrc32C().GetValue(), int64(crc32.Checksum([]byte(out.GetPem()), castagnoliTable)); got != want {
		return fmt.Errorf("KMS GetPublicKey integrity check failed: pem_crc32c mismatch")
	}
	block, _ := pem.Decode([]byte(out.GetPem()))
	if block == nil {
		return fmt.Errorf("failed to decode PEM public key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse KMS public key: %w", err)
	}

	edPubKey, ok := pub.(ed25519.PublicKey)
	if !ok {
		return fmt.Errorf("unsupported key type from KMS: expected ed25519, got %T", pub)
	}

	cryptoPubKey, err := crypto.UnmarshalEd25519PublicKey(edPubKey)
	if err != nil {
		return fmt.Errorf("failed to convert to libp2p pubkey: %w", err)
	}

	bz, err := cryptoPubKey.Raw()
	if err != nil {
		return fmt.Errorf("failed to get raw pubkey bytes: %w", err)
	}

	address := sha256.Sum256(bz)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pubKey = cryptoPubKey
	s.address = address[:]

	return nil
}

// Sign signs a message using the remote KMS key with configurable timeout
// and retry with exponential backoff.
func (s *KmsSigner) Sign(ctx context.Context, message []byte) ([]byte, error) {
	var lastErr error
	maxRetries := s.opts.maxRetries()
	timeout := s.opts.timeout()
	maxAttempts := maxRetries + 1

	for attempt := range maxAttempts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			// Exponential backoff with cap: 100ms, 200ms, 400ms, ... up to 5s.
			backoff := retryBackoff(attempt)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("KMS Sign canceled: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		callCtx, cancel := context.WithTimeout(ctx, timeout)
		dataCRC32C := int64(crc32.Checksum(message, castagnoliTable))
		out, err := s.client.AsymmetricSign(callCtx, &kmspb.AsymmetricSignRequest{
			Name:       s.keyName,
			Data:       message,
			DataCrc32C: wrapperspb.Int64(dataCRC32C),
		})
		cancel()

		if err != nil {
			lastErr = err
			if !isRetryableKMSError(err) {
				return nil, fmt.Errorf("GCP KMS sign failed with non-retryable error: %w", err)
			}
			continue
		}

		if err := verifySignResponse(out, s.keyName); err != nil {
			lastErr = err
			continue
		}

		return out.GetSignature(), nil
	}

	return nil, fmt.Errorf("KMS Sign failed after %d attempts: %w", maxAttempts, lastErr)
}

func retryBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return baseRetryBackoff
	}

	backoff := baseRetryBackoff
	for i := 1; i < attempt; i++ {
		if backoff >= maxRetryBackoff/2 {
			return maxRetryBackoff
		}
		backoff *= 2
	}

	if backoff > maxRetryBackoff {
		return maxRetryBackoff
	}

	return backoff
}

func verifySignResponse(out *kmspb.AsymmetricSignResponse, expectedName string) error {
	if !out.GetVerifiedDataCrc32C() {
		return fmt.Errorf("KMS Sign integrity check failed: verified_data_crc32c is false")
	}
	if out.GetName() != expectedName {
		return fmt.Errorf("KMS Sign integrity check failed: unexpected key name %q", out.GetName())
	}
	signatureCRC32C := out.GetSignatureCrc32C()
	if signatureCRC32C == nil {
		return fmt.Errorf("KMS Sign integrity check failed: signature_crc32c is missing")
	}

	signature := out.GetSignature()
	expectedCRC32C := int64(crc32.Checksum(signature, castagnoliTable))
	if signatureCRC32C.GetValue() != expectedCRC32C {
		return fmt.Errorf("KMS Sign integrity check failed: signature_crc32c mismatch")
	}

	return nil
}

// GetPublic returns the cached public key.
func (s *KmsSigner) GetPublic() (crypto.PubKey, error) {
	s.mu.RLock()
	pubKey := s.pubKey
	s.mu.RUnlock()

	if pubKey == nil {
		return nil, fmt.Errorf("public key not loaded")
	}

	return pubKey, nil
}

// GetAddress returns the cached address derived from the public key.
func (s *KmsSigner) GetAddress() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.address == nil {
		return nil, fmt.Errorf("address not loaded")
	}
	r := make([]byte, len(s.address))
	copy(r, s.address)
	return r, nil
}

func isRetryableKMSError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}

	switch status.Code(err) {
	case codes.DeadlineExceeded, codes.Unavailable, codes.ResourceExhausted, codes.Aborted, codes.Internal:
		return true
	default:
		return false
	}
}
