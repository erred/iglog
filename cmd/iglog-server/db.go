package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/cockroach-go/v2/crdb"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/log/zerologadapter"
	"github.com/jackc/pgx/v4/pgxpool"
)

func ExecuteTx(ctx context.Context, pool *pgxpool.Pool, txOpts pgx.TxOptions, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, txOpts)
	if err != nil {
		return err
	}
	return crdb.ExecuteInTx(ctx, pgxTxAdapter{tx}, func() error { return fn(tx) })
}

type pgxTxAdapter struct {
	pgx.Tx
}

func (tx pgxTxAdapter) Exec(ctx context.Context, q string, args ...interface{}) error {
	_, err := tx.Tx.Exec(ctx, q, args...)
	return err
}

func (s *Server) dbSetup(ctx context.Context) error {
	// connection pool
	config, err := pgxpool.ParseConfig(s.dsn)
	if err != nil {
		return fmt.Errorf("dbSetup parse dsn=%s: %w", s.dsn, err)
	}
	config.ConnConfig.Logger = zerologadapter.NewLogger(s.log)
	s.pool, err = pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("dbSetup connect dsn=%s: %w", s.dsn, err)
	}

	// create tables
	err = ExecuteTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS goinsta (
	id INTEGER,
	state JSONB,
	timestamp TIMESTAMP
)`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users (
	uid INTEGER PRIMARY KEY,
	following BOOL,
	follower BOOL,
	username TEXT,
	data JSONB
)`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS events (
	timestamp TIMESTAMP,
	event STRING,
	uid INTEGER
)`)
		if err != nil {
			return err
		}

		// TODO: insert initial data
		row := tx.QueryRow(ctx, `SELECT state FROM goinsta WHERE id = 1 LIMIT 1`)
		err = row.Scan(s.IG)
		if errors.Is(err, pgx.ErrNoRows) {
			f, err := os.Open(s.initstate)
			if err != nil {
				return err
			}
			defer f.Close()
			err = json.NewDecoder(f).Decode(s.IG)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO goinsta VALUES (1, $1, $2)`, s.IG, time.Now().Add(-s.interval))
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		row = tx.QueryRow(ctx, `SELECT count(uid) FROM users WHERE follower = true`)
		err = row.Scan(&s.followers)
		if err != nil {
			return err
		}

		row = tx.QueryRow(ctx, `SELECT count(uid) FROM users WHERE following = true`)
		err = row.Scan(&s.following)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("dbSetup ensure tables: %w", err)
	}
	return nil
}
