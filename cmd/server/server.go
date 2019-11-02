package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/seankhliao/iglog"
	log "github.com/sirupsen/logrus"

	tapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type Server struct {
	bot   *tapi.BotAPI
	store *storage.Client

	sends chan tapi.MessageConfig

	Chats
}

func NewServer(ctx context.Context) (*Server, error) {
	bot, err := tapi.NewBotAPI(APIToken)
	if err != nil {
		return nil, fmt.Errorf("NewServer bot: %v", err)
	}

	store, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("NewServer storage: %v", err)
	}

	s := &Server{
		bot:   bot,
		store: store,
		sends: make(chan tapi.MessageConfig, 10),
		Chats: NewChats(),
	}

	r, err := store.Bucket(Bucket).Object(SaveFile).NewReader(ctx)
	if err == nil {
		defer r.Close()
		err = json.NewDecoder(r).Decode(s)
		if err != nil {
			log.Errorln("NewServer decode from storage:", err)
		}
	}
	return s, nil
}

func (s *Server) Export(ctx context.Context) {
	w := s.store.Bucket(Bucket).Object(SaveFile).NewWriter(ctx)
	defer w.Close()

	if err := json.NewEncoder(w).Encode(s); err != nil {
		log.Errorln("Server.Export:", err)
	}
}

func (s *Server) Sender() {
	for m := range s.sends {
		if _, err := s.bot.Send(m); err != nil {
			log.Errorln("Server.Sends:", err)
		}
	}

}

func (s *Server) Respond() {
	ups, err := s.bot.GetUpdatesChan(tapi.NewUpdate(0))
	if err != nil {
		log.Fatal("Server.Respond get updates chan:", err)
	}
	for up := range ups {
		if up.Message == nil {
			continue
		}

		ss := strings.Fields(up.Message.Text)
		if len(ss) == 0 {
			continue
		}

		var txt string
		cid := up.Message.Chat.ID
		switch strings.TrimPrefix(strings.ToLower(ss[0]), "/") {
		case "login":
			if len(ss) < 3 {
				txt = "Please provide a username and password"
			} else {
				txt = "Logged in successfully"
				err := s.Chats.login(cid, ss[1], ss[2])
				if err != nil {
					txt = err.Error()
				}
			}

		case "logout":
			txt = "Logged out successfully"
			err := s.Chats.logout(cid)
			if err != nil {
				txt = err.Error()
			}

		case "me", "whoami":
			if ud, ok := s.Chats[cid]; !ok {
				txt = "Please login first"
			} else {
				txt = fmt.Sprintf("You are @%s %s", ud.IG.Account.Username, ud.IG.Account.FullName)
			}

		case "followers":
			if ud, ok := s.Chats[cid]; !ok {
				txt = "Please login first"
			} else {
				for _, msg := range ud.ListFollowers().Strings() {
					s.sends <- tapi.NewMessage(cid, msg)
				}
			}

		case "following":
			if ud, ok := s.Chats[cid]; !ok {
				txt = "Please login first"
			} else {
				for _, msg := range ud.ListFollowing().Strings() {
					s.sends <- tapi.NewMessage(cid, msg)
				}
			}

		case "mutual":
			if ud, ok := s.Chats[cid]; !ok {
				txt = "Please login first"
			} else {
				for _, msg := range ud.Mutual.List().Strings() {
					s.sends <- tapi.NewMessage(cid, msg)
				}
			}

		case "notfollower":
			if ud, ok := s.Chats[cid]; !ok {
				txt = "Please login first"
			} else {
				for _, msg := range ud.NotFollower.List().Strings() {
					s.sends <- tapi.NewMessage(cid, msg)
				}
			}

		case "notfollowing":
			if ud, ok := s.Chats[cid]; !ok {
				txt = "Please login first"
			} else {
				for _, msg := range ud.NotFollowing.List().Strings() {
					s.sends <- tapi.NewMessage(cid, msg)
				}
			}

		case "export":
			if up.Message.From.UserName == "seankhliao" {
				s.Export(context.Background())
				txt = "export done"
			}

		case "start", "help":
			fallthrough
		default:
			txt = `Hello
I'm ig log
here's what I can do:

/login username password: login
/logout: logout
/help: show this message
/me: show who you are

/followers: list your followers
/following: list your following

/mutual: list people who follow you back
/notfollowing: list people you don't follow back
/notfollower: list people who don't follow back
                        `
		}
		if txt != "" {
			s.sends <- tapi.NewMessage(cid, txt)
		}
	}

}

func (s *Server) Update() {
	for cid, ud := range s.Chats {
		go func(cid int64, ud *iglog.UserData) {
			evs, err := ud.Update()
			if err != nil {
				log.Errorln("Server.Update:", err)
				return
			}
			for _, e := range evs {
				s.sends <- tapi.NewMessage(cid, e.String())
			}
			ud.Events = append(ud.Events, evs...)
		}(cid, ud)
	}
}

type Chats map[int64]*iglog.UserData

func NewChats() Chats {
	return make(Chats)
}

func (c Chats) login(cid int64, user, pass string) error {
	if _, ok := c[cid]; !ok {
		var err error
		c[cid], err = iglog.NewUserData(user, pass)
		if err != nil {
			return fmt.Errorf("Chats.login: %v", err)
		}
		_, err = c[cid].Update()
		if err != nil {
			return fmt.Errorf("Chats.login update: %v", err)
		}
		return nil
	}
	return fmt.Errorf("You are logged in, please logout first")
}

func (c Chats) logout(cid int64) error {
	defer delete(c, cid)
	if ud, ok := c[cid]; ok {
		if ud.IG != nil {
			err := ud.IG.Logout()
			if err != nil {
				return fmt.Errorf("Chats.logout: %v", err)
			}
			return nil
		}
	}
	return fmt.Errorf("You are not logged in")
}
