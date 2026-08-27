Could you write me a small pattern-matching tool? This directory is empty, so
set the Go project up yourself. Use `example.com/rx` as the module name, and
building it should produce an `rx` executable.

There are only three ways I'll call it:

```
rx <pattern> <file>      # print the lines that match
rx -o <pattern> <file>   # print only the matched text, one match per line
rx -c <pattern> <file>   # print just the number of matching lines
```

If no file is given, read standard input. That's the whole command surface.

The part I actually care about is the pattern syntax. It has to support all of
these, and they have to compose with each other:

- `.` any single character
- `*` `+` `?` repetition of whatever came before them
- `[abc]`, `[a-z]`, `[^abc]` character classes with ranges and negation
- `^` and `$` anchors
- `|` alternation
- `()` grouping, so `(ab)+` and `a(b|c)d` mean what you would expect
- `\` escapes, so `\.` matches a literal dot and `\\` a literal backslash

Matching is leftmost and greedy: for `a+` against `aaa`, one match of `aaa`,
not three of `a`. `-o` should print that whole match.

**Please implement the matching yourself — don't import `regexp`.** Writing a
one-line wrapper around the standard library defeats the point of asking. The
rest of the standard library is fine, and I'd rather not pull in outside
dependencies at all.

A few behaviours I care about:

- An invalid pattern — `a(b`, `[z-a]`, a trailing `\`, `*` with nothing before
  it — has to be reported on stderr with a non-zero exit, not treated as a
  literal.
- A file that doesn't exist is an error too, on stderr, non-zero exit.
- Finding no matches is *not* an error: print nothing (or `0` for `-c`) and
  exit 0.
- Exit code 0 when something matched, 1 when nothing did, and something else
  for actual errors. I want to use this in shell conditionals.

Please cover the syntax and the error cases with tests, and leave it with
`go test ./...` passing. Put usage examples in the README.

Work through this on your own without stopping to ask me. Don't stop at a plan
or at "here's how you'd approach it" — write the code and leave it in a state
where the tests pass. Keep going until you've checked for yourself that
everything above holds, then give me a short summary of what you did.
