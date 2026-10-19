// Lipgloss styles for the chat screen. Keep new colors here.
package cli

import "charm.land/lipgloss/v2"

// Palette. Styles below only pick from these colors.
const (
	cCyan  = "#00ADD8"
	cCoral = "#F09082"
	cGold  = "#F1BE58"
	cGreen = "#5FBF6A"
	cProse = "#E8D9A8"
	cDim   = "8"
	cHint  = "240"
	cKey   = "252"
	cSep   = "238"
	cQuote = "2"
	cDel   = "1"
	cBar   = "236"
	cPlace = "243"
	cPath  = "245"
)

var (
	cyanStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(cCyan))
	coralStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(cCoral))
	goldStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(cGold))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(cDim))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(cCoral))

	statusReadyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(cGreen))
	statusWorkStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(cGold))
	statusBadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(cCoral))
	hintKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(cKey))
	hintLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(cHint))
	sepStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color(cSep))

	// Assistant prose is a soft pale yellow. Codex leaves primary text as the
	// terminal foreground; #E8D9A8 is the warm tone nearest that look on a
	// dark background. Cyan, dim, green, and red accents stay separate.
	proseStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(cProse))
	headingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(cCyan)).Bold(true)
	strongStyle   = proseStyle.Bold(true)
	emStyle       = proseStyle.Italic(true)
	linkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(cCyan)).Underline(true)
	quoteStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(cQuote))
	listMarkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(cCyan))
	codeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(cCyan))
	diffAddStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(cQuote))
	diffDelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(cDel))
	toolHeadStyle = lipgloss.NewStyle().Bold(true)
	bulletStyle   = dimStyle

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(cHint))
	barStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(cBar)).
			Foreground(lipgloss.Color(cKey))
	placeholderStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(cBar)).
				Foreground(lipgloss.Color(cPlace))
	suggestStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(cPath))
	suggestOnStyle = lipgloss.NewStyle().Background(lipgloss.Color(cBar)).Foreground(lipgloss.Color(cCyan))
)
