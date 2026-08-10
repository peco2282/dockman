package ui

import "charm.land/lipgloss/v2"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	errorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)
