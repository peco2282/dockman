package ui

import (
	"testing"
	"time"

	"dockman/internal/model"
)

func TestContainerSearchFilterAndSort(t *testing.T) {
	s := containerScreen{items: []model.Container{
		{Name: "web", Image: "nginx:latest", State: "running", CPUPercent: 2, ComposeProject: "site", Created: time.Unix(1, 0)},
		{Name: "db", Image: "postgres:18", State: "exited", CPUPercent: 0, ComposeProject: "site", Created: time.Unix(2, 0)},
		{Name: "worker", Image: "busybox", State: "running", CPUPercent: 8, ComposeProject: "jobs", Created: time.Unix(3, 0)},
	}}
	s.query = "nginx"
	if got := s.visibleItems(); len(got) != 1 || got[0].Name != "web" {
		t.Fatalf("search result = %#v", got)
	}
	s.query, s.filter = "", 1
	if got := s.visibleItems(); len(got) != 2 {
		t.Fatalf("running count = %d", len(got))
	}
	s.filter = 3 // alphabetically first Compose project: jobs
	if got := s.visibleItems(); len(got) != 1 || got[0].ComposeProject != "jobs" {
		t.Fatalf("compose result = %#v", got)
	}
	s.filter, s.sort = 0, 4
	if got := s.visibleItems(); got[0].Name != "worker" {
		t.Fatalf("CPU sort first = %q", got[0].Name)
	}
}
