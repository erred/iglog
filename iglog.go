package iglog

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"time"

	goinsta "github.com/ahmdrz/goinsta/v2"
)

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

func (u *UserData) Update() (Events, error) {
	newFollowers, err := getUsers(u.IG.Account.Followers())
	if err != nil {
		return nil, fmt.Errorf("UserData.Update get followers: %v", err)
	}
	newFollowing, err := getUsers(u.IG.Account.Following())
	if err != nil {
		return nil, fmt.Errorf("UserData.Update get following: %v", err)
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
