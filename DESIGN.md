# tmux-treefort

A fast popup tree widget for navigating and managing tmux sessions and windows,
built with Bubble Tea and Lipgloss.

## Motivation

The Python/Textual version works well but starts slowly enough to be annoying
when used as a tmux popup. This rewrite prioritizes near-instant startup. Pane
support has been dropped — sessions and windows are enough.

## Features

- Tree view of all tmux sessions and their windows
- Sessions prefixed with `__` are always hidden
- Live switching: navigating the tree switches tmux focus in real-time
- Fuzzy search with match highlighting
- Number-jump mode: press `n` to label items, then type a number to jump
- Single-session focus: collapse the view to just the current session
- Add, rename, and delete sessions and windows
- Escape restores the original session/window

## Key Bindings

| Key        | Action                          |
|------------|---------------------------------|
| `j` / `k`  | Move cursor down / up           |
| `Enter`    | Select and exit                 |
| `Escape`   | Cancel and restore original     |
| `/`        | Toggle search                   |
| `n`        | Toggle number-jump labels       |
| `g`        | Toggle tree guides              |
| `s`        | Toggle single-session focus     |
| `a`        | Add window                      |
| `r`        | Rename session or window        |
| `d`        | Delete session or window        |

In number-jump mode, type one or more digits within 500ms to jump to that item.

In search mode, `/session/window` narrows by both levels. `Escape` clears the
search, `Enter` confirms and exits.

## Architecture

```
main.go   - entry point, flag parsing, initial session/window capture
model.go  - Bubble Tea model: state, Update(), View()
tmux.go   - data types (Session, Window) and tmux CLI queries
modal.go  - confirmation modal
```

### Model

The top-level model holds:

```go
type Model struct {
    sessions      []Session
    items         []Item        // flat list of visible tree rows, rebuilt on each refresh
    cursor        int
    searchTerm    string
    searchMode    bool
    inputMode     bool          // true when rename/add prompt is active
    inputPrompt   string        // label shown before the input cursor
    inputValue    []rune
    showNumbers   bool
    showGuides    bool
    focusSession  string        // session ID, or "" for all
    modal         tea.Model     // nil when no modal is active (only confirm modal)
    modalAction   ModalAction
    initialSessID string
    initialWinID  string
    jumpBuffer    string        // digits typed in number-jump mode
    jumpUpdated   time.Time
    commandFile   string        // path to write deferred commands (for add-window)
    returnCommand string
    history       []HistoryEntry
    width, height int
}
```

`items` is a flat slice rebuilt on every refresh. Each `Item` carries pointers
to its underlying `Session` and `Window`, a plain text `Name`, and a
pre-styled `Label` (with ANSI fuzzy markup and number prefix). The cursor index
into this slice drives all navigation and selection.

Cursor rows are rendered from `Name` (plain text) rather than `Label`, so that
the full-width background highlight isn't broken by ANSI reset codes embedded
in the styled label.

### Input

Rename and add-window use an inline bottom-bar input, the same way search does.
When `inputMode` is true, keypresses feed into `inputValue`; `Enter` submits and
`Escape` cancels. The prompt renders as a single line at the bottom of the view,
keeping the view height constant.

The delete confirmation still uses a centered modal (`ConfirmModal`) since it
needs to be more prominent.

### Fuzzy matching

- Characters in the search term must appear in order within the item name
  (case-insensitive, spaces ignored).
- A `/` in the search term splits into a session filter and a window filter.
- Matched characters are rendered bold+underline; non-matching items are dimmed.

### Live switching

On every cursor move, `Update()` issues a tmux command as a Cmd (off the main
goroutine):

- Cursor on a session → `tmux switch-client -t <session-id>`
- Cursor on a window → `tmux switch-client -t <session-id>:<window-id>`

### Add window

Adding a window writes a shell command to a file (specified via `--command-file`)
rather than running it directly. This is necessary because the tool runs inside
a tmux popup; the command must execute in the calling shell after the popup
closes. An optional `--return-command` is appended after it.

If no `--command-file` is given, `tmux new-window` is run directly.

### Delete with smart fallback

After deleting a session or window, the cursor moves to the most recently
visited item that is not the deleted one (from a history list). If history
offers nothing, it falls back to the first available session or window.

## CLI Flags

```
--command-file <path>     Write add-window command here instead of running it
--return-command <cmd>    Append this command to the command file
--search-mode             Start with the search input focused
```

## Dependencies

- `github.com/charmbracelet/bubbletea` — application framework
- `github.com/charmbracelet/lipgloss` — styling and layout
