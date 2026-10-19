// Lipgloss styles for the chat screen. Keep new colors here.
package cli

import "charm.land/lipgloss/v2"

var (
	cyanStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	coralStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))
	goldStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1BE58"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))

	statusReadyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5FBF6A"))
	statusWorkStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1BE58"))
	statusBadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))
	hintKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sepStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	// Assistant prose is a soft pale yellow. Codex leaves primary text as the
	// terminal foreground; #E8D9A8 is the warm tone nearest that look on a
	// dark background. Cyan, dim, green, and red accents stay separate.
	proseStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#E8D9A8"))
	headingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8")).Bold(true)
	strongStyle   = proseStyle.Bold(true)
	emStyle       = proseStyle.Italic(true)
	linkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8")).Underline(true)
	quoteStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	listMarkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	codeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	diffAddStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	diffDelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	toolHeadStyle = lipgloss.NewStyle().Bold(true)
	bulletStyle   = dimStyle

	cardBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	barStyle        = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))
	placeholderStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("243"))
	suggestStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	suggestOnStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("#00ADD8"))
)
