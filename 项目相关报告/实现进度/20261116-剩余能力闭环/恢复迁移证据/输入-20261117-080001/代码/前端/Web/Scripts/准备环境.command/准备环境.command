#!/bin/zsh
# shell: zsh
# 依赖只写用户级缓存，不修改系统Node/npm或其它项目。
init_paths() {
  setopt NO_NOMATCH
  SCRIPT_DIR="${0:A:h}"
  APP_DIR="${SCRIPT_DIR:h:h}"
  CACHE_DIR="${JOBSIM_WEB_CACHE:-${HOME}/.cache/JobsIM/Web}"
  NPM_CACHE="${HOME}/.cache/JobsIM/Npm"
}
check_tools() {
  command -v node >/dev/null && command -v npm >/dev/null || { print -u2 '需要Node.js 22.12以上与npm。'; return 1; }
  node -e 'const v=process.versions.node.split(".").map(Number);process.exit(v[0]>22||(v[0]===22&&v[1]>=12)?0:1)' || return 1
}
prepare() {
  mkdir -p "$CACHE_DIR" "$NPM_CACHE" || return 1
  cp "$APP_DIR/package.json" "$CACHE_DIR/package.json" || return 1
  cp "$APP_DIR/package-lock.json" "$CACHE_DIR/package-lock.json" || return 1
  npm ci --prefix "$CACHE_DIR" --cache "$NPM_CACHE" --no-audit --no-fund || return 1
  if [[ -e "$APP_DIR/node_modules" && ! -L "$APP_DIR/node_modules" ]]; then
    print -u2 '工程已有实体node_modules，保留现场，请先确认归属。'
    return 1
  fi
  if [[ -L "$APP_DIR/node_modules" ]]; then
    [[ "$(readlink "$APP_DIR/node_modules")" == "$CACHE_DIR/node_modules" ]] || { print -u2 '已有不同依赖链接，保留并停止。'; return 1; }
  else
    ln -s "$CACHE_DIR/node_modules" "$APP_DIR/node_modules" || return 1
  fi
}
main() {
  init_paths
  print 'JobsIM Web环境准备：依赖写入用户缓存，工程只保留链接，不升级全局工具。'
  [[ "${1:-}" == '--check' ]] && { check_tools; return $?; }
  if [[ "${1:-}" != '--yes' ]]; then
    print '按回车继续；输入空格后回车取消。'
    local answer
    IFS= read -r answer
    [[ -z "$answer" ]] || return 130
  fi
  check_tools && prepare
}
main "$@"
