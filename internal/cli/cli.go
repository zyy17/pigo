// Package cli is the terminal session for the coding agent.
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

const version = "0.1.0"

const longHelp = `pigo is a small coding agent.

Inside the session:
  /skill:name       load that skill, then an optional request
  /reset            clear the transcript
  /exit             quit

Environment:
  PIGO_PROVIDER     chat backend (default openai)
  OPENAI_API_KEY    API key for PIGO_PROVIDER=openai (required)
  OPENAI_BASE_URL   OpenAI-compatible API root (default https://api.openai.com/v1)
  OPENAI_MODEL      model name that endpoint accepts (default gpt-5.6-luna)
  PIGO_YOLO         set to 1 or true to skip the dangerous-command check

Config files are optional: ~/.pigo/config.yaml, then ./pigo.yaml.
A yolo: true key in those files skips the same check.
Precedence is flags, then the environment, then .env, then those files, then defaults.
`

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
		Long:          longHelp,
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
	cmd.Flags().StringVar(&flags.Model, "model", "", "model name (default: OPENAI_MODEL or gpt-5.6-luna)")
	cmd.Flags().StringVar(&flags.BaseURL, "base-url", "", "API root (default: OPENAI_BASE_URL or https://api.openai.com/v1)")
	cmd.Flags().BoolVar(&flags.Yolo, "yolo", false, "run dangerous shell commands without asking")
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
	session, err := coding.New(coding.Config{
		APIKey:    cfg.APIKey,
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
		Workspace: cfg.Workspace,
		Yolo:      cfg.Yolo,
	})
	if err != nil {
		return err
	}
	app := &app{session: session, in: in, out: out}
	if cfg.Prompt != "" {
		session.Subscribe(func(ev agent.Event) { render(out, ev) })
		stop := app.watchSignals()
		defer stop()
		return app.runLine(cfg.Prompt)
	}
	if interactiveTerminal(in, out) {
		return runTUI(session)
	}
	session.Subscribe(func(ev agent.Event) { render(out, ev) })
	return app.repl()
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

func (a *app) watchSignals() func() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-sig:
				a.session.Abort()
			}
		}
	}()
	return func() {
		signal.Stop(sig)
		close(done)
	}
}

func (a *app) repl() error {
	fmt.Fprintf(a.out, "pigo  workspace %s  model %s\n", a.session.Root(), a.session.Model())
	fmt.Fprintln(a.out, "Enter a task. /skill:name loads a skill. /exit quits. /reset clears the transcript.")
	sc := bufio.NewScanner(a.in)
	// The default token limit is 64KiB, which rejects a long pasted task.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Fprint(a.out, "you> ")
		if !sc.Scan() {
			fmt.Fprintln(a.out)
			return sc.Err()
		}
		line := strings.TrimSpace(sc.Text())
		switch line {
		case "":
			continue
		case "/exit", "exit", "quit", "/quit":
			return nil
		case "/reset":
			a.session.Reset()
			fmt.Fprintln(a.out, "transcript cleared")
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return a.session.Prompt(ctx, text)
}
