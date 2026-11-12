#!/usr/bin/env bash
set -euo pipefail

skill_name="tiki"
source_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ai/skills/${skill_name}"

tools=(claude-code codex gemini opencode)

say() {
  printf '%s\n' "$*"
}

usage() {
  cat <<'EOF'
Usage: install-skill.sh [--claude-code] [--codex] [--gemini] [--opencode] [--all] <project-dir>

Copy the tiki skill into a project, replacing any copy already there.

Without a tool flag, the skill is installed for every AI tool the project
already uses. With one or more flags, it is installed for exactly those tools,
which also works in a project that has no AI tool set up yet.

  Flag            Installs into           Detected by
  --claude-code   .claude/skills/tiki     .claude/ or CLAUDE.md
  --codex         .agents/skills/tiki     .codex/, .agents/ or AGENTS.md
  --gemini        .gemini/skills/tiki     .gemini/ or GEMINI.md
  --opencode      .opencode/skills/tiki   .opencode/, opencode.json or opencode.jsonc
  --all           all of the above

  -h, --help      Show this help
EOF
}

usage_error() {
  say "error: $1" >&2
  usage >&2
  exit 2
}

skill_dir() {
  case "$1" in
    claude-code) say ".claude/skills/${skill_name}" ;;
    codex) say ".agents/skills/${skill_name}" ;;
    gemini) say ".gemini/skills/${skill_name}" ;;
    opencode) say ".opencode/skills/${skill_name}" ;;
  esac
}

uses_tool() {
  local project="$1"
  case "$2" in
    claude-code) [ -d "${project}/.claude" ] || [ -f "${project}/CLAUDE.md" ] ;;
    codex) [ -d "${project}/.codex" ] || [ -d "${project}/.agents" ] || [ -f "${project}/AGENTS.md" ] ;;
    gemini) [ -d "${project}/.gemini" ] || [ -f "${project}/GEMINI.md" ] ;;
    opencode) [ -d "${project}/.opencode" ] || [ -f "${project}/opencode.json" ] || [ -f "${project}/opencode.jsonc" ] ;;
  esac
}

project=""
selected=()
for arg in "$@"; do
  case "${arg}" in
    -h|--help)
      usage
      exit 0
      ;;
    --all) selected=("${tools[@]}") ;;
    --claude-code|--codex|--gemini|--opencode) selected+=("${arg#--}") ;;
    -*) usage_error "unknown flag: ${arg}" ;;
    *)
      [ -z "${project}" ] || usage_error "more than one project directory: ${project} and ${arg}"
      project="${arg}"
      ;;
  esac
done

[ -n "${project}" ] || usage_error "missing project directory"
[ -d "${project}" ] || usage_error "not a directory: ${project}"
if [ ! -f "${source_dir}/SKILL.md" ]; then
  say "error: skill not found at ${source_dir}" >&2
  exit 1
fi
project="$(cd "${project}" && pwd)"

if [ "${#selected[@]}" -eq 0 ]; then
  for tool in "${tools[@]}"; do
    if uses_tool "${project}" "${tool}"; then
      selected+=("${tool}")
    fi
  done
fi
if [ "${#selected[@]}" -eq 0 ]; then
  say "error: no AI tool setup found in ${project}" >&2
  say "Name the tools to install for: --claude-code, --codex, --gemini, --opencode or --all." >&2
  exit 1
fi

# The copy replaces the whole directory, so files the skill no longer ships don't linger.
# That is only safe for a directory that already holds a copy of the skill,
# so check them all before changing anything.
for tool in "${selected[@]}"; do
  dest="${project}/$(skill_dir "${tool}")"
  if [ -e "${dest}" ] && [ ! -f "${dest}/SKILL.md" ] && [ -n "$(ls -A "${dest}" 2>/dev/null)" ]; then
    say "error: ${dest} exists and is not a copy of the skill; move it away first" >&2
    exit 1
  fi
done

installed=""
for tool in "${selected[@]}"; do
  case " ${installed} " in
    *" ${tool} "*) continue ;;
  esac
  installed="${installed} ${tool}"

  rel="$(skill_dir "${tool}")"
  dest="${project}/${rel}"
  verb="installed"
  if [ -e "${dest}" ]; then
    verb="updated"
    rm -rf "${dest}"
  fi
  mkdir -p "$(dirname "${dest}")"
  cp -R "${source_dir}" "${dest}"
  say "${verb} ${rel} (${tool})"
done

if ! command -v tiki >/dev/null 2>&1 || ! tiki workflow schema --help >/dev/null 2>&1; then
  say "note: the skill needs a tiki that has 'tiki workflow schema'; run 'make install' in the tiki repository" >&2
fi
