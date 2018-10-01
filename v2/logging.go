package services

import (
	"context"

	"github.com/altipla-consulting/sentry"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	if IsLocal() {
		log.SetFormatter(&log.TextFormatter{
			ForceColors: true,
		})
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetFormatter(new(log.JSONFormatter))
	}
}

func grpcErrorLogger(dsn string) grpc.UnaryServerInterceptor {
	var client *sentry.Client
	if dsn != "" {
		client = sentry.NewClient(dsn)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
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
