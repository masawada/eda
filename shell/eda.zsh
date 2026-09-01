# zsh integration for eda, emitted by `eda init - zsh`.
#
# Add to .zshrc:
#
#   eval "$(eda init - zsh)"
#
# A CLI process cannot change the parent shell's directory, so this thin
# wrapper cds into the path that `eda switch` / `eda root` print on stdout.
# Every other subcommand passes through untouched.

eda() {
  case "${1-}" in
    switch|root)
      local dir
      dir="$(command eda "$@")" || return $?
      [[ -n "$dir" ]] && cd "$dir"
      ;;
    *)
      command eda "$@"
      ;;
  esac
}

_eda() {
  local -a cmds
  cmds=(
    'switch:resolve or create the worktree for a branch and move there'
    'path:print the worktree path for a branch'
    'list:list worktrees of the current repository'
    'remove:remove worktrees and their branches as pairs'
    'root:move to the primary checkout'
    'status:print current repository, worktree, and branch'
  )
  if (( CURRENT == 2 )); then
    _describe 'eda command' cmds
    return
  fi
  case "$words[2]" in
    switch|path|remove)
      local -aU branches
      branches=(${(f)"$(git for-each-ref --format='%(refname:short)' refs/heads 2>/dev/null)"})
      branches+=(${${(f)"$(git for-each-ref --format='%(refname:short)' refs/remotes/origin 2>/dev/null)"}#origin/})
      branches=(${branches:#HEAD})
      _describe 'branch' branches
      ;;
  esac
}

if (( $+functions[compdef] )); then
  compdef _eda eda
fi
