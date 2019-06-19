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
	if c.followEvents == nil {
		log.Errorln("EventLog not initialized")
		return evs, nil
	}
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
	if c.followDiff == nil || c.followDiff.followers == nil {
		log.Errorln("Followers not initialized")
		return us, nil
	}
	for _, u := range c.followDiff.followers {
		us.Users = append(us.Users, user2proto(u))
	}
	log.Infoln("Followers done")
	return us, nil
}

func (c *Client) Following(ctx context.Context, r *iglog.Request) (*iglog.Users, error) {
	log.Infoln("Following started")
	us := &iglog.Users{}
	if c.followDiff == nil || c.followDiff.following == nil {
		log.Errorln("Following not initialized")
		return us, nil
	}
	for _, u := range c.followDiff.following {
		us.Users = append(us.Users, user2proto(u))
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
