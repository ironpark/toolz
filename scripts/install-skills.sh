#!/usr/bin/env bash

set -euo pipefail

repository_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  echo "Usage: $0 [all|codex|claude]" >&2
}

install_platform() {
  local platform="$1"
  local source_directory="$2"
  local installation_directory="$3"
  local skill_directory skill_name link_path current_target

  mkdir -p "$installation_directory"

  for skill_directory in "$source_directory"/*; do
    [ -d "$skill_directory" ] || continue

    skill_name="${skill_directory##*/}"
    link_path="$installation_directory/$skill_name"

    if [ -e "$link_path" ] && [ ! -L "$link_path" ]; then
      echo "Refusing to replace non-symlink path: $link_path" >&2
      return 1
    fi

    if [ -L "$link_path" ]; then
      current_target="$(readlink "$link_path")"
      if [ "$current_target" = "$skill_directory" ]; then
        echo "Already installed $platform/$skill_name"
        continue
      fi
      rm "$link_path"
    fi

    ln -s "$skill_directory" "$link_path"
    echo "Installed $platform/$skill_name"
  done
}

if [ "$#" -gt 1 ]; then
  usage
  exit 2
fi

platform="${1:-all}"
case "$platform" in
  all)
    install_platform codex "$repository_root/skillz/codex" "${CODEX_HOME:-"$HOME/.codex"}/skills"
    install_platform claude "$repository_root/skillz/claude" "${CLAUDE_HOME:-"$HOME/.claude"}/skills"
    ;;
  codex)
    install_platform codex "$repository_root/skillz/codex" "${CODEX_HOME:-"$HOME/.codex"}/skills"
    ;;
  claude)
    install_platform claude "$repository_root/skillz/claude" "${CLAUDE_HOME:-"$HOME/.claude"}/skills"
    ;;
  *)
    usage
    exit 2
    ;;
esac
