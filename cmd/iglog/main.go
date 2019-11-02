package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/seankhliao/iglog"
	"golang.org/x/crypto/ssh/terminal"
)

func initLog() {
	logfmt := os.Getenv("LOGFMT")
	if logfmt != "json" {
		logfmt = "text"
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, NoColor: !terminal.IsTerminal(int(os.Stdout.Fd()))})
	}

	level, _ := zerolog.ParseLevel(os.Getenv("LOGLVL"))
	if level == zerolog.NoLevel {
		level = zerolog.InfoLevel
	}
	log.Info().Str("FMT", logfmt).Str("LVL", level.String()).Msg("log initialized")
	zerolog.SetGlobalLevel(level)
}

func main() {
	initLog()

	var user, pass string
	flag.StringVar(&user, "user", "", "username to login")
	flag.StringVar(&pass, "pass", "", "passwd to login")
	flag.Parse()

	var ud *iglog.UserData
	var err error
	if user != "" && pass != "" {
		ud, err = iglog.NewUserData(user, pass)
		if err != nil {
			log.Fatal().Str("user", user).Err(err).Msg("failed to login")
		}
	} else {
		b, err := ioutil.ReadFile("iglog.json")
		if err != nil {
			log.Fatal().Str("file", "iglog.json").Err(err).Msg("failed to read")
		}
		err = json.Unmarshal(b, ud)
		if err != nil {
			log.Fatal().Str("obj", "ud").Err(err).Msg("failed to unmarshal")
		}
	}
	defer func() {
		b, err := json.Marshal(ud)
		if err != nil {
			log.Fatal().Str("obj", "ud").Err(err).Msg("failed to marshal")
		}
		err = ioutil.WriteFile("iglog.json", b, 0644)
		if err != nil {
			log.Fatal().Str("file", "iglog.json").Err(err).Msg("failed to write")
		}
	}()

	evs, err := ud.Update()
	if err != nil {
		log.Error().Err(err).Msg("update failed")
		return
	}
	s := evs.String()
	fmt.Printf("\nFollow events:\n%s\n", s)

	b, err := ioutil.ReadFile("events.log")
	if err != nil {
		log.Error().Str("file", "events.log").Err(err).Msg("failed to read file")
	}

	buf := bytes.NewBuffer(b)
	buf.WriteString(s)

	err = ioutil.WriteFile("events.log", buf.Bytes(), 0644)
	if err != nil {
		log.Error().Str("file", "events.log").Err(err).Msg("failed to write file")
	}
}
