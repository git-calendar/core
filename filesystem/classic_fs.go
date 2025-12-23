//go:build !js

package filesystem

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
)

const RepoDirName string = ".git-calendar-data"

// easy as that you bozo
func GetRepoFS() (billy.Filesystem, string, error) {
	// get user home dir
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}

	repoPath := filepath.Join(home, RepoDirName)

	err = os.MkdirAll(repoPath, 0o755)
	if err != nil {
		return nil, "", err
	}

	return osfs.New(repoPath), ".", nil
}
