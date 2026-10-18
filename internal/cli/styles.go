package cli

import "github.com/charmbracelet/lipgloss"

// Lipgloss styles for the chat screen. Keep new colors here.
var (
	cyanStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	coralStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))
	goldStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1BE58"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))

	statusReadyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5FBF6A"))
	statusWorkStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1BE58"))
	statusBadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))
	modelStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8")).Bold(true)
	pathStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	hintKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sepStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	// Assistant prose stays the default foreground. Accents match Codex:
	// cyan for code, headings, and links; green and red for diffs.
	headingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8")).Bold(true)
	strongStyle   = lipgloss.NewStyle().Bold(true)
	emStyle       = lipgloss.NewStyle().Italic(true)
	linkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8")).Underline(true)
	quoteStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	listMarkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	codeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	diffAddStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	diffDelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	toolHeadStyle = lipgloss.NewStyle().Bold(true)
	bulletStyle   = dimStyle

	barStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))
	placeholderStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("243"))
	suggestStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	suggestOnStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("#00ADD8"))
)
