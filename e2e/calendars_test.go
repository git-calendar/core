package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/git-calendar/core/pkg/core"
	"github.com/git-calendar/core/pkg/filesystem"
)

func TestCreateCalendar(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	dirs, err := os.ReadDir(filepath.Join(home, filesystem.DirName))
	if err != nil {
		t.Errorf("failed to read event json file: %v", err)
	}

	var found bool
	for _, d := range dirs {
		if d.Name() == TestCalendarName {
			found = true
			break
		}
	}
	if !found {
		t.Error("directory not found")
	}
}
