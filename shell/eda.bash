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
  local cur=${COMP_WORDS[COMP_CWORD]}
  COMPREPLY=()
  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=($(compgen -W "switch list tree remove root status init" -- "$cur"))
    return
  fi
  case "${COMP_WORDS[1]}" in
    switch|remove)
      # Never feed ref names to compgen -W: it expands its word list, so a
      # hostile ref name (fetched from a remote) could execute code. Append
      # literally filtered candidates instead.
      local ref
      while IFS= read -r ref; do
        [[ $ref == HEAD ]] && continue
        [[ $ref == "$cur"* ]] && COMPREPLY+=("$ref")
      done < <(
        {
          git for-each-ref --format='%(refname:short)' refs/heads 2>/dev/null
          git for-each-ref --format='%(refname:short)' refs/remotes/origin 2>/dev/null | sed 's|^origin/||'
        } | sort -u
      )
      ;;
  esac
}

complete -F _eda eda
