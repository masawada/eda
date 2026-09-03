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
      # candidates literally instead, and shell-quote the ones that carry
      # shell metacharacters: bash inserts completions on the command line
      # verbatim, where an unquoted $(...) would run on Enter. Names without
      # such characters stay as they are, since %q mangles non-ASCII names
      # into $'...' on bash 3.2 and under LC_ALL=C.
      #
      # The word being completed may carry escapes from an earlier round:
      # bash inserts the common prefix of the quoted candidates, so after
      # `x-\$foo` and `x-\;bar` it reads `x-\`. Drop the backslashes before
      # matching; ref names cannot contain one, so nothing else is lost.
      local ref quoted plain=${cur//\\/}
      while IFS= read -r ref; do
        [[ $ref == HEAD ]] && continue
        [[ $ref == "$plain"* ]] || continue
        case $ref in
          *[\$\`\'\"\;\&\|\<\>\(\)\{\}\!\#]*) printf -v quoted '%q' "$ref" ;;
          *) quoted=$ref ;;
        esac
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
