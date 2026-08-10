package model

type ComposeProject struct {
	Name        string
	WorkingDir  string
	ConfigFiles []string
	Containers  []ComposeContainer
	Registered  bool
}

type ComposeContainer struct {
	ID      string
	Name    string
	Service string
	Image   string
	State   string
	Status  string
}

func (p ComposeProject) RunningCount() int {
	count := 0
	for _, container := range p.Containers {
		if container.State == "running" {
			count++
		}
	}
	return count
}
