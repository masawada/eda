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
      # hostile ref name (fetched from a remote) could execute code. Filter
      # candidates literally instead, and shell-quote them on the way out:
      # bash inserts them on the command line verbatim, where an unquoted
      # $(...) would run on Enter.
      local ref quoted
      while IFS= read -r ref; do
        [[ $ref == HEAD ]] && continue
        [[ $ref == "$cur"* ]] || continue
        printf -v quoted '%q' "$ref"
        COMPREPLY+=("$quoted")
      done < <(
        # strip= rather than :short so origin/HEAD comes out as HEAD (its
        # short form is a bare "origin") and gets dropped above.
        {
          git for-each-ref --format='%(refname:strip=2)' refs/heads 2>/dev/null
          git for-each-ref --format='%(refname:strip=3)' refs/remotes/origin 2>/dev/null
        } | sort -u
      )
      ;;
  esac
}

complete -F _eda eda
