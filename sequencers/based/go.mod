module github.com/rollkit/rollkit/sequencers/based

go 1.24.0

replace (
	github.com/rollkit/rollkit => ../../
	github.com/rollkit/rollkit/core => ../../core
	github.com/rollkit/rollkit/da => ../../da
)

require (
	cosmossdk.io/log v1.5.0
	github.com/ipfs/go-datastore v0.7.0
	github.com/rollkit/rollkit/core v0.0.0-20250312114929-104787ba1a4c
	github.com/rollkit/rollkit/da v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/bytedance/sonic v1.12.3 // indirect
	github.com/bytedance/sonic/loader v0.2.0 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/cloudwego/iasm v0.2.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	golang.org/x/arch v0.0.0-20210923205945-b76863e36670 // indirect
	golang.org/x/sys v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
