package config

import (
	"strings"
	"time"
)

type _duration struct {
	time.Duration
}

func (d *_duration) UnmarshalTOML(data []byte) (err error) {
	duration := strings.Replace(string(data), "\"", "", -1)
	d.Duration, err = time.ParseDuration(duration)
	return
}

type _time struct {
	time.Time
}

func (t *_time) UnmarshalTOML(data []byte) (err error) {

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return
	}

	rawTime := strings.Replace(string(data), "\"", "", -1)

	layout := "Jan _2 15:04 2006"
	t.Time, err = time.ParseInLocation(layout, rawTime, loc)
	if err != nil {
		return
	}

	return
}
