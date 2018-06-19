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
	Name       string
	HTTPRouter *httprouter.Router
	KingRouter *httprouter.Router

	SentryEnabled bool
	SentryDSN     string

	KingEnabled bool

	CronEnabled bool

	RoutingEnabled bool

	ProfilerEnabled bool

	CronRunner *cron.Runner
}

func Init(name string) *Service {
	rand.Seed(time.Now().UTC().UnixNano())

	return &Service{
		Name:       name,
		KingRouter: httprouter.New(),
		HTTPRouter: httprouter.New(),
		CronRunner: cron.NewRunner(),
	}
}

func (service *Service) ConfigureSentry(dsn string) {
	if dsn != "" {
		service.SentryEnabled = true
		service.SentryDSN = dsn
	}
}

func (service *Service) ConfigureKing() {
	service.KingEnabled = true
}

func (service *Service) ConfigureCron() {
	service.CronEnabled = true
}

func (service *Service) ConfigureRouting() {
	service.RoutingEnabled = true
}

func (service *Service) ConfigureProfiler() {
	service.ProfilerEnabled = true
}

func (service *Service) Run() {
	if !IsLocal() && service.ProfilerEnabled {
		cnf := profiler.Config{
			Service:        service.Name,
			ServiceVersion: Version(),
			DebugLogging:   IsLocal(),
		}
		if err := profiler.Start(cnf); err != nil {
			log.Fatal(err)
		}
	}

	if service.KingEnabled {
		options := []king.ServerOption{
			king.WithHttprouter(service.KingRouter),
			king.WithLogrus(),
			king.Debug(IsLocal()),
		}
		if !IsLocal() {
			options = append(options, king.WithSentry(service.SentryDSN))
		}
		king.NewServer(options...)
	}

	if service.CronEnabled {
		if IsLocal() {
			service.KingRouter.GET(fmt.Sprintf("/crons/%s/:job", service.Name), service.CronRunner.Handler())
			service.HTTPRouter.GET(fmt.Sprintf("/crons/%s/:job", service.Name), service.CronRunner.Handler())
		}
	}

	if service.KingEnabled || service.CronEnabled {
		go func() {
			log.Fatal(http.ListenAndServe(":9000", service.KingRouter))
		}()
	}

	if service.RoutingEnabled || service.CronEnabled {
		go func() {
			log.Fatal(http.ListenAndServe(":8080", service.HTTPRouter))
		}()
	}

	trace.AuthRequest = func(req *http.Request) (any, sensitive bool) { return true, true }
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "%s is ok\n", service.Name) })

	log.WithField("name", service.Name).Println("Instance initialized successfully!")
	log.Fatal(http.ListenAndServe(":8000", nil))
}
