module github.com/rollkit/rollkit/execution/grpc

go 1.24.1

toolchain go1.24.5

require (
	connectrpc.com/connect v1.18.1
	connectrpc.com/grpcreflect v1.3.0
	github.com/rollkit/rollkit v0.0.0
	github.com/rollkit/rollkit/core v0.0.0
	golang.org/x/net v0.42.0
	google.golang.org/protobuf v1.36.6
)

require golang.org/x/text v0.27.0 // indirect

replace (
	github.com/rollkit/rollkit => ../../
	github.com/rollkit/rollkit/core => ../../core
)
