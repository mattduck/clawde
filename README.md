# clawde

_CLaude code Wrapper (Duck Extensions)_

This program transparently wraps the claude code terminal interface, but
modifies stdin to give me some UX improvements.

## Install and getting started

- install: `go install github.com/mattduck/clawde@latest`

- run: `clawde`. Arguments are directly passed through to `claude`, which must
  be on your $PATH. Usage is the same as `claude`, but with the features mentioned
  below.

## Features

### Adjusted enter key behaviour for better multi-line support

When the `-- INSERT --` mode indicator is detected, enter will automatically be
translated to backslash + enter. You can press `Ctrl-J` to actually send enter,
which will submit the prompt. This swaps the claude code prompt submit
behaviour, where the only way to insert a newline is to type backslash. For me
this makes it more intuitive to manage multi-line text without accidentally
submitting it.

### Aider-style AI keywords in code comments

Aider has an optional feature that will watch files for changes, and, if it
detects a code comment ending in `AI?` or `AI!`, will automatically submit that
comment to the chat in "ask" mode or "edit" mode as appropriate. This is a quick
IDE-agnostic way to message the LLM without having to switch to the terminal and
describe where the code is.

`clawde` does something similar: when a file is saved, it looks for AI-marker
comments and submits a prompt to claude code via stdin, with a snippet of the
comment and instructions on where claude code should find the whole thing.

There are three styles of comments:

- `AI?` and `AI!` comments are picked up by the file-watcher on save, and
  immediately submit prompts to either answer a question, or make a code
  edit. These can appear at the beginning or end of a code comment.

- `AI:` comments are contextual: if a `?` or `!` comment is submitted, then any
  `AI:` comments in the repo will be included as additional context. The `AI:`
  marker must appear at the start of the comment.

Examples:

``` python
  def parse_config(file_path):
      # Refactor to use context manager and handle missing files AI!
      f = open(file_path)
      data = json.load(f)
      f.close()
      return data
```

``` javascript
  function fetchData(url) {
      // Add proper error handling and retry logic AI!
      return fetch(url).then(r => r.json());
  }
```

``` javascript
  function processUser(user) {
      // How can I make this async without breaking the API AI?
      const result = database.query(`SELECT * FROM users WHERE id = ${user.id}`);
      return result;
  }
```

Additionally, you can use `Ctrl+/` to manually find all `AI:` comments and copy
them into the prompt, but NOT submit them. This allows you to quickly reference
locations without having to tell claude where to find each item.

If you're in a git repo, gitignore rules are followed -- ignored files won't be
processed for comments.

## Notes

- Obviously wrapping the shell is brittle and the features would be better built
  in direct. This was mostly an experiment in using claude to generate a tool
  from scratch.

- Features subject to change to whatever I find useful.
