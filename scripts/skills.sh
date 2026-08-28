#!/usr/bin/env bash

set -euo pipefail

repository_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'USAGE'
Usage: skills.sh <command> [target]

저장소의 스킬을 각 플랫폼 스킬 디렉터리에 심볼릭 링크로 연결하고 관리합니다.

Commands:
  install    스킬을 심볼릭 링크로 설치합니다.
  uninstall  이 저장소를 가리키는 스킬 심볼릭 링크를 제거합니다.
  status     각 스킬의 설치 상태(installed/missing/conflict)를 출력합니다.
  help       이 도움말을 출력합니다.

Targets:
  all        codex와 claude를 모두 처리합니다. (기본값)
  codex      Codex 스킬만 처리합니다.
  claude     Claude 스킬만 처리합니다.

Environment:
  CODEX_HOME    Codex 홈 디렉터리. 기본값 ~/.codex
  CLAUDE_HOME   Claude 홈 디렉터리. 기본값 ~/.claude

Examples:
  skills.sh install            # 모든 플랫폼에 설치
  skills.sh install codex      # Codex에만 설치
  skills.sh status             # 설치 상태 확인
  skills.sh uninstall claude   # Claude 스킬 링크 제거

기존의 일반 파일이나 디렉터리, 다른 대상을 가리키는 심볼릭 링크는 덮어쓰거나
제거하지 않습니다.
USAGE
}

usage_error() {
  usage >&2
  exit 2
}

installation_directory_for() {
  case "$1" in
    codex) echo "${CODEX_HOME:-"$HOME/.codex"}/skills" ;;
    claude) echo "${CLAUDE_HOME:-"$HOME/.claude"}/skills" ;;
  esac
}

install_platform() {
  local platform="$1"
  local source_directory="$repository_root/skillz/$platform"
  local installation_directory
  installation_directory="$(installation_directory_for "$platform")"
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

uninstall_platform() {
  local platform="$1"
  local source_directory="$repository_root/skillz/$platform"
  local installation_directory
  installation_directory="$(installation_directory_for "$platform")"
  local skill_directory skill_name link_path current_target

  [ -d "$installation_directory" ] || return 0

  for skill_directory in "$source_directory"/*; do
    [ -d "$skill_directory" ] || continue

    skill_name="${skill_directory##*/}"
    link_path="$installation_directory/$skill_name"

    if [ ! -L "$link_path" ]; then
      if [ -e "$link_path" ]; then
        echo "Skipping non-symlink path: $link_path" >&2
      fi
      continue
    fi

    current_target="$(readlink "$link_path")"
    if [ "$current_target" != "$skill_directory" ]; then
      echo "Skipping symlink to another target: $link_path -> $current_target" >&2
      continue
    fi

    rm "$link_path"
    echo "Removed $platform/$skill_name"
  done
}

status_platform() {
  local platform="$1"
  local source_directory="$repository_root/skillz/$platform"
  local installation_directory
  installation_directory="$(installation_directory_for "$platform")"
  local skill_directory skill_name link_path current_target

  for skill_directory in "$source_directory"/*; do
    [ -d "$skill_directory" ] || continue

    skill_name="${skill_directory##*/}"
    link_path="$installation_directory/$skill_name"

    if [ -L "$link_path" ]; then
      current_target="$(readlink "$link_path")"
      if [ "$current_target" = "$skill_directory" ]; then
        echo "installed  $platform/$skill_name"
      else
        echo "conflict   $platform/$skill_name -> $current_target"
      fi
    elif [ -e "$link_path" ]; then
      echo "conflict   $platform/$skill_name (심볼릭 링크가 아님)"
    else
      echo "missing    $platform/$skill_name"
    fi
  done
}

case "${1:-}" in
  help | -h | --help)
    usage
    exit 0
    ;;
esac

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  usage_error
fi

command="$1"
platform="${2:-all}"

case "$command" in
  install | uninstall | status) ;;
  *) usage_error ;;
esac

case "$platform" in
  all) platforms=(codex claude) ;;
  codex | claude) platforms=("$platform") ;;
  *) usage_error ;;
esac

for platform in "${platforms[@]}"; do
  "${command}_platform" "$platform"
done
