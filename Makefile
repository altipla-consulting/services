
FILES = $(shell find . -type f -name '*.go' -not -path './vendor/*')

gofmt:
	@gofmt -w $(FILES)
	@gofmt -r '&α{} -> new(α)' -w $(FILES)

deps:
	go get -u github.com/mgechev/revive

	go get -u cloud.google.com/go/profiler
	go get -u github.com/altipla-consulting/cron
	go get -u github.com/altipla-consulting/king
	go get -u github.com/julienschmidt/httprouter
	go get -u github.com/sirupsen/logrus
	go get -u golang.org/x/net/trace

lint:
	revive -formatter friendly
	go install .
