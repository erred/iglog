package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/ahmdrz/goinsta/v2"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/api/metric"
	"go.opentelemetry.io/otel/api/trace"
	"go.seankhliao.com/usvc"
)

const (
	name = "go.seankhliao.com/iglog"
)

func main() {
	usvc.Run(context.Background(), name, &Server{}, true)
}

type Server struct {
	initstate string
	IG        *IG
	interval  time.Duration
	dsn       string
	pool      *pgxpool.Pool

	log    zerolog.Logger
	tracer trace.Tracer

	mu        sync.Mutex
	following int64
	followers int64
}

func (s *Server) Flag(fs *flag.FlagSet) {
	fs.StringVar(&s.dsn, "db", "", "connection string for pgx")
	fs.StringVar(&s.initstate, "initstate", "/var/secret/iglog/iglog.json", "initial state file")
	fs.DurationVar(&s.interval, "interval", 15*time.Minute, "update interval")
}

func (s *Server) Register(c *usvc.Components) error {
	s.log = c.Log
	s.tracer = c.Tracer

	sname := path.Base(name) + "."

	var (
		err       error
		followers metric.Int64ValueObserver
		following metric.Int64ValueObserver
	)

	bo := c.Meter.NewBatchObserver(func(ctx context.Context, bor metric.BatchObserverResult) {
		s.mu.Lock()
		defer s.mu.Unlock()
		bor.Observe(
			nil,
			followers.Observation(s.followers),
			following.Observation(s.following),
		)
	})
	followers, err = bo.NewInt64ValueObserver(
		sname+"followers",
		metric.WithDescription("number of instagram followers"),
	)
	if err != nil {
		return fmt.Errorf("create followers metric: %w", err)
	}
	following, err = bo.NewInt64ValueObserver(
		sname+"following",
		metric.WithDescription("number of instagram following"),
	)
	if err != nil {
		return fmt.Errorf("create following metric: %w", err)
	}

	err = s.dbSetup(context.Background())
	if err != nil {
		return fmt.Errorf("setup db: %w", err)
	}

	go s.updater(context.Background())
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.pool.Close()
	return nil
}

func (s *Server) updater(ctx context.Context) {
	s.log.Info().Dur("interval", s.interval).Msg("starting updater")
	for {
		err := s.update(ctx)
		if err != nil {
			s.log.Error().Err(err).Msg("update")
		}
		time.Sleep(s.interval)
	}
}

func (s *Server) update(ctx context.Context) error {
	s.log.Info().Msg("starting update")
	// get users
	newFollowers, err := getUsersPage(s.IG.Account.Followers())
	if err != nil {
		return fmt.Errorf("update get followers: %w", err)
	}
	newFollowing, err := getUsersPage(s.IG.Account.Following())
	if err != nil {
		return fmt.Errorf("update get following: %w", err)
	}
	s.log.Info().Int("followers", len(newFollowers)).Int("following", len(newFollowing)).Msg("update got from ig")

	s.mu.Lock()
	s.followers = int64(len(newFollowers))
	s.following = int64(len(newFollowing))
	s.mu.Unlock()

	err = ExecuteTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// save state
		_, err := tx.Exec(ctx, `UPDATE goinsta SET state = $1, timestamp = $2 WHERE id = 1;`, s.IG, time.Now())
		if err != nil {
			return fmt.Errorf("update state: %w", err)
		}

		// get old users
		oldFollowers, err := getUsersDB(ctx, tx, true, false)
		if err != nil {
			return fmt.Errorf("get old followers: %w", err)
		}

		oldFollowing, err := getUsersDB(ctx, tx, false, true)
		if err != nil {
			return fmt.Errorf("get old following: %w", err)
		}

		// diff users
		lostFollowers, _, gainedFollowers := intersect(oldFollowers, newFollowers)
		lostFollowing, _, gainedFollowing := intersect(oldFollowing, newFollowing)
		s.log.Info().Strs("follower-", usernames(lostFollowers)).Strs("follower+", usernames(gainedFollowers)).
			Strs("following-", usernames(lostFollowing)).Strs("following+", usernames(gainedFollowing)).Msg("diff")

		// save users
		err = upsertUsersDB(ctx, tx, newFollowers, true, false)
		if err != nil {
			return fmt.Errorf("update followers: %w", err)
		}
		err = upsertUsersDB(ctx, tx, newFollowing, false, true)
		if err != nil {
			return fmt.Errorf("update following: %w", err)
		}

		// save events
		err = insertEvents(ctx, tx, []map[int64]goinsta.User{lostFollowers, gainedFollowers, lostFollowing, gainedFollowing})
		if err != nil {
			return fmt.Errorf("update events: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("update db: %w", err)
	}
	s.log.Info().Msg("update complete")
	return nil
}

// IG holds authorization to use the Instagram api
type IG struct {
	*goinsta.Instagram
}

func (i *IG) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	err := goinsta.Export(i.Instagram, &b)
	if err != nil {
		return nil, fmt.Errorf("IG.MarshalJSON: %v", err)
	}
	return b.Bytes(), nil
}

func (i *IG) UnmarshalJSON(b []byte) error {
	var err error
	i.Instagram, err = goinsta.ImportReader(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("IG.UnmarshalJSON: %v", err)
	}
	return nil
}

func getUsersPage(u *goinsta.Users) (map[int64]goinsta.User, error) {
	users := make(map[int64]goinsta.User)
	for u.Next() {
		for _, uu := range u.Users {
			users[uu.ID] = uu
		}
		err := u.Error()
		if errors.Is(err, goinsta.ErrNoMore) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("getUsers: %v", err)
		}
	}
	return users, nil
}

func getUsersDB(ctx context.Context, tx pgx.Tx, followers, following bool) (map[int64]goinsta.User, error) {
	sqlstr := `SELECT uid, data FROM users WHERE %s = true`
	if followers {
		sqlstr = fmt.Sprintf(sqlstr, "follower")
	} else if following {
		sqlstr = fmt.Sprintf(sqlstr, "following")
	} else {
		return nil, fmt.Errorf("getUsersDB unknown combo following=%v followers=%v", following, followers)
	}
	rows, err := tx.Query(ctx, sqlstr)
	if err != nil {
		return nil, fmt.Errorf("getUsersDB query following=%v followers=%v: %w", following, followers, err)
	}
	defer rows.Close()

	users := make(map[int64]goinsta.User, 300)
	for rows.Next() {
		var user int64
		var state goinsta.User
		err := rows.Scan(&user, &state)
		if err != nil {
			return nil, fmt.Errorf("getUsersDB scan: %w", err)
		}
		users[user] = state
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return users, nil
}

func upsertUsersDB(ctx context.Context, tx pgx.Tx, users map[int64]goinsta.User, followers, following bool) error {
	sqlstr := `UPSERT INTO users (uid, %s, username, data) VALUES ($1, $2, $3, $4)`
	if followers {
		sqlstr = fmt.Sprintf(sqlstr, "follower")
	} else if following {
		sqlstr = fmt.Sprintf(sqlstr, "following")
	} else {
		return fmt.Errorf("upsertUsersDB unknown combo following=%v followers=%v", following, followers)
	}
	for uid, user := range users {
		_, err := tx.Exec(ctx, sqlstr, uid, true, user.Username, user)
		if err != nil {
			return fmt.Errorf("upsertUsersDB exec: %w", err)
		}
	}
	return nil
}

func insertEvents(ctx context.Context, tx pgx.Tx, events []map[int64]goinsta.User) error {
	sqlstr := `INSERT INTO events (timestamp, event, uid) VALUES ($1, $2, $3)`
	order := []string{"- follower", "+ follower", "- following", "+ following"}
	for i, event := range events {
		for uid := range event {
			_, err := tx.Exec(
				ctx,
				sqlstr,
				time.Now(),
				order[i],
				uid,
			)
			if err != nil {
				return fmt.Errorf("insertEvents %s: %w", order[i], err)
			}
		}
	}
	return nil
}

func intersect(one, two map[int64]goinsta.User) (only1, both, only2 map[int64]goinsta.User) {
	only1, both, only2 = make(map[int64]goinsta.User), make(map[int64]goinsta.User), make(map[int64]goinsta.User)
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

func usernames(us map[int64]goinsta.User) []string {
	uns := make([]string, 0, len(us))
	for _, v := range us {
		uns = append(uns, v.Username)
	}
	return uns
}
