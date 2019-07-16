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
	APIToken = os.Getenv("TELEGRAM_TOKEN")
	Bucket   = os.Getenv("BUCKET")
	Interval = 20 * time.Minute
	SaveFile = "iglogbot.json"
)

func init() {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "error":
		fallthrough
	default:
		log.SetLevel(log.ErrorLevel)
	}
	log.Infoln("Log level set to", log.GetLevel())

	switch strings.ToLower(os.Getenv("LOG_FORMAT")) {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
		log.Infoln("Log format set to json")
	default:
		log.SetFormatter(&log.TextFormatter{})
		log.Infoln("Log format set to text")
	}
}

func main() {
	ctx := context.Background()

	s, err := NewServer(ctx)
	if err != nil {
		log.Fatal("main:", err)
	}
	defer s.Export(ctx)

	go s.Sender()
	go s.Respond()
	go func(d time.Duration) {
		s.Update()
		s.Export(ctx)
		for range time.NewTicker(d).C {
			s.Update()
			s.Export(ctx)
		}
	}(Interval)

	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGKILL)
	sig := <-sigs
	log.Errorln("main got signal:", sig)
}
