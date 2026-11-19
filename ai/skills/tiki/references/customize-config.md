# Changing tiki's settings

First check that tiki is new enough: `tiki --help` must list `tiki config`. If it doesn't, never run
`tiki config` (an older tiki opens its viewer on the word "config"). There, a project's `./config.yaml`
replaces `~/.config/tiki/config.yaml` entirely, and nothing below about merging or `project:` applies.

## Which file

Run `tiki config`. It lists each folder, **global** (the user's config directory) and **local** (the
current directory), and marks the file used in each as `loaded`.

- The two files merge: local settings override global ones key by key. Write only the settings that
  should differ.
- A setting for this project goes in the local file marked `loaded`. If there is none, create
  `tiki-config.yaml` in the current directory. Never add a second file beside a loaded `config.yaml`.
- Personal settings (`identity`, `ai`, `logging`) go in the global file. A project file is usually
  committed, so they would apply to everyone who uses the project. If the user asks for one in the
  project, say so and ask first.
- A `config.yaml` shown as `skipped` isn't tiki's (or has a misspelled section, named in the reason).
  Don't edit it for tiki.
- tiki reads settings when it starts. You can't see its screen: say what you changed and ask the user
  to restart tiki and look.

## Project identity

```yaml
project:
  name: Acme
  color: "#ff8800"
  logo: |2
     ╔═╗┌─┐┌┬┐┌─┐
     ╠═╣│  │││├┤
     ╩ ╩└─┘┴ ┴└─┘
```

- `name`: the terminal title (`Acme • tiki`) and the header (`Acme • <view>`). Default: the folder name.
- `color`: `#rrggbb`, `#rgb` or a colour name. Colours the logo, the name in the header and the
  directory block in the bottom bar, and leaves the theme alone.
- `logo`: replaces the dot logo; at most 6 lines. `●` `▓` `▒` take the three logo colours, any other
  character the main one. Use `|2` when the first line starts with a space, and no `#` comments inside
  the block: they become part of the logo. `figlet <word>` prints big letters. The logo is hidden when
  the terminal is too narrow.

## Other settings

- `appearance.theme` — theme names: `auto`, `dark`, `light`, `dracula`, `tokyo-night`, `gruvbox-dark`, `catppuccin-mocha`, `solarized-dark`, `nord`, `monokai`, `one-dark`, `catppuccin-latte`, `solarized-light`, `gruvbox-light`, `github-light`.
  A theme recolours everything; `project.color` is the lighter way to tell projects apart.
- `appearance.gradientThreshold` — `16`, `256` or `16777216`: the fewest terminal colours for gradients.
- `header.visible` — `true` or `false`.
- `tiki.maxImageRows` — the most rows an inline image takes (default 40).
- `liveReload.enabled` — `true` refreshes views when files change outside tiki.
- `liveStatus.staleAfter` — a duration such as `15m`: when an agent's live status dims.
- `ai.agent` (personal) — `claude`, `gemini`, `codex` or `opencode`; empty turns AI chat off.
- `identity.name`, `identity.email` (personal) — who `user()` resolves to; default: the git user.
- `logging.level` (personal) — `debug`, `info`, `warn` or `error`.

For a one-off run, every setting also has an environment variable: `TIKI_` plus the key in upper case
with `.` as `_`, for example `TIKI_APPEARANCE_THEME=nord tiki`.

The full reference is at https://github.com/kion/tiki/blob/main/docs/config.md. Fetch it only when
this file doesn't cover the case.
