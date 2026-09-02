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

# Prints one branch candidate per line: local branches plus origin's.
# strip= rather than :short so origin/HEAD comes out as HEAD (its short
# form is a bare "origin") and gets dropped with the local HEAD.
_eda_branches() {
  local -aU branches
  branches=(${(f)"$(git for-each-ref --format='%(refname:strip=2)' refs/heads 2>/dev/null)"})
  branches+=(${(f)"$(git for-each-ref --format='%(refname:strip=3)' refs/remotes/origin 2>/dev/null)"})
  print -l -- ${branches:#HEAD}
}

_eda() {
  local -a cmds
  cmds=(
    'switch:resolve or create the worktree for a branch and move there'
    'list:list worktrees of the current repository'
    'tree:show where the worktrees diverged from each other'
    'remove:remove worktrees and their branches as pairs'
    'root:move to the primary checkout'
    'status:print current repository, worktree, and branch'
    'init:print the shell integration script (zsh, bash)'
  )
  if (( CURRENT == 2 )); then
    _describe 'eda command' cmds
    return
  fi
  case "$words[2]" in
    switch|remove)
      local -a branches
      branches=(${(f)"$(_eda_branches)"})
      _describe 'branch' branches
      ;;
  esac
}

if (( $+functions[compdef] )); then
  compdef _eda eda
fi
