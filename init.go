package services

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"cloud.google.com/go/profiler"
	"github.com/altipla-consulting/cron"
	"github.com/altipla-consulting/king"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/trace"
)

type Service struct {
	name string

	enabledSentry bool
	sentryDSN     string

	enabledKing bool
	kingRouter  *httprouter.Router

	enabledCron bool
	cronRunner  *cron.Runner

	enabledRouting bool
	httpRouter     *httprouter.Router
	httpRouterCalled bool

	profilerEnabled bool
}

func Init(name string) *Service {
	return &Service{
		name: name,
		kingRouter: httprouter.New(),
		httpRouter: httprouter.New(),
	}
}

func (service *Service) ConfigureSentry(dsn string) {
	if dsn != "" {
		service.enabledSentry = true
		service.sentryDSN = dsn
	}
}

func (service *Service) ConfigureKing() {
	service.enabledKing = true
}

func (service *Service) ConfigureCron() {
	service.enabledCron = true
}

func (service *Service) ConfigureRouting() {
	service.enabledRouting = true
}

func (service *Service) ConfigureProfiler() {
	service.profilerEnabled = true
}

func (service *Service) HTTPRouter() *httprouter.Router {
	if !service.enabledRouting {
		panic("routing must be enabled to get an http router")
	}

	service.httpRouterCalled = true

	return service.httpRouter
}

func (service *Service) KingRouter() *httprouter.Router {
	if !service.enabledKing {
		panic("king must be enabled to get a king router")
	}

	return service.kingRouter
}

func (service *Service) CronRunner() *cron.Runner {
	if !service.enabledCron {
		panic("crons must be enabled to get a cron runner")
	}

	if service.cronRunner == nil {
		service.cronRunner = cron.NewRunner(cron.WithSentry(service.sentryDSN))
	}

	return service.cronRunner
}

func (service *Service) Run() {
	rand.Seed(time.Now().UTC().UnixNano())

	if service.enabledRouting && !service.httpRouterCalled {
		panic("do not configure routing without routes")
	}

	if service.profilerEnabled && !IsLocal() {
		cnf := profiler.Config{
			Service:        service.name,
			ServiceVersion: Version(),
			DebugLogging:   IsLocal(),
		}
		if err := profiler.Start(cnf); err != nil {
			log.Fatal(err)
		}
	}

	if service.enabledKing {
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

	if service.enabledCron && IsLocal() {
		service.kingRouter.GET(fmt.Sprintf("/crons/%s/:job", service.name), service.cronRunner.Handler())
		service.httpRouter.GET(fmt.Sprintf("/crons/%s/:job", service.name), service.cronRunner.Handler())
	}

	if service.enabledKing || service.enabledCron {
		go func() {
			log.Fatal(http.ListenAndServe(":9000", service.kingRouter))
		}()
	}

	if service.enabledRouting || service.enabledCron {
		go func() {
			log.Fatal(http.ListenAndServe(":8080", service.httpRouter))
		}()
	}

	trace.AuthRequest = func(req *http.Request) (any, sensitive bool) { return true, true }
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "%s is ok\n", service.name) })

	log.WithField("name", service.name).Println("Instance initialized successfully!")
	log.Fatal(http.ListenAndServe(":8000", nil))
}
