package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ahmdrz/goinsta"
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
		go func(cid int64, ud *UserData) {
			evs, err := ud.update()
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

type Chats map[int64]*UserData

func NewChats() Chats {
	return make(Chats)
}

func (c Chats) login(cid int64, user, pass string) error {
	if _, ok := c[cid]; !ok {
		var err error
		c[cid], err = NewUserData(user, pass)
		if err != nil {
			return fmt.Errorf("Chats.login: %v", err)
		}
		_, err = c[cid].update()
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

type IG struct {
	*goinsta.Instagram
}

func (i *IG) MarshalJSON() ([]byte, error) {
	b := &bytes.Buffer{}
	err := goinsta.Export(i.Instagram, b)
	if err != nil {
		err = fmt.Errorf("IG MarshalJSON: %v", err)
	}
	return b.Bytes(), err
}

func (i *IG) UnmarshalJSON(b []byte) error {
	var err error
	i.Instagram, err = goinsta.ImportReader(bytes.NewReader(b))
	if err != nil {
		err = fmt.Errorf("IG UnmarshalJSON: %v", err)
	}
	return err
}

type UserData struct {
	IG *IG

	Events       Events
	NotFollower  RawUsers
	Mutual       RawUsers
	NotFollowing RawUsers
}

func NewUserData(user, pass string) (*UserData, error) {
	ig := goinsta.New(user, pass)
	if err := ig.Login(); err != nil {
		return nil, fmt.Errorf("NewUserData login: %v", err)
	}

	return &UserData{
		IG: &IG{ig},

		Events:       NewEvents(),
		NotFollower:  NewRawUsers(),
		Mutual:       NewRawUsers(),
		NotFollowing: NewRawUsers(),
	}, nil
}

func (u UserData) ListFollowers() Users {
	var us Users
	for _, gu := range u.Mutual {
		us = append(us, NewUser(gu))
	}
	for _, gu := range u.NotFollowing {
		us = append(us, NewUser(gu))
	}
	sort.Slice(us, func(i, j int) bool {
		return us[i].Username < us[j].Username
	})
	return us
}

func (u UserData) ListFollowing() Users {
	var us Users
	for _, gu := range u.NotFollower {
		us = append(us, NewUser(gu))
	}
	for _, gu := range u.Mutual {
		us = append(us, NewUser(gu))
	}
	sort.Slice(us, func(i, j int) bool {
		return us[i].Username < us[j].Username
	})
	return us
}

func (u *UserData) update() (Events, error) {
	newFollowers, err := getUsers(u.IG.Account.Followers())
	if err != nil {
		return nil, fmt.Errorf("UserData.update get followers: %v", err)
	}
	newFollowing, err := getUsers(u.IG.Account.Following())
	if err != nil {
		return nil, fmt.Errorf("UserData.update get following: %v", err)
	}

	oldFollowers, oldFollowing := NewRawUsers(), NewRawUsers()
	for id, uu := range u.Mutual {
		oldFollowers[id], oldFollowing[id] = uu, uu
	}
	for id, uu := range u.NotFollowing {
		oldFollowers[id] = uu
	}
	for id, uu := range u.NotFollower {
		oldFollowing[id] = uu
	}

	evs := NewEvents()
	egfwer, _, elfwer := intersect(newFollowers, oldFollowers)
	for _, uu := range egfwer {
		evs = evs.Add(uu, FollowerGained)
	}
	for _, uu := range elfwer {
		evs = evs.Add(uu, FollowerLost)
	}

	egfwing, _, elfwing := intersect(newFollowing, oldFollowing)
	for _, uu := range egfwing {
		evs = evs.Add(uu, FollowingGained)
	}
	for _, uu := range elfwing {
		evs = evs.Add(uu, FollowingLost)
	}

	u.NotFollowing, u.Mutual, u.NotFollower = intersect(newFollowers, newFollowing)
	return evs, nil
}

func getUsers(u *goinsta.Users) (RawUsers, error) {
	us := NewRawUsers()
	for u.Next() {
		for _, uu := range u.Users {
			us[uu.ID] = uu
		}

		if err := u.Error(); err == goinsta.ErrNoMore {
			break
		} else if err != nil {
			return nil, fmt.Errorf("getUsers: %v", err)
		}
	}
	return us, nil
}

type RawUsers map[int64]goinsta.User

func NewRawUsers() RawUsers {
	return make(RawUsers)
}
func (u RawUsers) List() Users {
	var us Users
	for _, gu := range u {
		us = append(us, NewUser(gu))
	}
	sort.Slice(us, func(i, j int) bool {
		return us[i].Username < us[j].Username
	})
	return us
}

func intersect(one, two RawUsers) (only1, both, only2 RawUsers) {
	only1, both, only2 = NewRawUsers(), NewRawUsers(), NewRawUsers()
	for id, u := range one {
		if _, ok := two[id]; ok {
			both[id] = u
		} else {
			only1[id] = u
		}
	}
	for id, u := range two {
		if _, ok := one[id]; !ok {
			only2[id] = u
		}
	}
	return only1, both, only2
}

type Users []User

func NewUsers() Users {
	return nil
}
func (u Users) Sort() {
	sort.Slice(u, func(i, j int) bool {
		return u[i].Username < u[j].Username
	})
}
func (u Users) Strings() []string {
	var ss []string
	ss = append(ss, "Total: "+strconv.Itoa(len(u))+"\n")
	var sss string
	for i, us := range u {
		sss += "@" + us.Username + ": " + us.Name + "\n"
		if i%10 == 9 {
			ss = append(ss, sss)
			sss = ""
		}
	}
	if sss != "" {
		ss = append(ss, sss)
	}
	return ss
}

type User struct {
	Username string
	Name     string
}

func NewUser(gu goinsta.User) User {
	return User{
		gu.Username,
		gu.FullName,
	}
}

type Events []Event

func NewEvents() Events {
	return nil
}

func (e Events) Add(u goinsta.User, ev EventType) Events {
	return append(e, Event{
		time.Now(), ev, u,
	})
}

type Event struct {
	T time.Time
	E EventType
	U goinsta.User
}

func (e Event) String() string {
	ev := " unknown"
	switch e.E {
	case FollowerGained:
		ev = "+follower"
	case FollowerLost:
		ev = "-follower"
	case FollowingGained:
		ev = "+following"
	case FollowingLost:
		ev = "-following"
	}
	return fmt.Sprintf("%s %s @%s %s", e.T.Format("2006-01-02 15:04 -0700"), ev, e.U.Username, e.U.FullName)
}

type EventType int

const (
	FollowerGained EventType = iota
	FollowerLost
	FollowingGained
	FollowingLost
)
