//go:build !js

package filesystem

import (
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
)

func GetRepoFS(dirName string) (billy.Filesystem, error) {
	// easy as that you bozo
	return osfs.New(dirName), nil
}
