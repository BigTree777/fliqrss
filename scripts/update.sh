#!/usr/bin/env bash

set -Eeuo pipefail

readonly FRONTEND_SERVICE="${FLIQRSS_FRONTEND_SERVICE:-fliqrss-frontend}"
readonly BACKEND_HEALTH_ATTEMPTS="${FLIQRSS_BACKEND_HEALTH_ATTEMPTS:-30}"
readonly BACKEND_HEALTH_INTERVAL="${FLIQRSS_BACKEND_HEALTH_INTERVAL:-2}"

log() {
  printf '[fliqrss-update] %s\n' "$*"
}

fail() {
  printf '[fliqrss-update] ERROR: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "必要なコマンドが見つかりません: $1"
}

backend_is_healthy() {
  docker compose exec -T backend \
    wget -qO- http://localhost:8080/api/v1/health >/dev/null 2>&1
}

wait_for_backend() {
  local attempt
  log 'バックエンドのヘルスチェックを待っています'
  for ((attempt = 1; attempt <= BACKEND_HEALTH_ATTEMPTS; attempt++)); do
    if backend_is_healthy; then
      log 'バックエンドのヘルスチェックに成功しました'
      return 0
    fi
    sleep "$BACKEND_HEALTH_INTERVAL"
  done

  docker compose logs --tail=50 backend >&2 || true
  fail 'バックエンドのヘルスチェックに失敗しました'
}

restart_frontend() {
  local -a systemctl_command=(systemctl)
  if ((EUID != 0)); then
    require_command sudo
    systemctl_command=(sudo systemctl)
  fi

  "${systemctl_command[@]}" restart "$FRONTEND_SERVICE"
  "${systemctl_command[@]}" is-active --quiet "$FRONTEND_SERVICE" \
    || fail "フロントエンドサービスが起動していません: $FRONTEND_SERVICE"
  log 'フロントエンドを再起動しました'
}

require_command git

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPOSITORY_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$REPOSITORY_DIR"

[[ -z "$(git status --porcelain --untracked-files=normal)" ]] \
  || fail '未コミットまたは未追跡のファイルがあります。内容を確認してから再実行してください'

readonly GIT_DIR="$(git rev-parse --absolute-git-dir)"
readonly DEPLOYED_REVISION_FILE="$GIT_DIR/fliqrss-deployed-revision"

deployed_revision=''
if [[ -f "$DEPLOYED_REVISION_FILE" ]]; then
  read -r deployed_revision < "$DEPLOYED_REVISION_FILE" || true
fi

log 'リモートの更新を取得します'
git pull --ff-only
readonly current_revision="$(git rev-parse HEAD)"

backend_changed=false
compose_changed=false
frontend_changed=false
frontend_dependencies_changed=false
full_update=false
declare -a changed_files=()

if [[ -z "$deployed_revision" ]] || ! git cat-file -e "${deployed_revision}^{commit}" 2>/dev/null; then
  full_update=true
  backend_changed=true
  compose_changed=true
  frontend_changed=true
  frontend_dependencies_changed=true
  log '初回実行のため、バックエンドとフロントエンドを更新します'
elif [[ "$deployed_revision" == "$current_revision" ]]; then
  log '適用が必要な更新はありません'
  exit 0
else
  mapfile -t changed_files < <(git diff --name-only "$deployed_revision" "$current_revision")
  for changed_file in "${changed_files[@]}"; do
    case "$changed_file" in
      backend/*)
        backend_changed=true
        ;;
      compose.yaml)
        backend_changed=true
        compose_changed=true
        ;;
      frontend/*)
        frontend_changed=true
        case "$changed_file" in
          frontend/package.json|frontend/package-lock.json)
            frontend_dependencies_changed=true
            ;;
        esac
        ;;
    esac
  done

  log "未適用の変更を検出しました: ${deployed_revision:0:7}..${current_revision:0:7}"
  printf '  %s\n' "${changed_files[@]}"
fi

if [[ "$frontend_changed" == true ]]; then
  require_command npm
  if [[ "$frontend_dependencies_changed" == true || ! -d frontend/node_modules ]]; then
    log 'フロントエンドの依存関係をインストールします'
    npm --prefix frontend ci
  fi
  log 'フロントエンドの型検査を実行します'
  npm --prefix frontend run typecheck
fi

if [[ "$backend_changed" == true ]]; then
  require_command docker
  docker compose version >/dev/null
  docker compose config --quiet
  if [[ "$compose_changed" == true ]]; then
    log 'Composeサービスを再ビルドして更新します'
    docker compose up --build -d
  else
    log 'バックエンドを再ビルドして更新します'
    docker compose up --build -d backend
  fi
  wait_for_backend
fi

if [[ "$frontend_changed" == true ]]; then
  require_command systemctl
  restart_frontend
fi

if [[ "$full_update" == false && "$backend_changed" == false && "$frontend_changed" == false ]]; then
  log '実行中サービスに影響する変更はありませんでした'
fi

printf '%s\n' "$current_revision" > "$DEPLOYED_REVISION_FILE"
log "更新が完了しました: ${current_revision:0:7}"
