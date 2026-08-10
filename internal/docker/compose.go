package docker

import (
	"context"
	"sort"
	"strings"

	"github.com/moby/moby/client"

	"dockman/internal/model"
)

const (
	composeProjectLabel     = "com.docker.compose.project"
	composeServiceLabel     = "com.docker.compose.service"
	composeWorkingDirLabel  = "com.docker.compose.project.working_dir"
	composeConfigFilesLabel = "com.docker.compose.project.config_files"
)

func (m *Manager) ListComposeProjects(ctx context.Context) ([]model.ComposeProject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return nil, ErrNotConnected
	}
	result, err := m.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	projects := map[string]*model.ComposeProject{}
	for _, item := range result.Items {
		projectName := item.Labels[composeProjectLabel]
		if projectName == "" {
			continue
		}
		project := projects[projectName]
		if project == nil {
			project = &model.ComposeProject{Name: projectName, WorkingDir: item.Labels[composeWorkingDirLabel], ConfigFiles: splitComposeConfigFiles(item.Labels[composeConfigFilesLabel])}
			projects[projectName] = project
		}
		name := item.ID[:min(12, len(item.ID))]
		if len(item.Names) > 0 {
			name = strings.TrimPrefix(item.Names[0], "/")
		}
		project.Containers = append(project.Containers, model.ComposeContainer{ID: item.ID, Name: name, Service: item.Labels[composeServiceLabel], Image: item.Image, State: string(item.State), Status: item.Status})
	}
	list := make([]model.ComposeProject, 0, len(projects))
	for _, project := range projects {
		sort.Slice(project.Containers, func(i, j int) bool { return project.Containers[i].Name < project.Containers[j].Name })
		list = append(list, *project)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func splitComposeConfigFiles(value string) []string {
	var files []string
	for _, file := range strings.Split(value, ",") {
		if file = strings.TrimSpace(file); file != "" {
			files = append(files, file)
		}
	}
	return files
}
