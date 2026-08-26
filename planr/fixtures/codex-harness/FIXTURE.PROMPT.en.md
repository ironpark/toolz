Could you add a JSON output mode to the greeting CLI?

Right now `go run . --name Ada` just prints `Hello, Ada!`, and when I want to
consume that from another script I have to slice the string apart, which is
awkward. I'd like `--format json` to print one line of JSON instead. A single
`message` field with the greeting in it is enough.

A few things I care about:

- Without `--format` it has to print exactly what it prints today. It would be
  a problem if the scripts I already use broke.
- If someone passes an unsupported value like `--format xml`, don't just let it
  slide — say what went wrong and exit with a non-zero code.
- I'd like tests for both text and json, and for the bad-format case too.
- Please put usage examples for both modes in the README.

I'd rather not pull in outside libraries. The standard library should be plenty.

Please work through this on your own without stopping to ask me. Don't stop at
a plan or at "here's how you'd do it" — actually change the code and leave it in
a state where the tests pass. Keep going until you've confirmed for yourself
that everything above is handled, and when you're done give me a short summary
of what you did.
