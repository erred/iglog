package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
	"github.com/ahmdrz/goinsta"
	log "github.com/sirupsen/logrus"
)

type Client struct {
	store *storage.Client
	buck  *storage.BucketHandle
	insta *goinsta.Instagram
	svr   *http.Server

	alive, ready bool
	statefile    string

	followDiff *FollowDiff
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
		log.Errorln("Newclient get", bkt, "/", c.statefile, err)
		log.Infoln("NewClient", c.statefile, "not found, creating new instance of", username)
		c.insta = goinsta.New(username, password)
		log.Infoln("NewClient goinsta login")
		err = c.insta.Login()
		if err != nil {
			log.Errorln("NewClient goinsta login", err)
			return c, err
		}

		log.Infoln("NewClient exporting goinsta to /tmp/goinsta.state")
		err = c.insta.Export("/tmp/goinsta.state")
		if err != nil {
			log.Errorln("NewClient export goinsta to /tmp/goinsta.state", err)
		}

		log.Infoln("NewClient opening /tmp/goinsta.state")
		f, err := os.Open("/tmp/goinsta.state")
		if err != nil {
			log.Errorln("NewClient opening /tmp/goinsta.state", err)
		}
		defer f.Close()

		log.Infoln("NewClient writing goinsta state to Storage")
		w := c.buck.Object(c.statefile).NewWriter(ctx)
		defer w.Close()
		_, err = io.Copy(w, f)
		if err != nil {
			log.Errorln("NewClient writing state to storage", err)
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

	log.Infoln("Shutdown exporting goinsta to goinsta.state")
	err = c.insta.Export("goinsta.state")
	if err != nil {
		log.Errorln("Shutdown export goinsta to goinsta.state", err)
	}

	log.Infoln("Shutdown opening goinsta.state")
	f, err := os.Open("goinsta.state")
	if err != nil {
		log.Errorln("Shutdown opening goinsta.state", err)
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

func (c *Client) Decode(ctx context.Context, obj string, d interface{}) error {
	log.Infoln("Decoding", obj)
	r, err := c.buck.Object(obj).NewReader(ctx)
	if err != nil {
		log.Errorln("Decode get reader for", obj, err)
		return err
	}
	defer r.Close()
	err = json.NewDecoder(r).Decode(d)
	if err != nil {
		log.Errorln("Decode json for", obj, err)
	}
	return err
}

func (c *Client) Encode(ctx context.Context, obj string, d interface{}) error {
	log.Infoln("Encoding", obj)
	w := c.buck.Object(obj).NewWriter(ctx)
	defer w.Close()
	err := json.NewEncoder(w).Encode(d)
	if err != nil {
		log.Errorln("Encode json for", obj, err)
	}
	return err
}
