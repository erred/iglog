package main

import (
	"context"
	"time"

	"github.com/ahmdrz/goinsta"
	"github.com/seankhliao/iglog/iglog"
	log "github.com/sirupsen/logrus"
)

func (c *Client) EventLog(ctx context.Context, r *iglog.Request) (*iglog.Events, error) {
	log.Infoln("EventLog started")
	evs := &iglog.Events{}
	for _, e := range c.followEvents {
		for _, u := range e.GainedFollowers {
			evs.Events = append(evs.Events, &iglog.Event{
				Time: e.TimeStamp.Format(time.RFC3339),
				Type: iglog.EventType_FollowerGained,
				User: user2proto(u),
			})
		}
		for _, u := range e.LostFollowers {
			evs.Events = append(evs.Events, &iglog.Event{
				Time: e.TimeStamp.Format(time.RFC3339),
				Type: iglog.EventType_FollowerLost,
				User: user2proto(u),
			})
		}
		for _, u := range e.GainedFollowing {
			evs.Events = append(evs.Events, &iglog.Event{
				Time: e.TimeStamp.Format(time.RFC3339),
				Type: iglog.EventType_FollowingGained,
				User: user2proto(u),
			})
		}
		for _, u := range e.LostFollowing {
			evs.Events = append(evs.Events, &iglog.Event{
				Time: e.TimeStamp.Format(time.RFC3339),
				Type: iglog.EventType_FollowingLost,
				User: user2proto(u),
			})
		}
	}
	log.Infoln("EventLog done")
	return evs, nil
}
func (c *Client) Followers(ctx context.Context, r *iglog.Request) (*iglog.Users, error) {
	log.Infoln("Followers started")
	us := &iglog.Users{}
	us.Users = make([]*iglog.User, len(c.followDiff.followers))
	for i, u := range c.followDiff.followers {
		us.Users[i] = user2proto(u)
	}
	log.Infoln("Followers done")
	return us, nil
}

func (c *Client) Following(ctx context.Context, r *iglog.Request) (*iglog.Users, error) {
	log.Infoln("Following started")
	us := &iglog.Users{}
	us.Users = make([]*iglog.User, len(c.followDiff.following))
	for i, u := range c.followDiff.following {
		us.Users[i] = user2proto(u)
	}
	log.Infoln("Following done")
	return us, nil
}

func user2proto(u goinsta.User) *iglog.User {
	return &iglog.User{
		Id:          u.ID,
		Username:    u.Username,
		Displayname: u.FullName,
	}
}
