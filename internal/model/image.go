package model

import "time"

type Image struct {
	ID         string
	Repository string
	Tag        string
	Size       int64
	Created    time.Time
	Containers int64
}

type ImageDetail struct {
	ID           string
	RepoTags     []string
	RepoDigests  []string
	Size         int64
	Created      time.Time
	Architecture string
	OS           string
	Author       string
	Layers       int
}

type PullProgress struct {
	Status string
	Done   bool
	Err    error
}
