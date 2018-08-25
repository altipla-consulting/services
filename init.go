package services

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"time"

	"cloud.google.com/go/profiler"
	"github.com/altipla-consulting/cron"
	"github.com/altipla-consulting/king"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
)

type Service struct {
	name string

	enableSentry bool
	sentryDSN    string

	enableKing bool
	kingRouter *httprouter.Router

	enableCron bool
	cronRunner *cron.Runner

	enableRouting    bool
	httpRouter       *httprouter.Router
	httpRouterCalled bool

	enableProfiler bool

	enableGRPC       bool
	grpcServer       *grpc.Server
	grpcServerCalled bool
}

func Init(name string) *Service {
	return &Service{
		name:       name,
		kingRouter: httprouter.New(),
		httpRouter: httprouter.New(),
	}
}

func (service *Service) ConfigureSentry(dsn string) {
	if dsn != "" {
		service.enableSentry = true
		service.sentryDSN = dsn
	}
}

func (service *Service) ConfigureKing() {
	service.enableKing = true
}

func (service *Service) ConfigureCron() {
	service.enableCron = true
}

func (service *Service) ConfigureRouting() {
	service.enableRouting = true
}

func (service *Service) ConfigureProfiler() {
	service.enableProfiler = true
}

func (service *Service) ConfigureGRPC() {
	service.enableGRPC = true
	service.grpcServer = grpc.NewServer()
}

func (service *Service) GRPCServer() *grpc.Server {
	if !service.enableGRPC {
		panic("grpc must be enabled to get a grpc server")
	}

	service.grpcServerCalled = true

	return service.grpcServer
}

func (service *Service) HTTPRouter() *httprouter.Router {
	if !service.enableRouting {
		panic("routing must be enabled to get an http router")
	}

	service.httpRouterCalled = true

	return service.httpRouter
}

func (service *Service) KingRouter() *httprouter.Router {
	if !service.enableKing {
		panic("king must be enabled to get a king router")
	}

	return service.kingRouter
}

func (service *Service) CronRunner() *cron.Runner {
	if !service.enableCron {
		panic("crons must be enabled to get a cron runner")
	}

	if service.cronRunner == nil {
		service.cronRunner = cron.NewRunner(cron.WithSentry(service.sentryDSN))
	}

	return service.cronRunner
}

func (service *Service) Run() {
	rand.Seed(time.Now().UTC().UnixNano())

	if service.enableRouting && !service.httpRouterCalled {
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

	if service.enableKing {
		options := []king.ServerOption{
			king.WithHttprouter(service.kingRouter),
			king.WithLogrus(),
			king.Debug(IsLocal()),
		}
		if !IsLocal() {
			options = append(options, king.WithSentry(service.sentryDSN))
		}
		king.NewServer(options...)
	}

	if service.enableCron && IsLocal() {
		service.kingRouter.GET(fmt.Sprintf("/crons/%s/:job", service.name), service.cronRunner.Handler())
		service.httpRouter.GET(fmt.Sprintf("/crons/%s/:job", service.name), service.cronRunner.Handler())
	}

	if service.enableKing || service.enableCron {
		go func() {
			log.Fatal(http.ListenAndServe(":9000", service.kingRouter))
		}()
	}

	if service.enableRouting || service.enableCron {
		go func() {
			log.Fatal(http.ListenAndServe(":8080", service.httpRouter))
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
