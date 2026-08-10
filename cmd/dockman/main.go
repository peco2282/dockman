package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	dockerdocker "dockman/internal/docker"
	"dockman/internal/service"
	"dockman/internal/ui"
)

func main() {
	dockerManager := dockerdocker.NewManager()
	defer dockerManager.Close()

	program := tea.NewProgram(ui.NewApp(
		service.NewConnectionService(dockerManager),
		service.NewContainerService(dockerManager),
		service.NewImageService(dockerManager),
		service.NewVolumeService(dockerManager),
		service.NewNetworkService(dockerManager),
		service.NewComposeService(dockerManager, service.NewDockerComposeRunner(dockerManager)),
		service.NewSystemService(dockerManager),
	))
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "dockman could not start cleanly.")
		os.Exit(1)
	}
}
