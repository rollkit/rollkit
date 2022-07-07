package config

// RPCConfig holds RPC configuration params.
type RPCConfig struct {
	ListenAddress string

	// Cross Origin Resource Sharing settings
	CORSAllowedOrigins []string
	CORSAllowedMethods []string
	CORSAllowedHeaders []string

	// Maximum number of simultaneous connections (including WebSocket).
	// Does not include gRPC connections. See grpc-max-open-connections
	// If you want to accept a larger number than the default, make sure
	// you increase your OS limits.
	// 0 - unlimited.
	// Should be < {ulimit -Sn} - {MaxNumInboundPeers} - {MaxNumOutboundPeers} - {N of wal, db and other open files}
	// 1024 - 40 - 10 - 50 = 924 = ~900
	MaxOpenConnections int

	// The path to a file containing certificate that is used to create the HTTPS server.
	// Might be either absolute path or path related to Tendermint's config directory.
	//
	// If the certificate is signed by a certificate authority,
	// the certFile should be the concatenation of the server's certificate, any intermediates,
	// and the CA's certificate.
	//
	// NOTE: both tls-cert-file and tls-key-file must be present for Tendermint to create HTTPS server.
	// Otherwise, HTTP server is run.
	TLSCertFile string `mapstructure:"tls-cert-file"`

	// The path to a file containing matching private key that is used to create the HTTPS server.
	// Might be either absolute path or path related to tendermint's config directory.
	//
	// NOTE: both tls-cert-file and tls-key-file must be present for Tendermint to create HTTPS server.
	// Otherwise, HTTP server is run.
	TLSKeyFile string `mapstructure:"tls-key-file"`
}
