#!/usr/bin/env bash

set -euo pipefail

repository_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
source_directory="$repository_root/skills"
installation_directory="${1:-"$HOME/.codex/skills"}"

mkdir -p "$installation_directory"

for skill_directory in "$source_directory"/*; do
  [ -d "$skill_directory" ] || continue

  skill_name="$(basename "$skill_directory")"
  link_path="$installation_directory/$skill_name"

  if [ -e "$link_path" ] && [ ! -L "$link_path" ]; then
    echo "Refusing to replace non-symlink path: $link_path" >&2
    exit 1
  fi

  if [ -L "$link_path" ]; then
    rm "$link_path"
  fi

  ln -s "$skill_directory" "$link_path"
  echo "Installed $skill_name"
done
