# bash integration for eda, emitted by `eda init - bash`.
#
# Add to .bashrc:
#
#   eval "$(eda init - bash)"
#
# A CLI process cannot change the parent shell's directory, so this thin
# wrapper cds into the path that `eda switch` / `eda root` print on stdout.
# Every other subcommand passes through untouched.

eda() {
  case "$1" in
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
  local cur=${COMP_WORDS[COMP_CWORD]}
  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=($(compgen -W "switch path list remove root status init" -- "$cur"))
    return
  fi
  case "${COMP_WORDS[1]}" in
    switch|path|remove)
      local branches
      branches=$({
        git for-each-ref --format='%(refname:short)' refs/heads 2>/dev/null
        git for-each-ref --format='%(refname:short)' refs/remotes/origin 2>/dev/null | sed 's|^origin/||'
      } | grep -vx HEAD | sort -u)
      COMPREPLY=($(compgen -W "$branches" -- "$cur"))
      ;;
  esac
}

complete -F _eda eda
