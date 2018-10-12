package services

import (
	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
)

// Endpoint is a simple string with the host and port of the remote GRPC
// service. We use a custom type to avoid using grpc.Dial without noticing the bug.
// 
// This needs a "discovery" package with the full list of remote addresses that
// use this type instead of string and never using the direct address. That way
// if you use grpc.Dial it will report the compilation error inmediatly.
type Endpoint string

// Dial helps to open a connection to a remote GRPC server with tracing support and
// other goodies configured in this package.
func Dial(target Endpoint, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts = append(opts, grpc.WithStatsHandler(new(ocgrpc.ClientHandler)))
	return grpc.Dial(string(target), opts...)
}