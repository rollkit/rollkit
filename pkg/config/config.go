package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/celestiaorg/go-square/v3/share"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	datypes "github.com/evstack/ev-node/pkg/da/types"
)

const (
	FlagPrefixRollkit = "rollkit."
	FlagPrefixEvnode  = "evnode."

	// Base configuration flags

	// FlagRootDir is a flag for specifying the root directory
	FlagRootDir = "home"
	// FlagDBPath is a flag for specifying the database path
	FlagDBPath = FlagPrefixEvnode + "db_path"

	// Node configuration flags

	// FlagAggregator is a flag for running node in aggregator mode
	FlagAggregator = FlagPrefixEvnode + "node.aggregator"
	// FlagPromotable is a flag for running a full node with aggregator dependencies available for future promotion
	FlagPromotable = FlagPrefixEvnode + "node.promotable"
	// FlagBasedSequencer is a flag for enabling based sequencer mode (requires aggregator mode)
	FlagBasedSequencer = FlagPrefixEvnode + "node.based_sequencer"
	// FlagLight is a flag for running the node in light mode
	FlagLight = FlagPrefixEvnode + "node.light"
	// FlagBlockTime is a flag for specifying the block time
	FlagBlockTime = FlagPrefixEvnode + "node.block_time"
	// FlagLazyAggregator is a flag for enabling lazy aggregation mode that only produces blocks when transactions are available
	FlagLazyAggregator = FlagPrefixEvnode + "node.lazy_mode"
	// FlagMaxPendingHeadersAndData is a flag to limit and pause block production when too many headers or data are waiting for DA confirmation
	FlagMaxPendingHeadersAndData = FlagPrefixEvnode + "node.max_pending_headers_and_data"
	// FlagLazyBlockTime is a flag for specifying the maximum interval between blocks in lazy aggregation mode
	FlagLazyBlockTime = FlagPrefixEvnode + "node.lazy_block_interval"
	// FlagReadinessWindowSeconds configures the time window (in seconds) used to calculate readiness threshold
	FlagReadinessWindowSeconds = FlagPrefixEvnode + "node.readiness_window_seconds"
	// FlagReadinessMaxBlocksBehind configures how many blocks behind best-known head is still considered ready
	FlagReadinessMaxBlocksBehind = FlagPrefixEvnode + "node.readiness_max_blocks_behind"
	// FlagScrapeInterval is a flag for specifying the reaper scrape interval
	FlagScrapeInterval = FlagPrefixEvnode + "node.scrape_interval"
	// FlagCatchupTimeout enables recovery catchup and limits configured P2P recovery.
	FlagCatchupTimeout = FlagPrefixEvnode + "node.catchup_timeout"
	// FlagClearCache is a flag for clearing the cache
	FlagClearCache = FlagPrefixEvnode + "clear_cache"

	// Data Availability configuration flags

	// FlagDAAddress is a flag for specifying the data availability layer address
	FlagDAAddress = FlagPrefixEvnode + "da.address"
	// FlagDAAuthToken is a flag for specifying the data availability layer auth token
	FlagDAAuthToken = FlagPrefixEvnode + "da.auth_token" // #nosec G101
	// FlagDABlockTime is a flag for specifying the data availability layer block time
	FlagDABlockTime = FlagPrefixEvnode + "da.block_time"
	// FlagDANamespace is a flag for specifying the DA namespace ID
	FlagDANamespace = FlagPrefixEvnode + "da.namespace"
	// FlagDADataNamespace is a flag for specifying the DA data namespace ID
	FlagDADataNamespace = FlagPrefixEvnode + "da.data_namespace"
	// FlagDAForcedInclusionNamespace is a flag for specifying the DA forced inclusion namespace ID
	FlagDAForcedInclusionNamespace = FlagPrefixEvnode + "da.forced_inclusion_namespace"
	// FlagDASubmitOptions is a flag for data availability submit options
	FlagDASubmitOptions = FlagPrefixEvnode + "da.submit_options"
	// FlagDASigningAddresses is a flag for specifying multiple DA signing addresses
	FlagDASigningAddresses = FlagPrefixEvnode + "da.signing_addresses"
	// FlagDAMempoolTTL is a flag for specifying the DA mempool TTL
	FlagDAMempoolTTL = FlagPrefixEvnode + "da.mempool_ttl"
	// FlagDAMaxSubmitAttempts is a flag for specifying the maximum DA submit attempts
	FlagDAMaxSubmitAttempts = FlagPrefixEvnode + "da.max_submit_attempts"
	// FlagDARequestTimeout controls the per-request timeout when talking to the DA layer
	FlagDARequestTimeout = FlagPrefixEvnode + "da.request_timeout"
	// FlagDABatchingStrategy is a flag for specifying the batching strategy
	FlagDABatchingStrategy = FlagPrefixEvnode + "da.batching_strategy"
	// FlagDABatchSizeThreshold is a flag for specifying the batch size threshold
	FlagDABatchSizeThreshold = FlagPrefixEvnode + "da.batch_size_threshold"
	// FlagDABatchMaxDelay is a flag for specifying the maximum batch delay
	FlagDABatchMaxDelay = FlagPrefixEvnode + "da.batch_max_delay"
	// FlagDABatchMinItems is a flag for specifying the minimum batch items
	FlagDABatchMinItems = FlagPrefixEvnode + "da.batch_min_items"
	// FlagDAStartHeight is a flag for forcing the DA retrieval height to start from a specific height
	FlagDAStartHeight = FlagPrefixEvnode + "da.start_height"
	// FlagDATxSignerEnabled enables signing PayForBlobs locally instead of via the DA node
	FlagDATxSignerEnabled = FlagPrefixEvnode + "da.tx_signer.enabled"
	// FlagDATxSignerGRPCAddress is the consensus gRPC endpoint transactions are broadcast to
	FlagDATxSignerGRPCAddress = FlagPrefixEvnode + "da.tx_signer.grpc_address"
	// FlagDATxSignerGRPCAuthToken is sent as x-token metadata on consensus gRPC calls
	FlagDATxSignerGRPCAuthToken = FlagPrefixEvnode + "da.tx_signer.grpc_auth_token" // #nosec G101
	// FlagDATxSignerGRPCTLS enables TLS for the consensus gRPC connection
	FlagDATxSignerGRPCTLS = FlagPrefixEvnode + "da.tx_signer.grpc_tls"
	// FlagDATxSignerKeyringDir is the keyring directory
	FlagDATxSignerKeyringDir = FlagPrefixEvnode + "da.tx_signer.keyring_dir"
	// FlagDATxSignerKeyringBackend is the keyring backend (file, test)
	FlagDATxSignerKeyringBackend = FlagPrefixEvnode + "da.tx_signer.keyring_backend"
	// FlagDATxSignerKeyName is the keyring key that signs and pays for blobs
	FlagDATxSignerKeyName = FlagPrefixEvnode + "da.tx_signer.key_name"
	// FlagDATxSignerPassphraseFile is the file holding the keyring passphrase for the file backend
	FlagDATxSignerPassphraseFile = FlagPrefixEvnode + "da.tx_signer.passphrase_file"

	// P2P configuration flags

	// FlagP2PListenAddress is a flag for specifying the P2P listen address
	FlagP2PListenAddress = FlagPrefixEvnode + "p2p.listen_address"
	// FlagP2PPeers is a flag for specifying the P2P peers
	FlagP2PPeers = FlagPrefixEvnode + "p2p.peers"
	// FlagP2PBlockedPeers is a flag for specifying the P2P blocked peers
	FlagP2PBlockedPeers = FlagPrefixEvnode + "p2p.blocked_peers"
	// FlagP2PAllowedPeers is a flag for specifying the P2P allowed peers
	FlagP2PAllowedPeers = FlagPrefixEvnode + "p2p.allowed_peers"
	// FlagP2PDisableConnectionGater disables the P2P connection gater (no-op mode).
	// Enabled by default; set to false to activate peer filtering when experiencing P2P flooding.
	FlagP2PDisableConnectionGater = FlagPrefixEvnode + "p2p.disable_connection_gater"

	// Instrumentation configuration flags

	// FlagPrometheus is a flag for enabling Prometheus metrics
	FlagPrometheus = FlagPrefixEvnode + "instrumentation.prometheus"
	// FlagPrometheusListenAddr is a flag for specifying the Prometheus listen address
	FlagPrometheusListenAddr = FlagPrefixEvnode + "instrumentation.prometheus_listen_addr"
	// FlagMaxOpenConnections is a flag for specifying the maximum number of open connections
	FlagMaxOpenConnections = FlagPrefixEvnode + "instrumentation.max_open_connections"
	// FlagPprof is a flag for enabling pprof profiling endpoints for runtime debugging
	FlagPprof = FlagPrefixEvnode + "instrumentation.pprof"
	// FlagPprofListenAddr is a flag for specifying the pprof listen address
	FlagPprofListenAddr = FlagPrefixEvnode + "instrumentation.pprof_listen_addr"

	// Tracing configuration flags

	// FlagTracing enables OpenTelemetry tracing
	FlagTracing = FlagPrefixEvnode + "instrumentation.tracing"
	// FlagTracingEndpoint configures the OTLP endpoint (host:port)
	FlagTracingEndpoint = FlagPrefixEvnode + "instrumentation.tracing_endpoint"
	// FlagTracingServiceName configures the service.name resource attribute
	FlagTracingServiceName = FlagPrefixEvnode + "instrumentation.tracing_service_name"
	// FlagTracingSampleRate configures the TraceID ratio-based sampler
	FlagTracingSampleRate = FlagPrefixEvnode + "instrumentation.tracing_sample_rate"

	// Logging configuration flags

	// FlagLogLevel is a flag for specifying the log level
	FlagLogLevel = FlagPrefixEvnode + "log.level"
	// FlagLogFormat is a flag for specifying the log format
	FlagLogFormat = FlagPrefixEvnode + "log.format"
	// FlagLogTrace is a flag for enabling stack traces in error logs
	FlagLogTrace = FlagPrefixEvnode + "log.trace"

	// Signer configuration flags

	// FlagSignerType is a flag for specifying the signer type
	FlagSignerType = FlagPrefixEvnode + "signer.signer_type"
	// FlagSignerPath is a flag for specifying the signer path
	FlagSignerPath = FlagPrefixEvnode + "signer.signer_path"
	// FlagSignerKmsProvider is a flag for specifying the signer KMS provider
	FlagSignerKmsProvider = FlagPrefixEvnode + "signer.kms.provider"
	// FlagSignerKmsAwsKeyID is a flag for specifying the AWS KMS key ID
	FlagSignerKmsAwsKeyID = FlagPrefixEvnode + "signer.kms.aws.key_id"
	// FlagSignerKmsAwsRegion is a flag for specifying the AWS KMS region
	FlagSignerKmsAwsRegion = FlagPrefixEvnode + "signer.kms.aws.region"
	// FlagSignerKmsAwsProfile is a flag for specifying the AWS profile
	FlagSignerKmsAwsProfile = FlagPrefixEvnode + "signer.kms.aws.profile"
	// FlagSignerKmsAwsTimeout is a flag for specifying the AWS KMS sign timeout
	FlagSignerKmsAwsTimeout = FlagPrefixEvnode + "signer.kms.aws.timeout"
	// FlagSignerKmsAwsMaxRetries is a flag for specifying the AWS KMS sign max retries
	FlagSignerKmsAwsMaxRetries = FlagPrefixEvnode + "signer.kms.aws.max_retries"
	// FlagSignerKmsGcpKeyName is a flag for specifying the GCP KMS key version resource name
	FlagSignerKmsGcpKeyName = FlagPrefixEvnode + "signer.kms.gcp.key_name"
	// FlagSignerKmsGcpCredentialsFile is a flag for specifying GCP credentials JSON file
	FlagSignerKmsGcpCredentialsFile = FlagPrefixEvnode + "signer.kms.gcp.credentials_file"
	// FlagSignerKmsGcpTimeout is a flag for specifying the GCP KMS sign timeout
	FlagSignerKmsGcpTimeout = FlagPrefixEvnode + "signer.kms.gcp.timeout"
	// FlagSignerKmsGcpMaxRetries is a flag for specifying the GCP KMS sign max retries
	FlagSignerKmsGcpMaxRetries = FlagPrefixEvnode + "signer.kms.gcp.max_retries"

	// FlagSignerPassphraseFile is a flag for specifying the file containing the signer passphrase
	FlagSignerPassphraseFile = FlagPrefixEvnode + "signer.passphrase_file"

	// RPC configuration flags

	// FlagRPCAddress is a flag for specifying the RPC server address
	FlagRPCAddress = FlagPrefixEvnode + "rpc.address"
	// FlagRPCEnableDAVisualization is a flag for enabling DA visualization endpoints
	FlagRPCEnableDAVisualization = FlagPrefixEvnode + "rpc.enable_da_visualization"

	// Raft configuration flags

	// FlagRaftEnable is a flag for enabling Raft consensus
	FlagRaftEnable = FlagPrefixEvnode + "raft.enable"
	// FlagRaftNodeID is a flag for specifying the Raft node ID
	FlagRaftNodeID = FlagPrefixEvnode + "raft.node_id"
	// FlagRaftAddr is a flag for specifying the Raft communication address
	FlagRaftAddr = FlagPrefixEvnode + "raft.raft_addr"
	// FlagRaftDir is a flag for specifying the Raft data directory
	FlagRaftDir = FlagPrefixEvnode + "raft.raft_dir"
	// FlagRaftBootstrap is a flag for bootstrapping a new Raft cluster
	FlagRaftBootstrap = FlagPrefixEvnode + "raft.bootstrap"
	// FlagRaftPeers is a flag for specifying Raft peer addresses
	FlagRaftPeers = FlagPrefixEvnode + "raft.peers"
	// FlagRaftSnapCount is a flag for specifying how many snapshot files to retain on disk
	FlagRaftSnapCount = FlagPrefixEvnode + "raft.snap_count"
	// FlagRaftSendTimeout max time to wait for a message to be sent to a peer
	FlagRaftSendTimeout = FlagPrefixEvnode + "raft.send_timeout"
	// FlagRaftHeartbeatTimeout is a flag for specifying heartbeat timeout
	FlagRaftHeartbeatTimeout = FlagPrefixEvnode + "raft.heartbeat_timeout"
	// FlagRaftLeaderLeaseTimeout is a flag for specifying leader lease timeout
	FlagRaftLeaderLeaseTimeout = FlagPrefixEvnode + "raft.leader_lease_timeout"
	// FlagRaftElectionTimeout is the flag for the raft election timeout.
	FlagRaftElectionTimeout = FlagPrefixEvnode + "raft.election_timeout"
	// FlagRaftSnapshotThreshold is the flag for the raft snapshot threshold.
	FlagRaftSnapshotThreshold = FlagPrefixEvnode + "raft.snapshot_threshold"
	// FlagRaftTrailingLogs is the flag for the number of trailing logs after a snapshot.
	FlagRaftTrailingLogs = FlagPrefixEvnode + "raft.trailing_logs"
	// Pruning configuration flags
	FlagPruningMode       = FlagPrefixEvnode + "pruning.pruning_mode"
	FlagPruningKeepRecent = FlagPrefixEvnode + "pruning.pruning_keep_recent"
	FlagPruningInterval   = FlagPrefixEvnode + "pruning.pruning_interval"
)

// Config stores Rollkit configuration.
type Config struct {
	RootDir    string `mapstructure:"-" yaml:"-" comment:"Root directory where rollkit files are located"`
	ClearCache bool   `mapstructure:"-" yaml:"-" comment:"Clear the cache"`

	// Base configuration
	DBPath string `mapstructure:"db_path" yaml:"db_path" comment:"Path inside the root directory where the database is located"`
	// P2P configuration
	P2P P2PConfig `mapstructure:"p2p" yaml:"p2p"`

	// Node specific configuration
	Node NodeConfig `mapstructure:"node" yaml:"node"`

	// Data availability configuration
	DA DAConfig `mapstructure:"da" yaml:"da"`

	// RPC configuration
	RPC RPCConfig `mapstructure:"rpc" yaml:"rpc"`

	// Instrumentation configuration
	Instrumentation *InstrumentationConfig `mapstructure:"instrumentation" yaml:"instrumentation"`

	// Logging configuration
	Log LogConfig `mapstructure:"log" yaml:"log"`

	// Remote signer configuration
	Signer SignerConfig `mapstructure:"signer" yaml:"signer"`

	// Raft consensus configuration
	Raft RaftConfig `mapstructure:"raft" yaml:"raft"`

	// Pruning configuration
	Pruning PruningConfig `mapstructure:"pruning" yaml:"pruning"`
}

// DAConfig contains all Data Availability configuration parameters
type DAConfig struct {
	StartHeight uint64 `mapstructure:"start_height" yaml:"-" comment:"Force DA retrieval to start from a specific height (0 for default)"`

	Address                  string          `mapstructure:"address" yaml:"address" comment:"Address of the data availability layer service (host:port). This is the endpoint where Rollkit will connect to submit and retrieve data."`
	AuthToken                string          `mapstructure:"auth_token" yaml:"auth_token" comment:"Authentication token for the data availability layer service. Required if the DA service needs authentication."` //nolint:gosec // this is ok.
	SubmitOptions            string          `mapstructure:"submit_options" yaml:"submit_options" comment:"Additional options passed to the DA layer when submitting data. Format depends on the specific DA implementation being used."`
	SigningAddresses         []string        `mapstructure:"signing_addresses" yaml:"signing_addresses" comment:"List of addresses to use for DA submissions. When multiple addresses are provided, they will be used in round-robin fashion to prevent sequence mismatches. Useful for high-throughput chains."`
	Namespace                string          `mapstructure:"namespace" yaml:"namespace" comment:"Namespace ID used when submitting blobs to the DA layer. When a DataNamespace is provided, only the header is sent to this namespace."`
	DataNamespace            string          `mapstructure:"data_namespace" yaml:"data_namespace" comment:"Namespace ID for submitting data to DA layer. Use this to speed-up light clients."`
	ForcedInclusionNamespace string          `mapstructure:"forced_inclusion_namespace" yaml:"forced_inclusion_namespace" comment:"Namespace ID for forced inclusion transactions on the DA layer."`
	BlockTime                DurationWrapper `mapstructure:"block_time" yaml:"block_time" comment:"Average block time of the DA chain (duration). Determines frequency of DA layer syncing, maximum backoff time for retries, and is multiplied by MempoolTTL to calculate transaction expiration. Examples: \"15s\", \"30s\", \"1m\", \"2m30s\", \"10m\"."`
	MempoolTTL               uint64          `mapstructure:"mempool_ttl" yaml:"mempool_ttl" comment:"Number of DA blocks after which a transaction is considered expired and dropped from the mempool. Controls retry backoff timing."`
	MaxSubmitAttempts        int             `mapstructure:"max_submit_attempts" yaml:"max_submit_attempts" comment:"Maximum number of attempts to submit data to the DA layer before giving up. Higher values provide more resilience but can delay error reporting."`
	RequestTimeout           DurationWrapper `mapstructure:"request_timeout" yaml:"request_timeout" comment:"Timeout for requests to DA layer"`

	// Batching strategy configuration
	BatchingStrategy   string          `mapstructure:"batching_strategy" yaml:"batching_strategy" comment:"Batching strategy for DA submissions. Options: 'immediate' (submit as soon as items are available), 'size' (wait until batch reaches size threshold), 'time' (wait for time interval), 'adaptive' (balance between size and time). Default: 'time'."`
	BatchSizeThreshold float64         `mapstructure:"batch_size_threshold" yaml:"batch_size_threshold" comment:"Minimum blob size threshold (as fraction of max blob size, 0.0-1.0) before submitting. Only applies to 'size' and 'adaptive' strategies. Example: 0.8 means wait until batch is 80% full. Default: 0.8."`
	BatchMaxDelay      DurationWrapper `mapstructure:"batch_max_delay" yaml:"batch_max_delay" comment:"Maximum time to wait before submitting a batch regardless of size. Applies to 'time' and 'adaptive' strategies. Lower values reduce latency but may increase costs. Examples: \"6s\", \"12s\", \"30s\". Default: DA BlockTime."`
	BatchMinItems      uint64          `mapstructure:"batch_min_items" yaml:"batch_min_items" comment:"Minimum number of items (headers or data) to accumulate before considering submission. Helps avoid submitting single items when more are expected soon. Default: 1."`

	TxSigner DATxSignerConfig `mapstructure:"tx_signer" yaml:"tx_signer"`
}

// DATxSignerConfig configures signing PayForBlobs locally and broadcasting
// them over consensus gRPC, for DA endpoints that cannot sign (e.g. da-proxy).
// Reads still go through da.address.
type DATxSignerConfig struct {
	Enabled        bool   `mapstructure:"enabled" yaml:"enabled" comment:"Sign blob submissions locally and broadcast them over consensus gRPC instead of calling blob.Submit on the DA node."`
	GRPCAddress    string `mapstructure:"grpc_address" yaml:"grpc_address" comment:"Consensus gRPC endpoint (host:port), e.g. grpc.da-proxy-lb.celestia-mocha.com:443."`
	GRPCAuthToken  string `mapstructure:"grpc_auth_token" yaml:"grpc_auth_token" comment:"Token sent as x-token metadata on consensus gRPC calls. Defaults to da.auth_token."` //nolint:gosec // user-provided config
	GRPCTLS        bool   `mapstructure:"grpc_tls" yaml:"grpc_tls" comment:"Use TLS for the consensus gRPC connection."`
	KeyringDir     string `mapstructure:"keyring_dir" yaml:"keyring_dir" comment:"Keyring directory; relative paths are resolved against the node home."`
	KeyringBackend string `mapstructure:"keyring_backend" yaml:"keyring_backend" comment:"Keyring backend: file, or test for local development."`
	KeyName        string `mapstructure:"key_name" yaml:"key_name" comment:"Keyring key that signs and pays for blobs."`
	PassphraseFile string `mapstructure:"passphrase_file" yaml:"passphrase_file" comment:"File holding the file-backend keyring passphrase. EVNODE_DA_KEYRING_PASSPHRASE takes precedence."`
}

// GRPCAuthTokenOrDefault returns the consensus gRPC token, falling back to
// da.auth_token since one da-proxy token covers both endpoints.
func (d *DAConfig) GRPCAuthTokenOrDefault() string {
	if d.TxSigner.GRPCAuthToken != "" {
		return d.TxSigner.GRPCAuthToken
	}
	return d.AuthToken
}

func (d *DAConfig) validateTxSigner() error {
	t := d.TxSigner
	if !t.Enabled {
		return nil
	}
	if d.Address == "" {
		return errors.New("evnode.da.address is required when tx_signer is enabled")
	}
	if t.GRPCAddress == "" {
		return errors.New("evnode.da.tx_signer.grpc_address is required when tx_signer is enabled")
	}
	if t.KeyName == "" {
		return errors.New("evnode.da.tx_signer.key_name is required when tx_signer is enabled")
	}
	if t.KeyringBackend != "file" && t.KeyringBackend != "test" {
		return fmt.Errorf("evnode.da.tx_signer.keyring_backend must be file or test, got %q", t.KeyringBackend)
	}
	if len(d.SigningAddresses) > 0 {
		return errors.New("evnode.da.signing_addresses must be empty when tx_signer is enabled")
	}
	return nil
}

// DAEnabled reports whether this node should initialize a DA client. Full
// followers without a DA address sync exclusively through P2P.
func (c Config) DAEnabled() bool {
	return !c.Node.Light && c.GetDAAddress() != ""
}

// GetDAAddress returns the configured DA address without surrounding whitespace.
func (c Config) GetDAAddress() string {
	return strings.TrimSpace(c.DA.Address)
}

// GetNamespace returns the namespace for header submissions.
func (d *DAConfig) GetNamespace() string {
	return d.Namespace
}

// GetDataNamespace returns the namespace for data submissions, falling back to the header namespace if not set
func (d *DAConfig) GetDataNamespace() string {
	if d.DataNamespace != "" {
		return d.DataNamespace
	}

	return d.GetNamespace()
}

// GetForcedInclusionNamespace returns the namespace for forced inclusion transactions
func (d *DAConfig) GetForcedInclusionNamespace() string {
	return d.ForcedInclusionNamespace
}

// NodeConfig contains all Rollkit specific configuration parameters
type NodeConfig struct {
	// Node mode configuration
	Aggregator     bool `mapstructure:"aggregator" yaml:"aggregator" comment:"Run node in aggregator mode"`
	Promotable     bool `mapstructure:"promotable" yaml:"promotable" comment:"Run full node with aggregator dependencies available for future promotion"`
	BasedSequencer bool `mapstructure:"based_sequencer" yaml:"based_sequencer" comment:"Run node with based sequencer (fetches transactions only from DA forced inclusion namespace). Requires aggregator mode to be enabled."`
	Light          bool `mapstructure:"light" yaml:"light" comment:"Run node in light mode"`

	// Block management configuration
	BlockTime                DurationWrapper `mapstructure:"block_time" yaml:"block_time" comment:"Block time (duration). Examples: \"500ms\", \"1s\", \"5s\", \"1m\", \"2m30s\", \"10m\"."`
	MaxPendingHeadersAndData uint64          `mapstructure:"max_pending_headers_and_data" yaml:"max_pending_headers_and_data" comment:"Maximum number of headers or data pending DA submission. When this limit is reached, the aggregator pauses block production until some headers or data are confirmed. Use 0 for no limit."`
	LazyMode                 bool            `mapstructure:"lazy_mode" yaml:"lazy_mode" comment:"Enables lazy aggregation mode, where blocks are only produced when transactions are available or after LazyBlockTime. Optimizes resources by avoiding empty block creation during periods of inactivity."`
	LazyBlockInterval        DurationWrapper `mapstructure:"lazy_block_interval" yaml:"lazy_block_interval" comment:"Maximum interval between blocks in lazy aggregation mode (LazyAggregator). Ensures blocks are produced periodically even without transactions to keep the chain active. Generally larger than BlockTime."`
	ScrapeInterval           DurationWrapper `mapstructure:"scrape_interval" yaml:"scrape_interval" comment:"Interval at which the reaper polls the execution layer for new transactions. Lower values reduce transaction detection latency but increase RPC load. Examples: \"250ms\", \"500ms\", \"1s\"."`
	CatchupTimeout           DurationWrapper `mapstructure:"catchup_timeout" yaml:"catchup_timeout" comment:"When set, the aggregator recovers before producing blocks. With configured P2P peers, failure to establish continuity before this timeout is fatal. Requires aggregator mode."`

	// Readiness / health configuration
	ReadinessWindowSeconds   uint64 `mapstructure:"readiness_window_seconds" yaml:"readiness_window_seconds" comment:"Time window in seconds used to calculate ReadinessMaxBlocksBehind based on block time. Default: 15 seconds."`
	ReadinessMaxBlocksBehind uint64 `mapstructure:"readiness_max_blocks_behind" yaml:"readiness_max_blocks_behind" comment:"How many blocks behind best-known head the node can be and still be considered ready. 0 means must be exactly at head."`
}

// LogConfig contains all logging configuration parameters
type LogConfig struct {
	Level  string `mapstructure:"level" yaml:"level" comment:"Log level (debug, info, warn, error)"`
	Format string `mapstructure:"format" yaml:"format" comment:"Log format (text, json)"`
	Trace  bool   `mapstructure:"trace" yaml:"trace" comment:"Enable stack traces in error logs"`
}

// P2PConfig contains all peer-to-peer networking configuration parameters
type P2PConfig struct {
	ListenAddress          string `mapstructure:"listen_address" yaml:"listen_address" comment:"Address to listen for incoming connections (host:port)"`
	Peers                  string `mapstructure:"peers" yaml:"peers" comment:"Comma-separated list of peers to connect to"`
	BlockedPeers           string `mapstructure:"blocked_peers" yaml:"blocked_peers" comment:"Comma-separated list of peer IDs to block from connecting"`
	AllowedPeers           string `mapstructure:"allowed_peers" yaml:"allowed_peers" comment:"Comma-separated list of peer IDs to allow connections from"`
	DisableConnectionGater bool   `mapstructure:"disable_connection_gater" yaml:"disable_connection_gater" comment:"Disable the P2P connection gater (no-op mode). Set to false to enforce peer filtering when experiencing P2P flooding."`
}

// SignerConfig contains all signer configuration parameters
type SignerConfig struct {
	SignerType string          `mapstructure:"signer_type" yaml:"signer_type" comment:"Type of remote signer to use (file, kms)"`
	SignerPath string          `mapstructure:"signer_path" yaml:"signer_path" comment:"Path to the signer file or address"`
	KMS        SignerKMSConfig `mapstructure:"kms" yaml:"kms"`
}

// SignerKMSConfig contains cloud-KMS signer configuration.
type SignerKMSConfig struct {
	Provider string             `mapstructure:"provider" yaml:"provider" comment:"KMS provider for signer type 'kms' (aws, gcp)"`
	AWS      SignerAWSKMSConfig `mapstructure:"aws" yaml:"aws"`
	GCP      SignerGCPKMSConfig `mapstructure:"gcp" yaml:"gcp"`
}

// SignerAWSKMSConfig contains AWS KMS signer configuration.
type SignerAWSKMSConfig struct {
	KeyID      string          `mapstructure:"key_id" yaml:"key_id" comment:"AWS KMS Key ID or ARN"`
	Region     string          `mapstructure:"region" yaml:"region" comment:"AWS region"`
	Profile    string          `mapstructure:"profile" yaml:"profile" comment:"AWS profile"`
	Timeout    DurationWrapper `mapstructure:"timeout" yaml:"timeout" comment:"Timeout for individual AWS KMS Sign requests"`
	MaxRetries int             `mapstructure:"max_retries" yaml:"max_retries" comment:"Maximum number of retries for transient AWS KMS failures"`
}

// SignerGCPKMSConfig contains GCP KMS signer configuration.
type SignerGCPKMSConfig struct {
	KeyName         string          `mapstructure:"key_name" yaml:"key_name" comment:"GCP KMS CryptoKeyVersion resource name"`
	CredentialsFile string          `mapstructure:"credentials_file" yaml:"credentials_file" comment:"Path to Google credentials JSON (optional; defaults to ADC)"`
	Timeout         DurationWrapper `mapstructure:"timeout" yaml:"timeout" comment:"Timeout for individual GCP KMS Sign requests"`
	MaxRetries      int             `mapstructure:"max_retries" yaml:"max_retries" comment:"Maximum number of retries for transient GCP KMS failures"`
}

// RPCConfig contains all RPC server configuration parameters
type RPCConfig struct {
	Address               string `mapstructure:"address" yaml:"address" comment:"Address to bind the RPC server to (host:port). Default: 127.0.0.1:7331"`
	EnableDAVisualization bool   `mapstructure:"enable_da_visualization" yaml:"enable_da_visualization" comment:"Enable DA visualization endpoints for monitoring blob submissions. Default: false"`
}

const (
	PruningModeDisabled = "disabled"
	PruningModeAll      = "all"
	PruningModeMetadata = "metadata"
)

// PruningConfig contains all pruning configuration parameters
type PruningConfig struct {
	Mode       string          `mapstructure:"pruning_mode" yaml:"pruning_mode" comment:"Pruning mode for stored block data and block metadata. Options: 'all' (prune all but recent blocks and their metatadas), 'metadata' (prune all but recent blocks metadatas), 'disabled' (keep all blocks and blocks metadata). Default: 'disabled'."`
	KeepRecent uint64          `mapstructure:"pruning_keep_recent" yaml:"pruning_keep_recent" comment:"Number of most recent blocks/blocks metadata to retain when pruning is enabled. Must be > 0."`
	Interval   DurationWrapper `mapstructure:"pruning_interval" yaml:"pruning_interval" comment:"Run pruning every N minutes. Examples: \"5m\", \"10m\", \"24h\"."`
}

// IsPruningEnabled returns true if pruning is enabled (i.e. pruning mode is not 'disabled')
func (c PruningConfig) IsPruningEnabled() bool {
	return c.Mode != PruningModeDisabled && len(c.Mode) > 0
}

// Validate pruning configuration
func (c PruningConfig) Validate(blockTime time.Duration) error {
	if c.Mode != PruningModeDisabled && c.Mode != PruningModeAll && c.Mode != PruningModeMetadata {
		return fmt.Errorf("invalid pruning mode: %s; must be one of '%s', '%s', or '%s'", c.Mode, PruningModeDisabled, PruningModeAll, PruningModeMetadata)
	}

	if c.Mode == PruningModeDisabled {
		return nil
	}

	if c.Interval.Duration == 0 {
		return fmt.Errorf("pruning_interval must be >= 1s when pruning is enabled")
	}

	if c.Interval.Duration < blockTime {
		return fmt.Errorf("pruning_interval (%v) must be greater than or equal to block time (%v)", c.Interval.Duration, blockTime)
	}

	if c.KeepRecent == 0 {
		return fmt.Errorf("pruning_keep_recent must be > 0 when pruning is enabled; use pruning_enabled=false to keep all blocks")
	}

	return nil
}

// RaftConfig contains all Raft consensus configuration parameters
type RaftConfig struct {
	Enable             bool          `mapstructure:"enable" yaml:"enable" comment:"Enable Raft consensus for leader election and state replication"`
	NodeID             string        `mapstructure:"node_id" yaml:"node_id" comment:"Unique identifier for this node in the Raft cluster"`
	RaftAddr           string        `mapstructure:"raft_addr" yaml:"raft_addr" comment:"Address for Raft communication (host:port)"`
	RaftDir            string        `mapstructure:"raft_dir" yaml:"raft_dir" comment:"Directory for Raft logs and snapshots"`
	Bootstrap          bool          `mapstructure:"bootstrap" yaml:"bootstrap" comment:"Bootstrap a new static Raft cluster during initial bring-up"`
	Peers              string        `mapstructure:"peers" yaml:"peers" comment:"Comma-separated list of peer Raft addresses (nodeID@host:port)"`
	SnapCount          uint64        `mapstructure:"snap_count" yaml:"snap_count" comment:"Number of snapshot files to retain on disk"`
	SendTimeout        time.Duration `mapstructure:"send_timeout" yaml:"send_timeout" comment:"Max duration to wait for a message to be sent to a peer"`
	HeartbeatTimeout   time.Duration `mapstructure:"heartbeat_timeout" yaml:"heartbeat_timeout" comment:"Time between leader heartbeats to followers"`
	LeaderLeaseTimeout time.Duration `mapstructure:"leader_lease_timeout" yaml:"leader_lease_timeout" comment:"Duration of the leader lease"`
	ElectionTimeout    time.Duration `mapstructure:"election_timeout" yaml:"election_timeout" comment:"Time a candidate waits for votes before restarting election; must be >= heartbeat_timeout"`
	SnapshotThreshold  uint64        `mapstructure:"snapshot_threshold" yaml:"snapshot_threshold" comment:"Number of outstanding log entries that trigger an automatic snapshot"`
	TrailingLogs       uint64        `mapstructure:"trailing_logs" yaml:"trailing_logs" comment:"Number of log entries to retain after a snapshot (controls rejoin catch-up cost)"`
}

func (c RaftConfig) Validate() error {
	if !c.Enable {
		return nil
	}
	var multiErr error
	if c.NodeID == "" {
		multiErr = fmt.Errorf("node ID is required")
	}
	if c.RaftAddr == "" {
		multiErr = errors.Join(multiErr, fmt.Errorf("raft address is required"))
	}
	if c.RaftDir == "" {
		multiErr = errors.Join(multiErr, fmt.Errorf("raft directory is required"))
	}

	if c.SendTimeout <= 0 {
		multiErr = errors.Join(multiErr, fmt.Errorf("send timeout must be positive"))
	}

	if c.HeartbeatTimeout <= 0 {
		multiErr = errors.Join(multiErr, fmt.Errorf("heartbeat timeout must be positive"))
	}

	if c.LeaderLeaseTimeout <= 0 {
		multiErr = errors.Join(multiErr, fmt.Errorf("leader lease timeout must be positive"))
	}

	if c.ElectionTimeout < 0 {
		multiErr = errors.Join(multiErr, fmt.Errorf("election timeout (%v) must be >= 0", c.ElectionTimeout))
	} else if c.ElectionTimeout > 0 && c.ElectionTimeout < c.HeartbeatTimeout {
		multiErr = errors.Join(multiErr, fmt.Errorf("election timeout (%v) must be >= heartbeat timeout (%v)", c.ElectionTimeout, c.HeartbeatTimeout))
	}

	return multiErr
}

// Validate validates the config and ensures that the root directory exists.
// It creates the directory if it does not exist.
func (c *Config) Validate() error {
	if c.Signer.SignerType == "kms" {
		switch c.Signer.KMS.Provider {
		case "aws":
			if c.Signer.KMS.AWS.KeyID == "" {
				return errors.New("evnode.signer.kms.aws.key_id is required when signer.signer_type is kms and signer.kms.provider is aws")
			}
			if c.Signer.KMS.AWS.Timeout.Duration <= 0 {
				return errors.New("evnode.signer.kms.aws.timeout must be positive")
			}
			if c.Signer.KMS.AWS.MaxRetries < 0 {
				return errors.New("evnode.signer.kms.aws.max_retries must be non-negative")
			}
		case "gcp":
			if c.Signer.KMS.GCP.KeyName == "" {
				return errors.New("evnode.signer.kms.gcp.key_name is required when signer.signer_type is kms and signer.kms.provider is gcp")
			}
			if c.Signer.KMS.GCP.Timeout.Duration <= 0 {
				return errors.New("evnode.signer.kms.gcp.timeout must be positive")
			}
			if c.Signer.KMS.GCP.MaxRetries < 0 {
				return errors.New("evnode.signer.kms.gcp.max_retries must be non-negative")
			}
		default:
			return errors.New("evnode.signer.kms.provider must be one of: aws, gcp when signer.signer_type is kms")
		}
	}
	if c.RootDir == "" {
		return fmt.Errorf("root directory cannot be empty")
	}

	fullDir := filepath.Dir(c.ConfigPath())
	if err := os.MkdirAll(fullDir, 0o750); err != nil {
		return fmt.Errorf("could not create directory %q: %w", fullDir, err)
	}

	if c.Node.Promotable && c.Node.BasedSequencer {
		return fmt.Errorf("promotable mode cannot be combined with based sequencer mode")
	}

	if c.Node.Promotable && c.Node.Light {
		return fmt.Errorf("promotable mode cannot be combined with light node mode")
	}

	// Promotable mode and Raft both control leader/follower role switching.
	// Combining them needs a composed elector that gates Raft leadership on the execution-selected proposer.
	if c.Node.Promotable && c.Raft.Enable {
		return fmt.Errorf("promotable mode cannot be combined with Raft consensus")
	}

	// Validate based sequencer requires aggregator mode
	if c.Node.BasedSequencer && !c.Node.Aggregator {
		return fmt.Errorf("based sequencer mode requires aggregator mode to be enabled")
	}

	// Validate catchup timeout requires aggregator mode
	if c.Node.CatchupTimeout.Duration > 0 && !c.Node.Aggregator {
		return fmt.Errorf("catchup timeout requires aggregator mode to be enabled")
	}

	if c.Node.CatchupTimeout.Duration > 0 && c.Raft.Enable {
		return fmt.Errorf("catchup timeout and Raft consensus are mutually exclusive; disable one of them")
	}

	// Validate namespaces
	if err := validateNamespace(c.DA.GetNamespace()); err != nil {
		return fmt.Errorf("could not validate namespace (%s): %w", c.DA.GetNamespace(), err)
	}

	if len(c.DA.GetDataNamespace()) > 0 {
		if err := validateNamespace(c.DA.GetDataNamespace()); err != nil {
			return fmt.Errorf("could not validate data namespace (%s): %w", c.DA.GetDataNamespace(), err)
		}
	}

	if len(c.DA.GetForcedInclusionNamespace()) > 0 {
		if err := validateNamespace(c.DA.GetForcedInclusionNamespace()); err != nil {
			return fmt.Errorf("could not validate forced inclusion namespace (%s): %w", c.DA.GetForcedInclusionNamespace(), err)
		}
	}

	// Validate lazy mode configuration
	if c.Node.LazyMode && c.Node.LazyBlockInterval.Duration <= c.Node.BlockTime.Duration {
		return fmt.Errorf("LazyBlockInterval (%v) must be greater than BlockTime (%v) in lazy mode",
			c.Node.LazyBlockInterval.Duration, c.Node.BlockTime.Duration)
	}

	if err := c.DA.validateTxSigner(); err != nil {
		return err
	}

	if err := c.Raft.Validate(); err != nil {
		return err
	}

	if err := c.Pruning.Validate(c.Node.BlockTime.Duration); err != nil {
		return err
	}

	return nil
}

func validateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	ns := datypes.NamespaceFromString(namespace)
	if _, err := share.NewNamespaceFromBytes(ns.Bytes()); err != nil {
		return fmt.Errorf("could not validate namespace (%s): %w", namespace, err)
	}
	return nil
}

// ConfigPath returns the path to the configuration file.
func (c *Config) ConfigPath() string {
	return filepath.Join(c.RootDir, AppConfigDir, ConfigName)
}

// AddGlobalFlags registers the basic configuration flags that are common across applications.
// This includes logging configuration and root directory settings.
// It should be used in apps that do not already define their logger and home flag.
func AddGlobalFlags(cmd *cobra.Command, defaultHome string) {
	def := DefaultConfig()

	cmd.PersistentFlags().String(FlagLogLevel, def.Log.Level, "Set the log level (debug, info, warn, error)")
	cmd.PersistentFlags().String(FlagLogFormat, def.Log.Format, "Set the log format (text, json)")
	cmd.PersistentFlags().Bool(FlagLogTrace, def.Log.Trace, "Enable stack traces in error logs")
	cmd.PersistentFlags().String(FlagRootDir, DefaultRootDirWithName(defaultHome), "Root directory for application data")
}

// AddFlags adds Evolve specific configuration options to cobra Command.
func AddFlags(cmd *cobra.Command) {
	def := DefaultConfig()

	// Set normalization function to support both flag prefixes
	cmd.Flags().SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		if strings.HasPrefix(name, FlagPrefixRollkit) {
			return pflag.NormalizedName(strings.Replace(name, FlagPrefixRollkit, FlagPrefixEvnode, 1))
		}

		return pflag.NormalizedName(name)
	})

	// Add base flags
	cmd.Flags().String(FlagDBPath, def.DBPath, "path for node database")
	cmd.Flags().Bool(FlagClearCache, def.ClearCache, "clear the cache")

	// Node configuration flags
	cmd.Flags().Bool(FlagAggregator, def.Node.Aggregator, "run node as an aggregator")
	cmd.Flags().Bool(FlagPromotable, def.Node.Promotable, "run full node with aggregator dependencies available for future promotion")
	cmd.Flags().Bool(FlagBasedSequencer, def.Node.BasedSequencer, "run node with based sequencer (requires aggregator mode)")
	cmd.Flags().Bool(FlagLight, def.Node.Light, "run node in light mode")
	cmd.Flags().Duration(FlagBlockTime, def.Node.BlockTime.Duration, "block time (for aggregator mode)")
	cmd.Flags().Bool(FlagLazyAggregator, def.Node.LazyMode, "produce blocks only when transactions are available or after lazy block time")
	cmd.Flags().Uint64(FlagMaxPendingHeadersAndData, def.Node.MaxPendingHeadersAndData, "maximum headers or data pending DA confirmation before pausing block production (0 for no limit)")
	cmd.Flags().Duration(FlagLazyBlockTime, def.Node.LazyBlockInterval.Duration, "maximum interval between blocks in lazy aggregation mode")
	cmd.Flags().Uint64(FlagReadinessWindowSeconds, def.Node.ReadinessWindowSeconds, "time window in seconds for calculating readiness threshold based on block time (default: 15s)")
	cmd.Flags().Uint64(FlagReadinessMaxBlocksBehind, def.Node.ReadinessMaxBlocksBehind, "how many blocks behind best-known head the node can be and still be considered ready (0 = must be at head)")
	cmd.Flags().Duration(FlagScrapeInterval, def.Node.ScrapeInterval.Duration, "interval at which the reaper polls the execution layer for new transactions")
	cmd.Flags().Duration(FlagCatchupTimeout, def.Node.CatchupTimeout.Duration, "recover before producing blocks; configured P2P recovery is fatal if continuity is not established before this timeout. Requires aggregator mode.")

	// Data Availability configuration flags
	cmd.Flags().String(FlagDAAddress, def.DA.Address, "DA address (host:port)")
	cmd.Flags().String(FlagDAAuthToken, def.DA.AuthToken, "DA auth token")
	cmd.Flags().Duration(FlagDABlockTime, def.DA.BlockTime.Duration, "DA chain block time (for syncing)")
	cmd.Flags().String(FlagDANamespace, def.DA.Namespace, "DA namespace for header (or blob) submissions")
	cmd.Flags().String(FlagDADataNamespace, def.DA.DataNamespace, "DA namespace for data submissions")
	cmd.Flags().String(FlagDAForcedInclusionNamespace, def.DA.ForcedInclusionNamespace, "DA namespace for forced inclusion transactions")
	cmd.Flags().String(FlagDASubmitOptions, def.DA.SubmitOptions, "DA submit options")
	cmd.Flags().StringSlice(FlagDASigningAddresses, def.DA.SigningAddresses, "Comma-separated list of addresses for DA submissions (used in round-robin)")
	cmd.Flags().Uint64(FlagDAMempoolTTL, def.DA.MempoolTTL, "number of DA blocks until transaction is dropped from the mempool")
	cmd.Flags().Int(FlagDAMaxSubmitAttempts, def.DA.MaxSubmitAttempts, "maximum number of attempts to submit data to the DA layer before giving up")
	cmd.Flags().Duration(FlagDARequestTimeout, def.DA.RequestTimeout.Duration, "per-request timeout when interacting with the DA layer")
	cmd.Flags().String(FlagDABatchingStrategy, def.DA.BatchingStrategy, "batching strategy for DA submissions (immediate, size, time, adaptive)")
	cmd.Flags().Float64(FlagDABatchSizeThreshold, def.DA.BatchSizeThreshold, "batch size threshold as fraction of max blob size (0.0-1.0)")
	cmd.Flags().Duration(FlagDABatchMaxDelay, def.DA.BatchMaxDelay.Duration, "maximum time to wait before submitting a batch")
	cmd.Flags().Uint64(FlagDABatchMinItems, def.DA.BatchMinItems, "minimum number of items to accumulate before submission")
	cmd.Flags().Uint64(FlagDAStartHeight, def.DA.StartHeight, "force DA retrieval to start from a specific height (0 for disabled)")
	cmd.Flags().MarkHidden(FlagDAStartHeight)
	cmd.Flags().Bool(FlagDATxSignerEnabled, def.DA.TxSigner.Enabled, "sign blob submissions locally and broadcast them over consensus gRPC")
	cmd.Flags().String(FlagDATxSignerGRPCAddress, def.DA.TxSigner.GRPCAddress, "consensus gRPC endpoint (host:port) for signed blob submission")
	cmd.Flags().String(FlagDATxSignerGRPCAuthToken, def.DA.TxSigner.GRPCAuthToken, "x-token for consensus gRPC (defaults to the DA auth token)")
	cmd.Flags().Bool(FlagDATxSignerGRPCTLS, def.DA.TxSigner.GRPCTLS, "use TLS for consensus gRPC")
	cmd.Flags().String(FlagDATxSignerKeyringDir, def.DA.TxSigner.KeyringDir, "keyring directory, relative to home unless absolute")
	cmd.Flags().String(FlagDATxSignerKeyringBackend, def.DA.TxSigner.KeyringBackend, "keyring backend (file, test)")
	cmd.Flags().String(FlagDATxSignerKeyName, def.DA.TxSigner.KeyName, "keyring key that signs and pays for blobs")
	cmd.Flags().String(FlagDATxSignerPassphraseFile, def.DA.TxSigner.PassphraseFile, "file holding the keyring passphrase (file backend)")

	// P2P configuration flags
	cmd.Flags().String(FlagP2PListenAddress, def.P2P.ListenAddress, "P2P listen address (host:port)")
	cmd.Flags().String(FlagP2PPeers, def.P2P.Peers, "Comma separated list of seed nodes to connect to")
	cmd.Flags().String(FlagP2PBlockedPeers, def.P2P.BlockedPeers, "Comma separated list of nodes to ignore")
	cmd.Flags().String(FlagP2PAllowedPeers, def.P2P.AllowedPeers, "Comma separated list of nodes to whitelist")
	cmd.Flags().Bool(FlagP2PDisableConnectionGater, def.P2P.DisableConnectionGater, "Disable P2P connection gater (no-op mode); set to false to enforce peer filtering when experiencing P2P flooding")

	// RPC configuration flags
	cmd.Flags().String(FlagRPCAddress, def.RPC.Address, "RPC server address (host:port)")
	cmd.Flags().Bool(FlagRPCEnableDAVisualization, def.RPC.EnableDAVisualization, "enable DA visualization endpoints for monitoring blob submissions")

	// Instrumentation configuration flags
	instrDef := DefaultInstrumentationConfig()
	cmd.Flags().Bool(FlagPrometheus, instrDef.Prometheus, "enable Prometheus metrics")
	cmd.Flags().String(FlagPrometheusListenAddr, instrDef.PrometheusListenAddr, "Prometheus metrics listen address")
	cmd.Flags().Int(FlagMaxOpenConnections, instrDef.MaxOpenConnections, "maximum number of simultaneous connections for metrics")
	cmd.Flags().Bool(FlagPprof, instrDef.Pprof, "enable pprof HTTP endpoint")
	cmd.Flags().String(FlagPprofListenAddr, instrDef.PprofListenAddr, "pprof HTTP server listening address")
	cmd.Flags().Bool(FlagTracing, instrDef.Tracing, "enable OpenTelemetry tracing")
	cmd.Flags().String(FlagTracingEndpoint, instrDef.TracingEndpoint, "OTLP endpoint for traces (host:port)")
	cmd.Flags().String(FlagTracingServiceName, instrDef.TracingServiceName, "OpenTelemetry service.name")
	cmd.Flags().Float64(FlagTracingSampleRate, instrDef.TracingSampleRate, "trace sampling rate (0.0-1.0)")

	// Signer configuration flags
	cmd.Flags().String(FlagSignerType, def.Signer.SignerType, "type of signer to use (file, grpc, kms)")
	cmd.Flags().String(FlagSignerPath, def.Signer.SignerPath, "path to the signer file or address")
	cmd.Flags().String(FlagSignerKmsProvider, def.Signer.KMS.Provider, "KMS provider for signer type kms (aws, gcp)")
	cmd.Flags().String(FlagSignerKmsAwsKeyID, def.Signer.KMS.AWS.KeyID, "AWS KMS Key ID or ARN for signer.kms.provider=aws")
	cmd.Flags().String(FlagSignerKmsAwsRegion, def.Signer.KMS.AWS.Region, "AWS region for signer.kms.provider=aws")
	cmd.Flags().String(FlagSignerKmsAwsProfile, def.Signer.KMS.AWS.Profile, "AWS profile for signer.kms.provider=aws")
	cmd.Flags().Duration(FlagSignerKmsAwsTimeout, def.Signer.KMS.AWS.Timeout.Duration, "Timeout for individual AWS KMS Sign requests")
	cmd.Flags().Int(FlagSignerKmsAwsMaxRetries, def.Signer.KMS.AWS.MaxRetries, "Maximum number of retries for transient AWS KMS failures")
	cmd.Flags().String(FlagSignerKmsGcpKeyName, def.Signer.KMS.GCP.KeyName, "GCP KMS CryptoKeyVersion resource name for signer.kms.provider=gcp")
	cmd.Flags().String(FlagSignerKmsGcpCredentialsFile, def.Signer.KMS.GCP.CredentialsFile, "Path to Google credentials JSON for signer.kms.provider=gcp (optional)")
	cmd.Flags().Duration(FlagSignerKmsGcpTimeout, def.Signer.KMS.GCP.Timeout.Duration, "Timeout for individual GCP KMS Sign requests")
	cmd.Flags().Int(FlagSignerKmsGcpMaxRetries, def.Signer.KMS.GCP.MaxRetries, "Maximum number of retries for transient GCP KMS failures")
	cmd.Flags().String(FlagSignerPassphraseFile, "", "path to file containing the signer passphrase (required for file signer when aggregator or promotable mode is enabled)")

	cmd.MarkFlagsMutuallyExclusive(FlagLight, FlagAggregator)
	cmd.MarkFlagsMutuallyExclusive(FlagBasedSequencer, FlagPromotable)
	cmd.MarkFlagsMutuallyExclusive(FlagLight, FlagPromotable)

	// Raft configuration flags
	cmd.Flags().Bool(FlagRaftEnable, def.Raft.Enable, "enable Raft consensus for leader election and state replication")
	cmd.Flags().String(FlagRaftNodeID, def.Raft.NodeID, "unique identifier for this node in the Raft cluster")
	cmd.Flags().String(FlagRaftAddr, def.Raft.RaftAddr, "address for Raft communication (host:port)")
	cmd.Flags().String(FlagRaftDir, def.Raft.RaftDir, "directory for Raft logs and snapshots")
	cmd.Flags().Bool(FlagRaftBootstrap, def.Raft.Bootstrap, "bootstrap a new static Raft cluster during initial bring-up")
	cmd.Flags().String(FlagRaftPeers, def.Raft.Peers, "comma-separated list of peer Raft addresses (nodeID@host:port)")
	cmd.Flags().Uint64(FlagRaftSnapCount, def.Raft.SnapCount, "number of snapshot files to retain on disk")
	cmd.Flags().Duration(FlagRaftSendTimeout, def.Raft.SendTimeout, "max duration to wait for a message to be sent to a peer")
	cmd.Flags().Duration(FlagRaftHeartbeatTimeout, def.Raft.HeartbeatTimeout, "time between leader heartbeats to followers")
	cmd.Flags().Duration(FlagRaftLeaderLeaseTimeout, def.Raft.LeaderLeaseTimeout, "duration of the leader lease")
	cmd.Flags().Duration(FlagRaftElectionTimeout, def.Raft.ElectionTimeout, "time a candidate waits for votes before restarting election")
	cmd.Flags().Uint64(FlagRaftSnapshotThreshold, def.Raft.SnapshotThreshold, "number of outstanding log entries that trigger an automatic snapshot")
	cmd.Flags().Uint64(FlagRaftTrailingLogs, def.Raft.TrailingLogs, "number of log entries to retain after a snapshot")
	cmd.MarkFlagsMutuallyExclusive(FlagCatchupTimeout, FlagRaftEnable)

	// Pruning configuration flags
	cmd.Flags().String(FlagPruningMode, def.Pruning.Mode, "pruning mode for stored block data and metadata (disabled, all, metadata)")
	cmd.Flags().Uint64(FlagPruningKeepRecent, def.Pruning.KeepRecent, "number of most recent blocks and their metadata to retain when pruning is enabled (must be > 0)")
	cmd.Flags().Duration(FlagPruningInterval, def.Pruning.Interval.Duration, "interval at which pruning is performed when pruning is enabled")
}

// Load loads the node configuration in the following order of precedence:
// 1. DefaultNodeConfig() (lowest priority)
// 2. YAML configuration file
// 3. Command line flags (highest priority)
func Load(cmd *cobra.Command) (Config, error) {
	home, _ := cmd.Flags().GetString(FlagRootDir)
	if home == "" {
		home = DefaultRootDir
	} else if !filepath.IsAbs(home) {
		// Convert relative path to absolute path
		absHome, err := filepath.Abs(home)
		if err != nil {
			return Config{}, fmt.Errorf("failed to resolve home directory: %w", err)
		}
		home = absHome
	}

	v := viper.New()
	v.SetConfigName(ConfigFileName)
	v.SetConfigType(ConfigExtension)
	v.AddConfigPath(filepath.Join(home, AppConfigDir))
	v.AddConfigPath(filepath.Join(home, AppConfigDir, ConfigName))
	v.SetConfigFile(filepath.Join(home, AppConfigDir, ConfigName))
	_ = v.BindPFlags(cmd.Flags())
	_ = v.BindPFlags(cmd.PersistentFlags())
	v.AutomaticEnv()

	// get the executable name
	executableName, err := os.Executable()
	if err != nil {
		return Config{}, err
	}

	if err := bindFlags(path.Base(executableName), cmd, v); err != nil {
		return Config{}, err
	}

	// read the configuration file
	// if the configuration file does not exist, we ignore the error
	// it will use the defaults
	_ = v.ReadInConfig()

	return loadFromViper(v, home)
}

// LoadFromViper loads the node configuration from a provided viper instance.
// It gets the home directory from the input viper, sets up a new viper instance
// to read the config file, and then merges both instances.
// This allows getting configuration values from both command line flags (or other sources)
// and the config file, with the same precedence as Load.
func LoadFromViper(inputViper *viper.Viper) (Config, error) {
	// get home directory from input viper
	home := inputViper.GetString(FlagRootDir)
	if home == "" {
		home = DefaultRootDir
	} else if !filepath.IsAbs(home) {
		// Convert relative path to absolute path
		absHome, err := filepath.Abs(home)
		if err != nil {
			return Config{}, fmt.Errorf("failed to resolve home directory: %w", err)
		}
		home = absHome
	}

	// create a new viper instance for reading the config file
	fileViper := viper.New()
	fileViper.SetConfigName(ConfigFileName)
	fileViper.SetConfigType(ConfigExtension)
	fileViper.AddConfigPath(filepath.Join(home, AppConfigDir))
	fileViper.AddConfigPath(filepath.Join(home, AppConfigDir, ConfigName))
	fileViper.SetConfigFile(filepath.Join(home, AppConfigDir, ConfigName))

	// read the configuration file
	// if the configuration file does not exist, we ignore the error
	// it will use the defaults
	_ = fileViper.ReadInConfig()

	// create a merged viper with input viper taking precedence
	mergedViper := viper.New()

	// first copy all settings from file viper
	for _, key := range fileViper.AllKeys() {
		mergedViper.Set(key, fileViper.Get(key))
	}

	// then override with settings from input viper (higher precedence)
	for _, key := range inputViper.AllKeys() {
		// we must not override config with default flags value
		if !inputViper.IsSet(key) {
			continue
		}

		// Handle special case for prefixed keys
		if after, ok := strings.CutPrefix(key, FlagPrefixEvnode); ok {
			// Strip the prefix for the merged viper
			strippedKey := after
			mergedViper.Set(strippedKey, inputViper.Get(key))
		} else if after, ok := strings.CutPrefix(key, FlagPrefixRollkit); ok {
			// Strip the prefix for the merged viper
			strippedKey := after
			mergedViper.Set(strippedKey, inputViper.Get(key))
		} else {
			mergedViper.Set(key, inputViper.Get(key))
		}
	}

	return loadFromViper(mergedViper, home)
}

// loadFromViper is a helper function that processes a viper instance and returns a Config.
// It is used by both Load and LoadFromViper to ensure consistent behavior.
func loadFromViper(v *viper.Viper, home string) (Config, error) {
	cfg := DefaultConfig()
	cfg.RootDir = home

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			func(f reflect.Type, t reflect.Type, data any) (any, error) {
				if t == reflect.TypeFor[DurationWrapper]() && f.Kind() == reflect.String {
					if str, ok := data.(string); ok {
						duration, err := time.ParseDuration(str)
						if err != nil {
							return nil, err
						}
						return DurationWrapper{Duration: duration}, nil
					}
				}
				return data, nil
			},
		),
		Result:           &cfg,
		WeaklyTypedInput: true,
	})
	if err != nil {
		return cfg, errors.Join(ErrReadYaml, fmt.Errorf("failed creating decoder: %w", err))
	}

	if err := decoder.Decode(v.AllSettings()); err != nil {
		return cfg, errors.Join(ErrReadYaml, fmt.Errorf("failed decoding viper: %w", err))
	}

	return cfg, nil
}

func bindFlags(basename string, cmd *cobra.Command, v *viper.Viper) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("bindFlags failed: %v", r)
		}
	}()

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// trimm possible prefixes from the flag name
		flagName := strings.TrimPrefix(f.Name, FlagPrefixEvnode)
		flagName = strings.TrimPrefix(flagName, FlagPrefixRollkit)

		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores, e.g. --favorite-color to STING_FAVORITE_COLOR
		err = v.BindEnv(flagName, fmt.Sprintf("%s_%s", basename, strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))))
		if err != nil {
			panic(err)
		}

		err = v.BindPFlag(flagName, f)
		if err != nil {
			panic(err)
		}

		// Apply the viper config value to the flag when the flag is not set and
		// viper has a value.
		if !f.Changed && v.IsSet(flagName) {
			val := v.Get(flagName)
			err = cmd.Flags().Set(flagName, fmt.Sprintf("%v", val))
			if err != nil {
				panic(err)
			}
		}
	})

	return err
}
