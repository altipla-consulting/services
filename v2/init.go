package services

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	// Register pprof
	_ "net/http/pprof"

	"cloud.google.com/go/profiler"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"github.com/altipla-consulting/routing"
	log "github.com/sirupsen/logrus"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/trace"
	gotrace "golang.org/x/net/trace"
	"google.golang.org/grpc"
)

// Service stores the configuration of the service we are configuring.
type Service struct {
	name string

	enableSentry bool
	sentryDSN    string

	enableRouting       bool
	routingServer       *routing.Server
	routingServerCalled bool
	routingHTTPServer   *http.Server
	routingOpts         []routing.ServerOption

	enableProfiler bool

	enableTracer        bool
	tracerGoogleProject string
	traceExporter       *stackdriver.Exporter

	enableGRPC       bool
	grpcServer       *grpc.Server
	grpcServerCalled bool

	debugHTTPServer *http.Server
}

// Init the configuration of a new service for the current application with
// the provided name.
func Init(name string) *Service {
	return &Service{
		name: name,
	}
}

// ConfigureSentry enables Sentry support in all the features that support it.
func (service *Service) ConfigureSentry(dsn string) {
	if dsn != "" {
		service.enableSentry = true
		service.sentryDSN = dsn
	}
}

// ConfigureRouting enables a HTTP router with the custom options we need. Logrus will
// be always enabled and Sentry will be configured if a DSN is provided in the
// ConfigureSentry call.
func (service *Service) ConfigureRouting(opts ...routing.ServerOption) {
	service.enableRouting = true
	service.routingOpts = opts
}

// ConfigureBetaRouting enables a HTTP router with a simple password for to beta
// test the real application.
//
// DEPRECATED: Use ConfigureRouting(routing.WithBetaAuth(username, password)) instead.
func (service *Service) ConfigureBetaRouting(username, password string) {
	service.ConfigureRouting(routing.WithBetaAuth(username, password))
}

// ConfigureProfiler enables the Stackdriver Profiler agent.
func (service *Service) ConfigureProfiler() {
	service.enableProfiler = !IsLocal()
}

// ConfigureTracer enables the Stackdriver Trace agent.
func (service *Service) ConfigureTracer(googleProject string) {
	if googleProject != "" && !IsLocal() {
		service.enableTracer = true
		service.tracerGoogleProject = googleProject
	}
}

// ConfigureGRPC enables a GRPC server.
func (service *Service) ConfigureGRPC() {
	service.enableGRPC = true
}

// GRPCServer returns the server to register new GRPC services on it.
func (service *Service) GRPCServer() *grpc.Server {
	if !service.enableGRPC {
		panic("grpc must be enabled to get a grpc server")
	}

	if service.grpcServer == nil {
		opts := []grpc.ServerOption{
			grpc.UnaryInterceptor(grpcUnaryErrorLogger(service.enableTracer, service.name, service.sentryDSN)),
			grpc.StreamInterceptor(grpcStreamErrorLogger(service.name, service.sentryDSN)),
		}
		if service.enableTracer {
			opts = append(opts, grpc.StatsHandler(new(ocgrpc.ServerHandler)))
		}

		service.grpcServer = grpc.NewServer(opts...)
	}

	service.grpcServerCalled = true

	return service.grpcServer
}

// RoutingServer returns the server to register new HTTP routes on it.
func (service *Service) RoutingServer() *routing.Server {
	if !service.enableRouting {
		panic("routing must be enabled to get a routing router")
	}

	if service.routingServer == nil {
		opts := []routing.ServerOption{
			routing.WithLogrus(),
			routing.WithSentry(service.sentryDSN),
		}
		opts = append(opts, service.routingOpts...)
		service.routingServer = routing.NewServer(opts...)
	}

	service.routingServerCalled = true

	return service.routingServer
}

// Run starts listening in every configure port needed to provide the configured features.
func (service *Service) Run() {
	rand.Seed(time.Now().UTC().UnixNano())

	if service.enableRouting && !service.routingServerCalled {
		panic("do not configure routing without routes")
	}
	if service.enableGRPC && !service.grpcServerCalled {
		panic("do not configure grpc without services")
	}

	if service.enableSentry {
		log.WithField("dsn", service.sentryDSN).Info("Sentry enabled")
	}

	if service.enableProfiler {
		log.Info("Stackdriver Profiler enabled")

		cnf := profiler.Config{
			Service:        service.name,
			ServiceVersion: Version(),
		}
		if err := profiler.Start(cnf); err != nil {
			log.Fatal(err)
		}
	}

	if service.enableTracer {
		log.WithField("project", service.tracerGoogleProject).Info("Stackdriver Trace enabled")

		var err error
		service.traceExporter, err = stackdriver.NewExporter(stackdriver.Options{ProjectID: service.tracerGoogleProject})
		if err != nil {
			log.Fatal(err)
		}
		trace.RegisterExporter(service.traceExporter)

		sampler := newCustomSampler()
		trace.ApplyConfig(trace.Config{
			DefaultSampler: sampler.Sampler(),
		})
	}

	if service.enableRouting {
		go func() {
			log.Info("Routing server enabled")

			service.routingHTTPServer = &http.Server{
				Addr:    ":8080",
				Handler: service.routingServer.Router(),
			}
			if err := service.routingHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal(err)
			}
		}()
	}

	if service.enableGRPC {
		go func() {
			log.Info("GRPC server enabled")

			listener, err := net.Listen("tcp", ":9000")
			if err != nil {
				log.Fatal(err)
			}
			log.Fatal(service.grpcServer.Serve(listener))
		}()
	}

	gotrace.AuthRequest = func(req *http.Request) (any, sensitive bool) { return true, true }
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "%s is ok\n", service.name) })

	service.stopListener()

	log.WithField("name", service.name).Println("Instance initialized successfully!")

	service.debugHTTPServer = &http.Server{
		Addr: ":8000",
	}
	if err := service.debugHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (service *Service) stopListener() {
	var gracefulStop = make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)

	go func() {
		sig := <-gracefulStop
		log.WithField("signal", sig).Info("Caught OS signal")

		var wg sync.WaitGroup

		if service.enableGRPC {
			wg.Add(1)
			go func() {
				defer wg.Done()

				service.grpcServer.GracefulStop()
			}()
		}

		if service.enableTracer {
			wg.Add(1)
			go func() {
				defer wg.Done()

				service.traceExporter.Flush()
			}()
		}

		if service.enableRouting {
			wg.Add(1)
			go func() {
				defer wg.Done()

				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()

				if err := service.routingHTTPServer.Shutdown(ctx); err != nil {
					log.WithField("error", err).Error("Cannot shutdown routing HTTP server")
				}
			}()
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			if err := service.debugHTTPServer.Shutdown(ctx); err != nil {
				log.WithField("error", err).Error("Cannot shutdown debug HTTP server")
			}
		}()

		wg.Wait()
		os.Exit(0)
	}()
}
