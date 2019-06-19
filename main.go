package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"sync"
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
	Emails    = make(map[string]struct{})
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
	if Port == "" {
		Port = ":8080"
	}

	for _, e := range strings.Split(os.Getenv("EMAILS"), ",") {
		Emails[strings.TrimSpace(e)] = struct{}{}
	}
}

func main() {
	ctx := context.Background()
	log.Infoln("main NewClient")
	c, err := NewClient(ctx, Bucket, StateFile, Username, Password)
	if err != nil {
		log.Fatalln("main:", err)
	}
	defer c.Shutdown(ctx)

	var wg sync.WaitGroup

	// start work
	wg.Add(1)
	go tick(ctx, time.Hour, c.FollowDiff, &wg)
	go func() {
		wg.Wait()
		c.ready = true
	}()

	// spin off server
	go c.svr.ListenAndServe()

	// block on waiting for signal
	log.Infoln("main waiting")
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGKILL)
	<-sigs
}

func tick(ctx context.Context, d time.Duration, f func(context.Context), wg *sync.WaitGroup) {
	f(ctx)
	wg.Done()
	for range time.NewTicker(d).C {
		f(ctx)
	}
}
