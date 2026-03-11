# Fish shell example

This example shows a reusable Fish helper for enabling a project's `bine`
environment.

Create `~/.config/fish/functions/bine-env.fish` with:

```fish
function bine-env
    go tool bine env --shell=fish | source

    if not functions -q original_fish_prompt
        functions -c fish_prompt original_fish_prompt
    end

    function fish_prompt
        original_fish_prompt
        set PROJECT_NAME (go tool bine config get project)
        echo -n "→ [$PROJECT_NAME] "
    end
end
```

Then run `bine-env` from a project root to:

- add the current project's `bine` bin directory to `PATH`
- show the current `bine` project name in the prompt

If `bine` is installed as a standalone binary rather than a Go tool, replace:

```fish
go tool bine
```

with:

```fish
bine
```
