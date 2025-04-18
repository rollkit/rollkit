module github.com/rollkit/rollkit/sequencers/single

go 1.24.1

replace (
	github.com/rollkit/rollkit => ../../
	github.com/rollkit/rollkit/core => ../../core
	github.com/rollkit/rollkit/da => ../../da
)

require (
	cosmossdk.io/log v1.5.1
	github.com/go-kit/kit v0.13.0
	github.com/ipfs/go-datastore v0.8.2
	github.com/prometheus/client_golang v1.21.1
	github.com/rollkit/rollkit v0.0.0-00010101000000-000000000000
	github.com/rollkit/rollkit/core v0.0.0-20250312114929-104787ba1a4c
	github.com/rollkit/rollkit/da v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.10.0
	google.golang.org/protobuf v1.36.6
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bytedance/sonic v1.13.2 // indirect
	github.com/bytedance/sonic/loader v0.2.4 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.63.0 // indirect
	github.com/prometheus/procfs v0.16.0 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	golang.org/x/arch v0.15.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
