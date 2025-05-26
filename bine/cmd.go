package bine

import "os/exec"

// execCommand can be replaced in tests to inject a fake command function.
var execCommand = exec.CommandContext
