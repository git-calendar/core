package e2e

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "git-calendar-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temporary e2e home: %v\n", err)
		os.Exit(1)
	}

	for _, name := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(name, home); err != nil {
			fmt.Fprintf(os.Stderr, "set %s for e2e tests: %v\n", name, err)
			_ = os.RemoveAll(home)
			os.Exit(1)
		}
	}

	code := m.Run()
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintf(os.Stderr, "remove temporary e2e home: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
