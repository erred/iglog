package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ahmdrz/goinsta"
	log "github.com/sirupsen/logrus"
)

type FollowDiff struct {
	events               []FollowEvent
	followers, following Users
	GainedFollowers      Users
	LostFollowers        Users
	GainedFollowing      Users
	LostFollowing        Users
}

type FollowEvent struct {
	TimeStamp time.Time
	Type      FollowEventType
	User      goinsta.User
}

type Users map[int64]goinsta.User

type FollowEventType int

const (
	FEFollowerGained FollowEventType = iota
	FEFollowerLost
	FEFollowingGained
	FEFollowingLost
)

func (c *Client) FollowDiff(ctx context.Context) {
	log.Infoln("FollowDiff starting")
	var err error

	log.Debugln("FollowDiff get old FollowDiff")
	if c.fDiff == nil {
		log.Debugln("FollowDiff get old FollowDiff from storage")
		c.retrieveFollowDiff(ctx)
	}

	log.Debugln("FollowDiff get new FollowDiff")
	err = c.newFollowDiff(ctx)
	if err != nil {
		log.Errorln("FollowDiff get new FollowDiff", err)
		return
	}

	c.fDiff.print()
	c.followDiffProto()
	c.saveFollowDiff(ctx)
}

// RetrieveFollowDiff creates a followdiff by getting it from storage
func (c *Client) retrieveFollowDiff(ctx context.Context) {
	c.fDiff = &FollowDiff{}

	log.Debugln("NewFollowDiff restoring followers")
	c.fDiff.followers = make(map[int64]goinsta.User)
	if err := c.Decode(ctx, "followers.json", &c.fDiff.followers); err != nil {
		log.Errorln("NewFollowDiff restore followers", err)
	}

	log.Debugln("NewFollowDiff restoring following")
	c.fDiff.following = make(map[int64]goinsta.User)
	if err := c.Decode(ctx, "following.json", &c.fDiff.following); err != nil {
		log.Errorln("NewFollowDiff restore following", err)
	}

	log.Debugln("NewFollowDiff restoring followEvents")
	if err := c.Decode(ctx, "followEvents.json", &c.fDiff.events); err != nil {
		log.Errorln("FollowDiff restore followEvents", err)
	}
}

// NewFollowDiff creates a new FollowDiff by calling the instagram api
func (c *Client) newFollowDiff(ctx context.Context) error {
	var err error
	n := &FollowDiff{
		events: c.fDiff.events,
	}

	log.Debugln("NewFollowDiff get followers")
	n.followers, err = getUsers(c.insta.Account.Followers())
	if err != nil {
		return fmt.Errorf("NewFollowDiff get followers: %v", err)
	}

	log.Debugln("NewFollowDiff get following")
	n.following, err = getUsers(c.insta.Account.Following())
	if err != nil {
		return fmt.Errorf("NewFollowDiff get following: %v", err)
	}

	log.Debugln("NewFollowDiff diffUsers")
	n.GainedFollowers, n.LostFollowers = diffUsers(n.followers, c.fDiff.followers)
	n.GainedFollowing, n.LostFollowing = diffUsers(n.following, c.fDiff.following)

	log.Debugln("NewFollowDiff update events")
	for _, u := range n.GainedFollowers {
		n.events = append(n.events, FollowEvent{time.Now(), FEFollowerGained, u})
	}
	for _, u := range n.LostFollowers {
		n.events = append(n.events, FollowEvent{time.Now(), FEFollowerLost, u})
	}
	for _, u := range n.GainedFollowing {
		n.events = append(n.events, FollowEvent{time.Now(), FEFollowingGained, u})
	}
	for _, u := range n.LostFollowing {
		n.events = append(n.events, FollowEvent{time.Now(), FEFollowingLost, u})
	}

	c.fDiff = n
	return nil
}

func (c *Client) saveFollowDiff(ctx context.Context) {
	if len(c.fDiff.GainedFollowers)+len(c.fDiff.GainedFollowing)+len(c.fDiff.LostFollowers)+len(c.fDiff.LostFollowing) > 0 {
		log.Debugln("SaveFollowDiff save followEvents")
		err := c.Encode(ctx, "followEvents.json", c.fDiff.events)
		if err != nil {
			log.Errorln("SaveFollowDiff save followEvents.json", err)
		}
	}

	if len(c.fDiff.GainedFollowers)+len(c.fDiff.LostFollowers) > 0 {
		log.Debugln("SaveFollowDiff save followers")
		err := c.Encode(ctx, "followers.json", c.fDiff.followers)
		if err != nil {
			log.Errorln("SaveFollowDiff save followers.json", err)
		}
	}

	if len(c.fDiff.GainedFollowing)+len(c.fDiff.LostFollowing) > 0 {
		log.Debugln("SaveFollowDiff save following")
		err := c.Encode(ctx, "following.json", c.fDiff.following)
		if err != nil {
			log.Errorln("SaveFollowDiff save following.json", err)
		}
	}
}

func (f *FollowDiff) print() {
	fmt.Println("followers", len(f.followers), "following", len(f.following))
	fmt.Println("gained followers", len(f.GainedFollowers), "lost followers", len(f.LostFollowers))
	fmt.Println("started following", len(f.GainedFollowing), "stopped following", len(f.LostFollowing))
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
