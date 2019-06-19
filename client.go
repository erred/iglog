package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"
	"github.com/ahmdrz/goinsta"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/seankhliao/iglog/iglog"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	store *storage.Client
	buck  *storage.BucketHandle
	insta *goinsta.Instagram
	svr   *http.Server
	auth  *auth.Client

	alive, ready bool
	statefile    string

	// followatch data
	fDiff *FollowDiff
	pDiff *ProtoDiff
	// cached grpc
	gEvents *iglog.Events
	gWers   *iglog.Users
	gWing   *iglog.Users
}

func allowOrigin(o string) bool {
	_, ok := Origins[o]
	log.Debugln("allowOrigin", o, ok)
	return ok
}

func NewClient(ctx context.Context, bkt, stateFile, username, password string) (*Client, error) {
	log.Infoln("NewClient starting")
	var err error
	c := &Client{
		svr: &http.Server{
			Addr: Port,
		},
		alive:     true,
		statefile: stateFile,
	}

	log.Debugln("NewClient firebase NewApp")
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		log.Fatalln("NewClient firebase NewApp", err)
	}

	log.Debugln("NewClient firebase auth")
	c.auth, err = app.Auth(ctx)
	if err != nil {
		log.Fatalln("NewClient firebase auth", err)
	}

	log.Debugln("Newclient register handlers")
	c.registerHandlers(ctx)

	log.Debugln("NewClient setup Storage client")
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
		log.Debugln("NewClient goinsta login")
		err = c.insta.Login()
		if err != nil {
			log.Errorln("NewClient goinsta login", err)
			return c, err
		}

		log.Debugln("Newclient saveInsta")
		c.saveInsta(ctx)
	} else {
		defer r.Close()
		log.Infoln("NewClient restore from ", c.statefile, "in", bkt)
		c.insta, err = goinsta.ImportReader(r)
		if err != nil {
			log.Errorln("NewClient restore goinsta", err)
			return c, err
		}
	}

	return c, nil
}

func (c *Client) Shutdown(ctx context.Context) {
	var err error
	c.ready = false
	log.Infoln("Shutdown starting")

	log.Debugln("Shutdown saveInsta")
	c.saveInsta(ctx)

	log.Debugln("Shutdown closing storage client")
	err = c.store.Close()
	if err != nil {
		log.Errorln("Shutdown closing storage client", err)
	}

	log.Debugln("Shutdown http server")
	err = c.svr.Shutdown(ctx)
	if err != nil {
		log.Errorln("Shutdown http server", err)
	}
}

func (c *Client) registerHandlers(ctx context.Context) {
	log.Infoln("registerHandlers starting")
	gsvr := grpc.NewServer(grpc.UnaryInterceptor(c.authInterceptor))
	iglog.RegisterFollowatchServer(gsvr, c)
	wsvr := grpcweb.WrapServer(gsvr,
		grpcweb.WithOriginFunc(allowOrigin),
		grpcweb.WithAllowedRequestHeaders(Headers),
	)

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
	http.Handle("/", wsvr)
}

func (c *Client) authInterceptor(ctx context.Context, r interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	log.Infoln("authInterceptor authorizing")

	log.Debugln("authInterceptor get metadata")
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		log.Errorln("authInterceptor no metadata")
		return nil, errors.New("authInterceptor: no metadata found")
	}

	log.Debugln("authInterceptor get authHeader")
	authHeader, ok := md["authorization"]
	if !ok || len(authHeader) == 0 {
		log.Errorln("authInterceptor no authHeader", authHeader)
		return nil, errors.New("authInterceptor: authorization header not found")
	}

	log.Debugln("authInterceptor VerifyIDToken")
	tok, err := c.auth.VerifyIDToken(ctx, authHeader[0])
	if err != nil {
		log.Errorln("authInterceptor VerifyIDToken", err)
		return nil, err
	}

	log.Debugln("authInterceptor GetUser")
	user, err := c.auth.GetUser(ctx, tok.UID)
	if err != nil {
		log.Errorln("authInterceptor GetUser", err)
		return nil, err
	}

	log.Debugln("authInterceptor check user")
	if _, ok := Emails[user.Email]; !ok {
		log.Errorln("User", user.Email, "not is authorized set")
	}

	log.Infoln("authInterceptor authorized")
	return handler(ctx, r)
}

func (c *Client) Decode(ctx context.Context, obj string, d interface{}) error {
	log.Debugln("Decoding", obj)
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
	log.Debugln("Encoding", obj)
	w := c.buck.Object(obj).NewWriter(ctx)
	defer w.Close()
	err := json.NewEncoder(w).Encode(d)
	if err != nil {
		log.Errorln("Encode json for", obj, err)
	}
	return err
}

func (c *Client) saveInsta(ctx context.Context) {
	log.Infoln("saveInsta starting")

	log.Debugln("saveInsta exporting goinsta to goinsta.state")
	err := c.insta.Export("goinsta.state")
	if err != nil {
		log.Errorln("saveInsta export goinsta to goinsta.state", err)
	}

	log.Debugln("saveInsta opening goinsta.state")
	f, err := os.Open("goinsta.state")
	if err != nil {
		log.Errorln("saveInsta opening goinsta.state", err)
	}
	defer f.Close()

	log.Debugln("saveInsta writing goinsta state to Storage")
	w := c.buck.Object(c.statefile).NewWriter(ctx)
	defer w.Close()
	_, err = io.Copy(w, f)
	if err != nil {
		log.Errorln("saveInsta writing state to storage", err)
	}
}
