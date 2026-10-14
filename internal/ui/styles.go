package ui

import "charm.land/lipgloss/v2"

type styles struct {
	title, subtle, selected, size, bar, barEmpty, context, err, marker lipgloss.Style
}

func newStyles(isDark bool) styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#D7005F"), lipgloss.Color("#FF5FAF"))
	subtle := ld(lipgloss.Color("#6C6C6C"), lipgloss.Color("#8A8A8A"))
	return styles{
		title:    lipgloss.NewStyle().Bold(true).Foreground(accent),
		subtle:   lipgloss.NewStyle().Foreground(subtle),
		selected: lipgloss.NewStyle().Reverse(true),
		size:     lipgloss.NewStyle().Bold(true),
		bar:      lipgloss.NewStyle().Foreground(accent),
		barEmpty: lipgloss.NewStyle().Foreground(subtle),
		context:  lipgloss.NewStyle().Foreground(subtle),
		err:      lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#D70000"), lipgloss.Color("#FF5F5F"))),
		marker:   lipgloss.NewStyle().Foreground(subtle),
	}
}
