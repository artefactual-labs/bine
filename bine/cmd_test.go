package bine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func fakeExecCommand(t *testing.T, testHelperName string) func(ctx context.Context, command string, args ...string) *exec.Cmd {
	return func(ctx context.Context, command string, args ...string) *exec.Cmd {
		cs := []string{fmt.Sprintf("-test.run=%s", testHelperName), "--", command}
		cs = append(cs, args...)
		t.Logf("Preparing *exec.Cmd using test binary %q with args: %v", os.Args[0], cs)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
}

func injectFakeExec(t *testing.T, testHelperName string) {
	t.Helper()
	execCommand = fakeExecCommand(t, testHelperName)
	t.Cleanup(func() { execCommand = exec.CommandContext })
}
