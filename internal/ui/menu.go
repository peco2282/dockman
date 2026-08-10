package ui

type menuItem struct {
	label  string
	screen Screen
	exit   bool
}

var mainMenuItems = []menuItem{
	{label: "Containers", screen: ScreenContainers},
	{label: "Images", screen: ScreenImages},
	{label: "Volumes", screen: ScreenVolumes},
	{label: "Networks", screen: ScreenNetworks},
	{label: "Compose", screen: ScreenCompose},
	{label: "System", screen: ScreenSystem},
	{label: "Exit", exit: true},
}
