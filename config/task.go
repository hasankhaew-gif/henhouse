package config

import (
	"encoding/xml"
)

type Task struct {
	Name          string
	Description   string
	NameEn        string
	DescriptionEn string
	Category      string
	Tags          string
	Level         int
	ForceClosed   bool
	Flag          string
	Author        string
}

func ParseXMLTask(rawXML []byte) (task Task, err error) {
	err = xml.Unmarshal(rawXML, &task)
	return
}
