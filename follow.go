package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ahmdrz/goinsta"
	log "github.com/sirupsen/logrus"
)

func (c *Client) TickFollowDiff(ctx context.Context, d time.Duration) {
	c.FollowDiff(ctx)
	t := time.NewTicker(d)
	for range t.C {
		c.FollowDiff(ctx)
	}
}

func (c *Client) FollowDiff(ctx context.Context) {
	var err error

	log.Infoln("FollowDiff restoring oldFollowers")
	oldFollowers := make(map[int64]goinsta.User)
	br, err := c.buck.Object("followers.json").NewReader(ctx)
	if err != nil {
		log.Errorln("FollowDiff get followers reader", err)
	} else {
		err = json.NewDecoder(br).Decode(&oldFollowers)
		if err != nil {
			log.Errorln("FollowDiff restore oldFollowers", err)
		}
		br.Close()
	}

	log.Infoln("FollowDiff restoring oldFollowing")
	oldFollowing := make(map[int64]goinsta.User)
	br, err = c.buck.Object("following.json").NewReader(ctx)
	if err != nil {
		log.Errorln("FollowDiff get following reader", err)
	} else {
		err = json.NewDecoder(br).Decode(&oldFollowing)
		if err != nil {
			log.Errorln("FollowDiff restore oldFollowing", err)
		}
		br.Close()
	}

	log.Infoln("FollowDiff restoring followEvents")
	var followEvents FollowEvents
	br, err = c.buck.Object("followEvents.json").NewReader(ctx)
	if err != nil {
		log.Errorln("FollowDiff get followEvents reader", err)
	} else {
		err = json.NewDecoder(br).Decode(&followEvents)
		if err != nil {
			log.Errorln("FollowDiff restore followEvents")
		}
		br.Close()
	}

	log.Infoln("FollowDiff start diffFollows")
	fd, err := diffFollows(c.insta.Account, oldFollowers, oldFollowing)
	if err != nil {
		log.Errorln("FollowDiff diffFollows", err)
	}

	fmt.Println("old followers:", len(oldFollowers), "old following:", len(oldFollowing))
	fmt.Println("new followers:", len(fd.followers), "new following:", len(fd.following))
	fmt.Println("gained followers:", len(fd.GainedFollowers), "lost followers:", len(fd.LostFollowers))
	fmt.Println("gained following:", len(fd.GainedFollowing), "lost following:", len(fd.LostFollowing))

	followEvents = append(followEvents, FollowEvent{time.Now(), fd})

	log.Infoln("FollowDiff saving followers")
	bw := c.buck.Object("followers.json").NewWriter(ctx)
	e := json.NewEncoder(bw)
	e.SetIndent("", "\t")
	err = e.Encode(fd.followers)
	if err != nil {
		log.Errorln("FollowDiff save followers", err)
	}
	bw.Close()

	log.Infoln("FollowDiff saving following")
	bw = c.buck.Object("following.json").NewWriter(ctx)
	e = json.NewEncoder(bw)
	e.SetIndent("", "\t")
	err = e.Encode(fd.following)
	if err != nil {
		log.Errorln("FollowDiff save following", err)
	}
	bw.Close()

	log.Infoln("FollowDiff saving followEvents")
	bw = c.buck.Object("followEvents.json").NewWriter(ctx)
	e = json.NewEncoder(bw)
	e.SetIndent("", "\t")
	err = e.Encode(followEvents)
	if err != nil {
		log.Errorln("FollowDiff save followEvents", err)
	}
	bw.Close()
}

type FollowEvents []FollowEvent
type FollowEvent struct {
	TimeStamp time.Time
	*FollowDiff
}

type Users map[int64]goinsta.User

type FollowDiff struct {
	followers, following Users
	GainedFollowers      Users
	LostFollowers        Users
	GainedFollowing      Users
	LostFollowing        Users
}

func diffFollows(a *goinsta.Account, oldFollowers, oldFollowing Users) (*FollowDiff, error) {
	var err error
	fd := &FollowDiff{}
	fd.followers, err = getUsers(a.Followers())
	if err != nil {
		return nil, fmt.Errorf("diffFollows get followers: %v", err)
	}
	fd.following, err = getUsers(a.Following())
	if err != nil {
		return nil, fmt.Errorf("diffFollows get following: %v", err)
	}
	fd.GainedFollowers, fd.LostFollowers = diffUsers(fd.followers, oldFollowers)
	fd.GainedFollowing, fd.LostFollowing = diffUsers(fd.following, oldFollowing)
	return fd, nil
}

// getUsers pages through a users refernce and returns all of them
func getUsers(us *goinsta.Users) (Users, error) {
	m := map[int64]goinsta.User{}
	for us.Next() {
		for _, u := range us.Users {
			m[u.ID] = u
		}

		if err := us.Error(); err == goinsta.ErrNoMore {
			break
		} else if err != nil {
			return nil, fmt.Errorf("getUsers next err: %v", err)
		}
	}
	return m, nil
}

// diffUsers returns the difference between 2 sets of users
func diffUsers(users1, users2 Users) (only1, only2 Users) {
	only1, only2 = make(map[int64]goinsta.User), make(map[int64]goinsta.User)
	for id, u := range users1 {
		if _, ok := users2[id]; !ok {
			only1[id] = u
		}
	}
	for id, u := range users2 {
		if _, ok := users1[id]; !ok {
			only2[id] = u
		}
	}
	return only1, only2
}
