package ui

type Screen int

const (
	ScreenConnecting Screen = iota
	ScreenConnectionError
	ScreenMainMenu
	ScreenContainers
	ScreenContainerDetail
	ScreenContainerLogs
	ScreenContainerDelete
	ScreenContainerCreate
	ScreenContainerImagePicker
	ScreenContainerCreatePreview
	ScreenContainerTerminal
	ScreenContainerStats
	ScreenImages
	ScreenImageDetail
	ScreenImagePull
	ScreenImageDelete
	ScreenVolumes
	ScreenVolumeDetail
	ScreenVolumeCreate
	ScreenVolumeDelete
	ScreenNetworks
	ScreenNetworkDetail
	ScreenNetworkCreate
	ScreenNetworkDelete
	ScreenNetworkConnect
	ScreenCompose
	ScreenComposeRegister
	ScreenComposeDetail
	ScreenComposeLogs
	ScreenComposeContainers
	ScreenSystem
	ScreenCleanup
	ScreenCleanupConfirm
	ScreenContexts
)
