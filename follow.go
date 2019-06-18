package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ahmdrz/goinsta"
	log "github.com/sirupsen/logrus"
)

func (c *Client) FollowDiff(ctx context.Context) {
	var err error
	oldFollowers, oldFollowing := map[int64]goinsta.User{}, map[int64]goinsta.User{}

	log.Infoln("FollowDiff restoring oldFollowers")
	if err = c.Decode(ctx, "followers.json", &oldFollowers); err != nil {
		log.Errorln("FollowDiff restore oldFollowers", err)
	}

	log.Infoln("FollowDiff restoring oldFollowing")
	if err = c.Decode(ctx, "following.json", &oldFollowing); err != nil {
		log.Errorln("FollowDiff restore oldFollowing", err)
	}

	if c.followEvents == nil {
		log.Infoln("FollowDiff restoring followEvents")
		if err = c.Decode(ctx, "followEvents.json", &c.followEvents); err != nil {
			log.Errorln("FollowDiff restore followEvents", err)
		}
	}

	log.Infoln("FollowDiff start diffFollows")
	if c.followDiff, err = diffFollows(c.insta.Account, oldFollowers, oldFollowing); err != nil {
		log.Errorln("FollowDiff diffFollows", err)
		return
	}

	oe, oi := len(oldFollowers), len(oldFollowing)
	ne, ni := len(c.followDiff.followers), len(c.followDiff.following)
	ge, gi := len(c.followDiff.GainedFollowers), len(c.followDiff.GainedFollowing)
	le, li := len(c.followDiff.LostFollowers), len(c.followDiff.LostFollowing)
	fmt.Println("old followers:", oe, "old following:", oi)
	fmt.Println("new followers:", ne, "new following:", ni)
	fmt.Println("gained followers:", ge, "lost followers:", le)
	fmt.Println("gained following:", gi, "lost following:", li)

	if ge+gi+le+li != 0 {
		log.Infoln("FollowDiff saving followEvents")
		c.followEvents = append(c.followEvents, FollowEvent{time.Now(), c.followDiff})
		if err = c.Encode(ctx, "followEvents.json", c.followEvents); err != nil {
			log.Errorln("FollowDiff save followEvents", err)
		}
	}

	log.Infoln("FollowDiff saving followers")
	if err = c.Encode(ctx, "followers.json", c.followDiff.followers); err != nil {
		log.Errorln("FollowDiff save followers", err)
	}

	log.Infoln("FollowDiff saving following")
	if err = c.Encode(ctx, "following.json", c.followDiff.following); err != nil {
		log.Errorln("FollowDiff save following", err)
	}
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
