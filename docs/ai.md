# AI collaboration

## AI skills

`tiki` ships optional [agent skills](https://agentskills.io/home) that you copy into your AI tool
manually — there is no in-app installer. Once installed you can:

- work with [Claude Code](https://code.claude.com),
  [Gemini CLI](https://github.com/google-gemini/gemini-cli), [Codex](https://openai.com/codex), and
  [Opencode](https://opencode.ai) by mentioning documents or ids in your prompts
- create, find, modify, and delete documents using AI
- create documents directly from Markdown files
- reference documents by id when implementing with AI-assisted development — `implement ABC123`
- watch the agent's progress on the board: it publishes a [live status](live-status.md) on the
  tiki it is working on
- keep a history of prompts/plans by saving them as documents alongside your repo

### Installing a skill

The skill ships in the tiki repository as the directory `ai/skills/tiki/`:

- `SKILL.md` — the core, loaded whenever the skill is used: querying, creating, updating and deleting
  documents, implementing one, and live status
- `references/*.md` — modules the agent reads only when their case comes up: points estimation,
  dependencies, long and quoted text, more query patterns, file layout, and provenance

From a checkout of the tiki repository, `install-skill.sh` copies it into a project:

```bash
./install-skill.sh /path/to/project                # every AI tool the project already uses
./install-skill.sh --claude-code /path/to/project  # exactly the named tools
./install-skill.sh --all /path/to/project
```

| Flag | Installs into | Detected by |
|---|---|---|
| `--claude-code` | `.claude/skills/tiki/` | `.claude/` or `CLAUDE.md` |
| `--codex` | `.agents/skills/tiki/` | `.codex/`, `.agents/` or `AGENTS.md` |
| `--gemini` | `.gemini/skills/tiki/` | `.gemini/` or `GEMINI.md` |
| `--opencode` | `.opencode/skills/tiki/` | `.opencode/`, `opencode.json` or `opencode.jsonc` |

- Without a flag, the script installs for each tool whose files it finds in the project, and stops
  with an error when it finds none. With flags it installs for exactly those tools, which is how you
  set up a project that has no AI tool files yet.
- Running it again updates the copy. The skill's directory is replaced as a whole, so don't keep your
  own files in it. A directory at that path that isn't a copy of the skill is left alone, with an
  error.
- Gemini CLI and Opencode also read `.agents/skills/`, and Opencode `.claude/skills/`, so a project
  that already has the skill there doesn't need a second copy for them.
- The script needs `bash`. On Windows, or for a user-wide install such as `~/.claude/skills/tiki/`,
  copy the whole `ai/skills/tiki/` directory by hand, not just `SKILL.md`.

The skill's content is tool-agnostic.

The same copy works in every project. The skill doesn't describe any workflow's fields: the agent runs
[`tiki workflow schema`](command-line.md#workflow-schema) once per session to learn the fields, values and
triggers of the project it is in, so it stays correct when a `workflow.yaml` changes. This needs a `tiki`
that has the command; with an older one the skill falls back to reading `workflow.yaml`.

When you update tiki, run the script again to pick up changes to the skill.

## Chat with AI

If you [configured](config.md) an AI agent these features are enabled:

- open the current document in an AI agent such as [Claude Code](https://code.claude.com) and have it read the
  file. You can then chat or edit the document without needing to find it first.

## Configuration

In your `config.yaml`:

```yaml
# AI agent integration
ai:
  agent: claude              # AI tool for chat: "claude", "gemini", "codex", "opencode"
                             # Enables AI collaboration features
                             # Omit or leave empty to disable
```
