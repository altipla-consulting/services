package services

import (
	"context"
	"strings"

	"github.com/altipla-consulting/sentry"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// CustomSampler controls the traces to avoid sending uninteresting ones.
func CustomSampler(params trace.SamplingParameters) trace.SamplingDecision {
	log.WithField("name", params.Name).Info("Trace decision")
	// Do not trace requests to the profiler that happen in the background.
	if strings.HasPrefix(params.Name, "Sent.google.devtools.cloudprofiler.") {
		return trace.SamplingDecision{}
	}

	return trace.SamplingDecision{Sample: true}
}

func grpcErrorLogger(serviceName, dsn string) grpc.UnaryServerInterceptor {
	var client *sentry.Client
	if dsn != "" {
		client = sentry.NewClient(dsn)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = sentry.WithContextRPC(ctx, serviceName, info.FullMethod)

		resp, err := handler(ctx, req)
		if err != nil {
			grpcerr, ok := status.FromError(err)
			if ok {
				// Always log the GRPC errors.
				log.WithFields(log.Fields{
					"code":    grpcerr.Code().String(),
					"message": grpcerr.Message(),
				}).Error("GRPC call failed")

				// Do not notify those status codes.
				switch grpcerr.Code() {
				case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists, codes.FailedPrecondition, codes.Aborted, codes.Unimplemented:
					return resp, err
				}
			} else {
				log.WithFields(log.Fields{
					"error": err.Error(),
					"stack": errors.ErrorStack(err),
				}).Error("Unknown error in GRPC call")
			}

			if client != nil {
				client.ReportInternal(ctx, err)
			}
		}

		return resp, err
	}
}
