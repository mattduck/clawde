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

### Additional key bindings

- `C-g` will send `ESC`.
- `C-p` and `C-n` map to up/down.

## Configuration

The following environment variables can be used to configure clawde's behavior:

- `CLAWDE_BETTER_DEFAULTS`: Sets some UX enhancements for the wrapped program, including `CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION=false` per https://github.com/anthropics/claude-code/issues/13878#issuecomment-3651710357 (default: true)
- `CLAWDE_FORCE_ANSI`: The builtin "ansi" theme actually uses true colour, which looks bad in my terminal. This forces ANSI color support by setting COLORTERM=ansi and TERM=xterm for the wrapped program (default: false)
- `CLAWDE_OUTPUT_THROTTLING`: Experiment to reduce terminal flicker, not sure it works. This just limits screen redraws to happen at a lower frame rate (default: true)
- `CLAWDE_INPUT_THROTTLING`: A separate, faster rate for when you're typing. (default: true)
- `CLAWDE_HELD_ENTER_DETECTION`: Feature I tried but didn't like: hold enter key to actually submit (default: false)
- `CLAWDE_LOG_FILE`: Specifies a file path for logging output (default: disabled)
- `CLAWDE_LOG_LEVEL`: Sets the logging level (info, debug, error, etc.) (default: info)

All boolean values accept "true", "1", "yes", or "on" (case-insensitive) as true.

## Notes

- Obviously wrapping the shell is brittle and the features would be better built
  in direct. This was mostly an experiment in using claude to generate a tool
  from scratch.

- Only tested on macOS using iterm2, YMMV on other platforms.

- Features subject to change to whatever I find useful.
