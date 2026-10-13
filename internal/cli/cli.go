// Package cli is the terminal session for the coding agent.
package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"golang.org/x/term"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/coding"
)

// Run parses args and starts a session. It reads .env from the current directory.
func Run(args []string) error {
	return run(args, os.Stdin, os.Stdout)
}

func run(args []string, in io.Reader, out io.Writer) error {
	if err := loadDotEnv(".env"); err != nil {
		return err
	}
	opts, err := parseArgs(args, out)
	if errors.Is(err, errHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	ep, err := endpointFromEnv(opts.baseURL, opts.model)
	if err != nil {
		return err
	}
	session, err := coding.New(coding.Config{
		APIKey:    ep.apiKey,
		BaseURL:   ep.baseURL,
		Model:     ep.model,
		Workspace: firstNonEmpty(opts.workspace, "."),
	})
	if err != nil {
		return err
	}
	app := &app{session: session, in: in, out: out}
	if opts.prompt != "" {
		session.Subscribe(func(ev agent.Event) { render(out, ev) })
		stop := app.watchSignals()
		defer stop()
		return app.prompt(opts.prompt)
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

type options struct {
	prompt    string
	workspace string
	model     string
	baseURL   string
}

func parseArgs(args []string, out io.Writer) (options, error) {
	fs := flag.NewFlagSet("pigo", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts options
	fs.StringVar(&opts.prompt, "p", "", "run one prompt and exit")
	fs.StringVar(&opts.workspace, "w", "", "workspace directory (default: current directory)")
	fs.StringVar(&opts.model, "model", "", "model name (default: OPENAI_MODEL or gpt-4o-mini)")
	fs.StringVar(&opts.baseURL, "base-url", "", "API root (default: OPENAI_BASE_URL or https://api.openai.com/v1)")
	fs.Usage = func() { writeUsage(out, fs) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, errHelp
		}
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	opts.prompt = strings.TrimSpace(opts.prompt)
	return opts, nil
}

// errHelp tells Run to exit successfully after printing usage.
var errHelp = errors.New("help")

func writeUsage(out io.Writer, fs *flag.FlagSet) {
	fmt.Fprintf(out, `pigo is a small coding agent.

Usage:
  pigo              chat in the current directory
  pigo -p "task"    run one task and exit
  pigo -w DIR       use DIR as the workspace

Environment:
  PIGO_PROVIDER     chat backend (default openai)
  OPENAI_API_KEY    API key for PIGO_PROVIDER=openai (required)
  OPENAI_BASE_URL   OpenAI-compatible API root (default https://api.openai.com/v1)
  OPENAI_MODEL      model name that endpoint accepts (default gpt-4o-mini)

`)
	fs.SetOutput(out)
	fs.PrintDefaults()
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
	fmt.Fprintln(a.out, "Enter a task. /exit quits. /reset clears the transcript.")
	sc := bufio.NewScanner(a.in)
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
		if err := a.prompt(line); err != nil {
			if errors.Is(err, agent.ErrInterrupted) {
				fmt.Fprintln(a.out, "interrupted")
				continue
			}
			fmt.Fprintf(a.out, "error: %v\n", err)
		}
	}
}

func (a *app) prompt(text string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return a.session.Prompt(ctx, text)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
