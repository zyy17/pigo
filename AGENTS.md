# AGENTS.md

Rules for humans and coding agents working on pigo. Prefer this file over history when they disagree. Project design and usage live in [README.md](README.md), [docs/design.md](docs/design.md), and [docs/tui.md](docs/tui.md); do not paste chat logs into the docs.

## What this project is

pigo is a **teaching-oriented**, minimal Go clone of [Pi](https://github.com/earendil-works/pi). The goal is to make Pi's core ideas readable in a small codebase: OpenAI-compatible transport, agent loop, coding tools, skills, and a calm terminal chat.

It is not a full Pi port. Keep the tree small and aligned with Pi's layers. Prefer deleting or refusing scope over growing toward a marketplace, multi-vendor SDKs, session store, sub-agents, or telemetry.

## Layering (non-negotiable)

| Package | May know | Must not know |
| --- | --- | --- |
| `internal/ai` | Messages, tool schemas, stream events, HTTP | Tool execution, workspace, skills, TUI |
| `internal/agent` | Transcript, ask/act loop, `Tool.Execute` | HTTP wire details, workspace paths, skills, CLI |
| `internal/coding` | Workspace, file/shell tools, system prompt | SSE parsing, Cobra, Bubble Tea |
| `internal/skills` | Finding `SKILL.md`, short prompt list | Agent loop, HTTP, TUI |
| `internal/config` | Flat settings load (koanf) | Agent, AI, and coding packages, and the screen |
| `internal/cli` | Cobra, TUI, `/skill:name`, wiring | Path confinement and wire encoding |

`BeforeToolCall` is the only core extension on a tool call. Bash denylist rules live in `internal/coding`. The TUI owns the confirm prompt. The agent core has no safety policy.

Skills are not a new agent type. Discovery feeds the system prompt (name, description, path). `/skill:name` becomes an ordinary user message before `Prompt`. The loop still only sees messages and tools.

Reuse Pi coding-agent system prompt sections where we already do. Do not invent a parallel prompt architecture.

## Code style

- Idiomatic Go: community naming, short functions, flat structs. Avoid deep nesting.
- Teachability first: a reader should follow one turn without jumping through many indirections.
- Comments only on key paths and critical sections (workspace confinement, request encoding, tool-result pairing, skill discovery safety). No narrating comments.
- Prefer the smallest clear code. Keep functions short and the tree easy to read at a glance. Do not pad or invent scope. Line count is a soft preference for simplicity, not a hard cap.
- Pass `go test ./...` and `golangci-lint run ./...` (see `.golangci.yml` / `make lint`) before you claim done. `make test` includes hermetic end-to-end tests. `make e2e` is the live model suite (`//go:build e2e`) and skips without `OPENAI_API_KEY` or `OPENAI_BASE_URL`.
- Prefer existing patterns in the tree over new abstractions.

## CLI, config, TUI

- CLI: **Cobra**. Interactive chat is the default; `-p` / `--print` stays non-interactive.
- Config: **koanf**, one flat struct (`provider`, `api_key`, `base_url`, `model`, `workspace`, `yolo`, `session_dir`). Precedence: flags > environment > `.env` > `./pigo.yaml` > `~/.pigo/config.yaml` > defaults. The built-in model is `gpt-5.6-luna`. Keep `PIGO_PROVIDER`, `OPENAI_*`, `PIGO_YOLO`, and `PIGO_SESSION_DIR` working. No API-key flag (avoid shell history). A false `--yolo` does not clear `yolo: true` from a file. Chat requests omit `max_tokens`, `temperature`, and `top_p`.
- Sessions: append-only JSONL under `~/.pigo/sessions/--<cwd>--/` (Pi's path encoding, pigo's directory). `--continue` and `--session` resume the leaf into the transcript. `/reset` starts a new file and leaves the old one. The system prompt and token totals are not stored.
- Only `internal/cli` and `internal/config` may import Cobra or koanf.
- TUI: Bubble Tea v2 + lipgloss. The hardware cursor is a steady bar on the composer cell, so an IME candidate window follows typing. Codex-like calm chat on the alternate screen (banner, `>` prompt, gray input bar, assistant bullet, `• Ran` tool lines, a working row with one blank line above and below, spinner, elapsed time, and `esc to interrupt`). The composer and footer stay fixed at the bottom. The transcript above them scrolls in the app: `scroll` is the first visible line, and a negative value follows the tail. PgUp/PgDn move a page with a one-line overlap, the mouse wheel moves one line, and Esc returns to the latest line when the turn is idle. New output sticks to the bottom until the user scrolls up. Agent events reach the program with `program.Send`. Ctrl-U clears the whole composer. Enter sends and clears the composer. While a turn is running the composer stays editable, and Enter queues one follow-up (`↳ queued:`); a second Enter appends a newline. A finished turn sends it and leaves a draft the user is still typing. An interrupt or error puts it back in the composer. `/exit`, `exit`, and `quit` abort and quit; `/reset` is refused until the turn ends. When the slash menu is open, Tab and Enter both complete the selected item; Tab never sends, and Enter sends an exact name. Esc interrupts a running turn even if that menu is open. Shift+Enter inserts a newline where the terminal reports it (Bubble Tea asks for keyboard disambiguation); Ctrl+J, Alt+Enter, and a trailing `\` do so everywhere. Wrap to the terminal width (CJK-aware); a resize rewraps the transcript from its source. Startup leaves one blank line, then the pigo ASCII wordmark (logo colors: Go cyan `#00ADD8`, coral `#F09082`, gold `#F1BE58`). Quitting leaves the alternate screen, which restores the shell buffer from before pigo. Do not enter the alternate screen when stdout is not a terminal. Study charmbracelet/crush for patterns; do not vendor crush.
- `/skill:name`, `/reset`, `/exit`, and Ctrl-C abort-turn behavior stay.

## Skills

- Scan `~/.pi/agent/skills` first, then `<workspace>/.pi/skills`. First name wins (user overrides project).
- A directory with `SKILL.md` is one skill; do not recurse into it. Skip unsafe symlink walks and `SKILL.md` targets outside the skill directory.
- Prompt lists name, description, path only. Load body via `read_file` or `/skill:name`.
- `read_file` may open a discovered skill directory (including outside the workspace). `write_file` / `edit_file` stay inside the workspace.
- Out of scope: ignore files, `.agents/skills`, package skills, marketplace, `/reload`.

## Docs

- Design and usage only (EN + ZH). No conversation transcripts.
- All diagrams: Mermaid.
- Keep README and `docs/design*.md` honest when behavior changes.
- Logo: `docs/images/logo.svg` (Pi-style blocks + Go gopher cues).

## Git and shipping

- Atomic Conventional Commits (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`), clean messages, no emoji.
- Self-test before push: `go test ./...`, `golangci-lint run ./...`, and a quick sanity check of the surface you touched.
- Do not commit secrets, `.env`, or API keys.

## Out of scope (unless the user explicitly expands)

Full Pi TUI feature set, native per-vendor APIs, skills marketplace, session branching, the session picker, compaction, sub-agents, plan mode, telemetry, permission sandbox, ignore-file skill discovery.

## When unsure

Answer the question first. Prefer the smallest change that keeps Pi alignment and the code easy to read. If a request fights teaching readability or the layer table above, say so and propose a smaller cut.
