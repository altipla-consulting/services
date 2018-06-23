
# services

[![GoDoc](https://godoc.org/github.com/altipla-consulting/services?status.svg)](https://godoc.org/github.com/altipla-consulting/services)
[![Build Status](https://travis-ci.org/altipla-consulting/services.svg?branch=master)](https://travis-ci.org/altipla-consulting/services)

Helper to initialize services & applications.


### Install

```shell
go get github.com/altipla-consulting/services
```

This library has the following dependencies:

- [cloud.google.com/go/profiler](cloud.google.com/go/profiler)
- [github.com/altipla-consulting/cron](github.com/altipla-consulting/cron)
- [github.com/altipla-consulting/king](github.com/altipla-consulting/king)
- [github.com/julienschmidt/httprouter](github.com/julienschmidt/httprouter)
- [github.com/sirupsen/logrus](github.com/sirupsen/logrus)
- [golang.org/x/net/trace](golang.org/x/net/trace)


### Contributing

You can make pull requests or create issues in GitHub. Any code you send should be formatted using `gofmt`.


### Running tests

Run the tests

```shell
make test
```


### License

[MIT License](LICENSE)
