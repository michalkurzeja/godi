package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

// launch opens a path or a URL with whatever the operating system uses for it.
//
// It returns once the command has started, not when the user closes what it
// opened. Some desktop openers do not exit until the browser does.
func launch(target string) error {
	name, args := opener(target)
	if name == "" {
		return fmt.Errorf("no way to open a file on %s", runtime.GOOS)
	}

	bin, err := exec.LookPath(name)
	if err != nil {
		return errorsx.Wrapf(err, "looking for %s", name)
	}

	//nolint:gosec // G204: the binary is resolved from PATH and the path is ours.
	cmd := exec.Command(bin, args...)
	err = cmd.Start()
	if err != nil {
		return errorsx.Wrapf(err, "running %s", name)
	}

	go func() { _ = cmd.Wait() }() // Reap it; nobody is waiting on the result.
	return nil
}

func opener(target string) (name string, args []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{target}
	case "windows":
		// The empty string is start's title argument, which it insists on.
		return "cmd", []string{"/c", "start", "", target}
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly", "solaris", "illumos":
		return "xdg-open", []string{target}
	default:
		return "", nil
	}
}
