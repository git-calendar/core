//go:build !js

package filesystem

import (
	"os"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
)

const DirName string = ".git-calendar-data"

// easy as that you bozo
//
// Returns a FS starting from users home directory
func GetFS() (billy.Filesystem, error) {
	// get user home dir
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return osfs.New(home), nil
}
