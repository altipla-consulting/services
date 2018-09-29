package services

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"time"

	"cloud.google.com/go/profiler"
	"github.com/altipla-consulting/routing"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/trace"
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
	routingUsername     string
	routingPassword     string

	enableProfiler bool

	enableGRPC       bool
	grpcServer       *grpc.Server
	grpcServerCalled bool
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

// ConfigureRouting enables a HTTP router.
func (service *Service) ConfigureRouting() {
	service.enableRouting = true
}

// ConfigureRouting enables a HTTP router with a simple password for to beta
// test the real application.
func (service *Service) ConfigureBetaRouting(username, password string) {
	service.enableRouting = true
	service.routingUsername = username
	service.routingPassword = password
}

// ConfigureProfiler enables the Stackdriver Profiler agent.
func (service *Service) ConfigureProfiler() {
	service.enableProfiler = true
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
		service.grpcServer = grpc.NewServer()
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
		service.routingServer = routing.NewServer(routing.WithLogrus(), routing.WithSentry(service.sentryDSN), routing.WithBetaAuth(service.routingUsername, service.routingPassword))
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

	if service.enableProfiler && !IsLocal() {
		cnf := profiler.Config{
			Service:        service.name,
			ServiceVersion: Version(),
			DebugLogging:   IsLocal(),
		}
		if err := profiler.Start(cnf); err != nil {
			log.Fatal(err)
		}
	}

	if service.enableRouting {
		go func() {
			log.Fatal(http.ListenAndServe(":8080", service.routingServer.Router()))
		}()
	}

	if service.enableGRPC {
		go func() {
			lis, err := net.Listen("tcp", ":9000")
			if err != nil {
				log.Fatal(err)
			}
			log.Info("GRPC server initialized successfully!")
			log.Fatal(service.grpcServer.Serve(lis))
		}()
	}

	trace.AuthRequest = func(req *http.Request) (any, sensitive bool) { return true, true }
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "%s is ok\n", service.name) })

	log.WithField("name", service.name).Println("Instance initialized successfully!")
	log.Fatal(http.ListenAndServe(":8000", nil))
}
