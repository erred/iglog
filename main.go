package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	Version = "set with -ldflags \"-X main.Verions=$VERSION\""

	Headers = strings.Split(os.Getenv("HEADERS"), ",")
	Origins = make(map[string]struct{})
	Port    = os.Getenv("PORT")

	StateFile = "state.goinsta"
	Username  = os.Getenv("IG_USER")
	Password  = os.Getenv("IG_PASS")
	Bucket    = os.Getenv("BUCKET")
)

func init() {
	switch os.Getenv("LOG_LEVEL") {
	case "DEBUG":
		log.SetLevel(log.DebugLevel)
	case "INFO":
		log.SetLevel(log.InfoLevel)
	case "ERROR":
		fallthrough
	default:
		log.SetLevel(log.ErrorLevel)
	}

	switch os.Getenv("LOG_FORMAT") {
	case "JSON":
		log.SetFormatter(&log.JSONFormatter{})
	default:
		log.SetFormatter(&log.TextFormatter{})
	}

	for i, h := range Headers {
		Headers[i] = strings.TrimSpace(h)
	}

	for _, o := range strings.Split(os.Getenv("ORIGINS"), ",") {
		Origins[strings.TrimSpace(o)] = struct{}{}
	}
}

func main() {
	ctx := context.Background()
	c, err := NewClient(ctx, Bucket, StateFile, Username, Password)
	if err != nil {
		log.Fatalln("main:", err)
	}
	defer c.Shutdown(ctx)

	// start work
	go tick(ctx, time.Hour, c.FollowDiff)

	// spin off server
	go c.svr.ListenAndServe()

	// block on waiting for signal
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGKILL)
	<-sigs
}

func tick(ctx context.Context, d time.Duration, f func(context.Context)) {
	f(ctx)
	for range time.NewTicker(d).C {
		f(ctx)
	}
}
