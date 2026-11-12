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

Copy the whole `ai/skills/tiki/` directory, not just `SKILL.md`, into your tool's skills location, for
example:

| Tool | Target location |
|---|---|
| Claude Code | `.claude/skills/tiki/SKILL.md` (project) or `~/.claude/skills/tiki/SKILL.md` (global) |
| Gemini CLI | the skills directory configured for `gemini` |
| Codex | the skills directory configured for `codex` |
| Opencode | the skills directory configured for `opencode` |

Consult each tool's own documentation for its exact skills path; the skill's content is tool-agnostic.

The same copy works in every project. The skill doesn't describe any workflow's fields: the agent runs
[`tiki workflow schema`](command-line.md#workflow-schema) once per session to learn the fields, values and
triggers of the project it is in, so it stays correct when a `workflow.yaml` changes. This needs a `tiki`
that has the command; with an older one the skill falls back to reading `workflow.yaml`.

When you update tiki, copy the directory again to pick up changes to the skill.

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
