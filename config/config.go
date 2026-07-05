package config

import (
	"io/ioutil"
	"os"

	"github.com/naoina/toml"
)

type Config struct {
	LogFile string

	TaskDir string

	Database struct {
		Connection     string
		MaxConnections int
		SafeReinit     bool
	}

	Scoreboard struct {
		WwwPath       string
		TemplatePath  string
		Addr          string
		RecalcTimeout _duration
		UnderProxy    bool
	}

	WebsocketTimeout struct {
		Info       _duration
		Scoreboard _duration
		Tasks      _duration
	}

	TaskPrice struct {
		UseNonLinear           bool
		UseTeamsBase           bool
		TeamsBase              int
		P500, P400, P300, P200 int
	}

	Game struct {
		Start _time
		End   _time
	}

	Flag struct {
		SendTimeout _duration
	}

	Task struct {
		OpenTimeout _duration

		AutoOpen        bool
		AutoOpenTimeout _duration
	}

	Teams []struct {
		Name        string
		Description string
		Token       string
		Test        bool
	}

	Admin struct {
		Enabled bool

		Token string
	}
}

func ReadConfig(path string) (cfg Config, err error) {

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	buf, err := ioutil.ReadAll(f)
	if err != nil {
		return
	}

	err = toml.Unmarshal(buf, &cfg)
	if err != nil {
		return
	}

	return
}
