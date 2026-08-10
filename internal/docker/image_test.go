package docker

import "testing"

func TestSplitImageTag(t *testing.T) {
	tests := map[string][2]string{"nginx:latest": {"nginx", "latest"}, "registry:5000/team/app:v1": {"registry:5000/team/app", "v1"}, "alpine": {"alpine", "latest"}}
	for input, want := range tests {
		repo, tag := splitImageTag(input)
		if repo != want[0] || tag != want[1] {
			t.Errorf("splitImageTag(%q) = %q, %q", input, repo, tag)
		}
	}
}
