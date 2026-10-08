package rootcmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/peterbourgon/ff/v4"
)

type interspersedFlags struct {
	*ff.FlagSet
	booleans []string
}

// Interspersed permits positional arguments between flags. Boolean flags must
// be listed because ff's public flag metadata does not expose their type.
// Global verbosity and help flags are included automatically.
func Interspersed(flags *ff.FlagSet, booleans ...string) ff.Flags {
	return &interspersedFlags{flags, append(booleans, "v", "verbose", "h", "help")}
}

func (f *interspersedFlags) Parse(args []string) error {
	var flags, positional []string
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if arg == "--" || arg == "-" {
			positional = append(positional, args...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if hasValue {
			continue
		}
		if slices.Contains(f.booleans, strings.ToLower(name)) {
			// ff permits a separate true/false value for long boolean flags.
			if strings.HasPrefix(arg, "--") && len(args) > 0 {
				if _, err := strconv.ParseBool(args[0]); err == nil {
					flags = append(flags, args[0])
					args = args[1:]
				}
			}
			continue
		}
		if _, known := f.GetFlag(name); known {
			if len(args) == 0 {
				return fmt.Errorf("missing value for %s", arg)
			}
			flags = append(flags, args[0])
			args = args[1:]
		}
	}
	return f.FlagSet.Parse(append(append(flags, "--"), positional...))
}
