package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ahmdrz/goinsta"

	log "github.com/sirupsen/logrus"
)

var (
	Version = "set with -ldflags \"-X main.Verions=$VERSION\""

	Port = os.Getenv("PORT")

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
}

func main() {
	ctx := context.Background()
	c, err := NewClient(ctx, Bucket, StateFile, Username, Password)
	if err != nil {
		log.Fatalln("main:", err)
	}
	defer c.Shutdown(ctx)

	// start work
	go c.TickFollowDiff(ctx, time.Hour)

	// spin off server
	go c.svr.ListenAndServe()

	// block on waiting for signal
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGKILL)
	<-sigs
}

type Client struct {
	store *storage.Client
	buck  *storage.BucketHandle
	insta *goinsta.Instagram
	svr   *http.Server

	alive, ready bool
	statefile    string
}

func NewClient(ctx context.Context, bkt, stateFile, username, password string) (*Client, error) {
	var err error
	c := &Client{
		svr: &http.Server{
			Addr: Port,
		},
		alive:     true,
		statefile: stateFile,
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		if !c.alive {
			status = http.StatusInternalServerError
		}
		w.WriteHeader(status)
	})
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		if !c.ready {
			status = http.StatusInternalServerError
		}
		w.WriteHeader(status)
	})

	log.Infoln("NewClient setup Storage client")
	c.store, err = storage.NewClient(ctx)
	if err != nil {
		log.Errorln("NewClient setup Storage:", err)
		return c, err
	}
	c.buck = c.store.Bucket(bkt)

	log.Infoln("NewClient get", c.statefile, "from bucket", bkt)
	r, err := c.buck.Object(c.statefile).NewReader(ctx)
	if err != nil {
		log.Infoln("NewClient", c.statefile, "not found, creating new instance of", username)
		c.insta = goinsta.New(username, password)
		log.Infoln("NewClient goinsta login")
		err = c.insta.Login()
		if err != nil {
			log.Errorln("NewClient goinsta login", err)
			return c, err
		}
	} else {
		defer r.Close()
		log.Infoln("NewClient restore from ", c.statefile, "in", bkt)
		c.insta, err = goinsta.ImportReader(r)
		if err != nil {
			log.Errorln("NewClient restore goinsta", err)
			return c, err
		}
	}

	c.ready = true
	return c, nil
}

func (c *Client) Shutdown(ctx context.Context) {
	var err error
	c.ready = false

	log.Infoln("Shutdown exporting goinsta to /tmp/goinsta.state")
	err = c.insta.Export("/tmp/goinsta.state")
	if err != nil {
		log.Errorln("Shutdown export goinsta to /tmp/goinsta.state", err)
	}

	log.Infoln("Shutdown opening /tmp/goinsta.state")
	f, err := os.Open("/tmp/goinsta.state")
	if err != nil {
		log.Errorln("Shutdown opening /tmp/goinsta.state", err)
	}
	defer f.Close()

	log.Infoln("Shutdown writing goinsta state to Storage")
	w := c.buck.Object(c.statefile).NewWriter(ctx)
	defer w.Close()
	_, err = io.Copy(w, f)
	if err != nil {
		log.Errorln("Shutdown writing state to storage", err)
	}

	log.Infoln("Shutdown closing storage client")
	err = c.store.Close()
	if err != nil {
		log.Errorln("Shutdown closing storage client", err)
	}

	log.Infoln("Shutdown http server")
	err = c.svr.Shutdown(ctx)
	if err != nil {
		log.Errorln("Shutdown http server", err)
	}
}
