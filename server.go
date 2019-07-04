package main

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/storage"
	"github.com/ahmdrz/goinsta"
	log "github.com/sirupsen/logrus"

	tapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type Server struct {
	Bot   *tapi.BotAPI
	store *storage.Client

	Chats
}

func NewServer(bucket, fn, token string) (*Server, error) {
	ctx := context.Background()
	log.Debugln("NewServer starting")
	store, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("NewServer storage client: %v", err)
	}
	r, err := store.Bucket(Bucket).Object(fn).NewReader(context.Background())
	if err == nil {
		defer r.Close()
		s := &Server{}
		log.Debugln("NewServer restoring from storage", Bucket, fn)
		err = json.NewDecoder(r).Decode(s)
		if err == nil {
			log.Infoln("NewServer restored from storage")
			s.store = store
			return s, nil
		}
		log.Debugln("NewServer restoring decode", err)
	}
	log.Debugln("NewServer reader", err)
	log.Debugln("NewServer new tapi bot from token")
	bot, err := tapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("NewServer new bot %v", err)
	}
	log.Debugln("NewServer created new bot")
	return &Server{
		Bot:   bot,
		store: store,
		Chats: NewChats(),
	}, nil
}

type Chats map[int64]UserData

func NewChats() Chats {
	return make(Chats)
}

type UserData struct {
	IG        *goinsta.Instagram
	Followers Users
	Following Users
}
