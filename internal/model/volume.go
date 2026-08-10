package model

import "time"

type Volume struct {
	Name       string
	Driver     string
	Mountpoint string
	Scope      string
	Created    time.Time
	InUse      bool
	Containers []string
	Labels     map[string]string
	Options    map[string]string
}
