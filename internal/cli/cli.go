package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/coding"
	"github.com/zyy17/pigo/internal/config"
)

// A pasted task can be longer than the scanner's default 64KiB token.
const (
	replLineStart = 64 * 1024
	replLineMax   = 1024 * 1024
)

const version = "0.1.0"

// /skill:name takes an argument, so it is a prefix rather than a row in builtins.
const (
	skillName = "/skill:name"
	skillDesc = "load that skill, then an optional request"
)

// command is what a typed line asks the session to do.
type command int

const (
	cmdPrompt command = iota
	cmdEmpty
	cmdQuit
	cmdReset
)

// slashCommand is one row in the "/" menu and one exact session command.
type slashCommand struct {
	Name string
	Desc string
	Do   command
}

// builtins is the only list of exact session commands. The menu, help,
// the REPL welcome, and the opening hint all read it.
var builtins = []slashCommand{
	{Name: "/reset", Desc: "clear the transcript", Do: cmdReset},
	{Name: "/exit", Desc: "quit", Do: cmdQuit},
}

// commandAlias covers the quit words that are not themselves menu rows.
var commandAlias = map[string]command{
	"exit":  cmdQuit,
	"quit":  cmdQuit,
	"/quit": cmdQuit,
}

func sessionHelp() string {
	var b strings.Builder
	b.WriteString("Inside the session:\n")
	fmt.Fprintf(&b, "  %-16s %s\n", skillName, skillDesc)
	for _, cmd := range builtins {
		fmt.Fprintf(&b, "  %-16s %s\n", cmd.Name, cmd.Desc)
	}
	return strings.TrimRight(b.String(), "\n")
}

func hintText() string {
	parts := make([]string, 0, 1+len(builtins))
	parts = append(parts, skillName+" "+skillDesc)
	for _, cmd := range builtins {
		parts = append(parts, cmd.Name+" "+cmd.Desc)
	}
	return strings.Join(parts, ". ") + "."
}

func usageText() string {
	return fmt.Sprintf(`pigo is a small coding agent.

%s

Environment:
  PIGO_PROVIDER     chat backend (default openai)
  OPENAI_API_KEY    API key for PIGO_PROVIDER=openai (required)
  OPENAI_BASE_URL   OpenAI-compatible API root (default %s)
  OPENAI_MODEL      model name that endpoint accepts (default %s)
  PIGO_YOLO         set to 1 or true to skip file-change review and the dangerous-command check
  PIGO_SESSION_DIR  directory for session files (default ~/.pigo/sessions)

Sessions are saved as JSONL unless --no-session is set.
--continue resumes the newest session for this workspace.
--session resumes a file path, an id, or a unique id prefix.

Config files are optional: ~/.pigo/config.yaml, then ./pigo.yaml.
A yolo: true key in those files skips the same checks.
Precedence is flags, then the environment, then .env, then those files, then defaults.
`, sessionHelp(), config.DefaultBaseURL, config.DefaultModel)
}

// Run parses args and starts a session.
func Run(args []string) error {
	return run(args, os.Stdin, os.Stdout)
}

func run(args []string, in io.Reader, out io.Writer) error {
	cmd := newCommand(in, out)
	cmd.SetArgs(args)
	cmd.SetOut(out)
	cmd.SetErr(out)
	return cmd.Execute()
}

func newCommand(in io.Reader, out io.Writer) *cobra.Command {
	var flags config.Config
	cmd := &cobra.Command{
		Use:           "pigo",
		Short:         "a small coding agent",
		Long:          usageText(),
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return start(flags, in, out)
		},
	}
	cmd.Flags().StringVarP(&flags.Prompt, "print", "p", "", "run one prompt and exit")
	cmd.Flags().StringVarP(&flags.Workspace, "workspace", "w", "", "workspace directory (default: current directory)")
	cmd.Flags().StringVar(&flags.Model, "model", "", "model name (default: OPENAI_MODEL or "+config.DefaultModel+")")
	cmd.Flags().StringVar(&flags.BaseURL, "base-url", "", "API root (default: OPENAI_BASE_URL or "+config.DefaultBaseURL+")")
	cmd.Flags().BoolVar(&flags.Yolo, "yolo", false, "apply file changes and dangerous shell commands without asking")
	cmd.Flags().BoolVarP(&flags.Continue, "continue", "c", false, "resume the newest session for this workspace")
	cmd.Flags().StringVar(&flags.Session, "session", "", "resume a session file, id, or unique id prefix")
	cmd.Flags().BoolVar(&flags.NoSession, "no-session", false, "do not read or write a session file")
	cmd.Flags().StringVar(&flags.SessionDir, "session-dir", "", "sessions directory (default: ~/.pigo/sessions or PIGO_SESSION_DIR)")
	cmd.MarkFlagsMutuallyExclusive("continue", "session", "no-session")
	return cmd
}

func start(flags config.Config, in io.Reader, out io.Writer) error {
	cfg, err := config.Load(flags)
	if err != nil {
		return err
	}
	if err := checkProvider(cfg); err != nil {
		return err
	}
	dir, resume, err := sessionChoice(cfg)
	if err != nil {
		return err
	}
	session, err := coding.New(coding.Config{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		Workspace:  cfg.Workspace,
		Yolo:       cfg.Yolo,
		SessionDir: dir,
		Resume:     resume,
	})
	if err != nil {
		return err
	}
	app := &app{session: session, in: in, out: out}
	if cfg.Prompt == "" && interactiveTerminal(in, out) {
		return runTUI(session)
	}
	if n := len(session.Messages()); n > 0 {
		fmt.Fprintf(out, "resumed %s (%d messages)\n", session.SessionID(), n)
	}
	session.Subscribe(func(ev agent.Event) { render(out, ev) })
	if cfg.Prompt != "" {
		return app.runLine(cfg.Prompt)
	}
	return app.repl()
}

func sessionChoice(cfg config.Config) (string, string, error) {
	if cfg.NoSession {
		return "", "", nil
	}
	dir := cfg.SessionDir
	if dir == "" {
		var err error
		dir, err = coding.DefaultSessionRoot()
		if err != nil {
			return "", "", err
		}
	}
	if cfg.Continue {
		return dir, coding.ResumeLatest, nil
	}
	return dir, cfg.Session, nil
}

func interactiveTerminal(in io.Reader, out io.Writer) bool {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	if !inOK || !outOK {
		return false
	}
	return term.IsTerminal(int(inFile.Fd())) && term.IsTerminal(int(outFile.Fd()))
}

type app struct {
	session *coding.Session
	in      io.Reader
	out     io.Writer
}

func (a *app) repl() error {
	fmt.Fprintf(a.out, "pigo  workspace %s  model %s\n", a.session.Root(), a.session.Model())
	fmt.Fprintln(a.out, "Enter a task. "+hintText())
	sc := bufio.NewScanner(a.in)
	sc.Buffer(make([]byte, 0, replLineStart), replLineMax)
	for {
		fmt.Fprint(a.out, "you> ")
		if !sc.Scan() {
			fmt.Fprintln(a.out)
			return sc.Err()
		}
		line := strings.TrimSpace(sc.Text())
		switch sessionCommand(line) {
		case cmdEmpty:
			continue
		case cmdQuit:
			return nil
		case cmdReset:
			// The file is already on disk. Say which one, then start a new file.
			// Clearing the screen transcript is not the same as deleting that file.
			saved := ""
			if a.session.SessionFile() != "" {
				saved = a.session.SessionID()
			}
			if err := a.session.Reset(); err != nil {
				fmt.Fprintf(a.out, "error: %v\n", err)
				continue
			}
			fmt.Fprintln(a.out, "transcript cleared")
			if saved != "" {
				fmt.Fprintf(a.out, "previous session %s\n", saved)
			}
			continue
		}
		if err := a.runLine(line); err != nil {
			if errors.Is(err, agent.ErrInterrupted) {
				fmt.Fprintln(a.out, "interrupted")
				continue
			}
			fmt.Fprintf(a.out, "error: %v\n", err)
		}
	}
}

func (a *app) runLine(line string) error {
	text, err := commandText(a.session, line)
	if err != nil {
		return err
	}
	return a.prompt(text)
}

func (a *app) prompt(text string) error {
	// Ctrl+C cancels this turn in -p and in the line REPL. The chat screen
	// handles that key itself and does not call prompt.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return a.session.Prompt(ctx, text)
}

// sessionCommand is the slash table shared by the chat screen and the REPL.
func sessionCommand(line string) command {
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return cmdEmpty
	}
	if cmd, ok := commandAlias[line]; ok {
		return cmd
	}
	for _, cmd := range builtins {
		if strings.EqualFold(cmd.Name, line) {
			return cmd.Do
		}
	}
	return cmdPrompt
}
