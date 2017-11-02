package hcsshim

import (
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

var lstat = os.Lstat // for testing, allow override of lstat

// walk recursively descends path, calling w.
func walk(path string, info os.FileInfo, walkFn filepath.WalkFunc) error {
	// Go1.9 changed the way Directories and Reprase Points are exposed
	// in the mode bits. We read directly from Win32FileAttributeData
	// so that this function is consistent across versions of Go.
	// This function should NOT rely on the results of the mode bits
	// or use info.IsDir(), as the behaviour depends on Go version.
	isReparsePoint := info.Sys().(*syscall.Win32FileAttributeData).FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
	isDir := info.Sys().(*syscall.Win32FileAttributeData).FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0

	err := walkFn(path, info, nil)
	if err != nil {
		if (isDir && !isReparsePoint) && err == filepath.SkipDir {
			return nil
		}
		return err
	}

	// If this is a file, or a Directory Reparse Point, skip it
	if !isDir || isReparsePoint {
		return nil
	}

	names, err := readDirNames(path)
	if err != nil {
		return walkFn(path, info, err)
	}

	for _, name := range names {
		filename := filepath.Join(path, name)
		fileInfo, err := lstat(filename)
		if err != nil {
			if err := walkFn(filename, fileInfo, err); err != nil && err != filepath.SkipDir {
				return err
			}
		} else {
			err = walk(filename, fileInfo, walkFn)
			if err != nil {
				isReparsePoint := fileInfo.Sys().(*syscall.Win32FileAttributeData).FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
				isDir := fileInfo.Sys().(*syscall.Win32FileAttributeData).FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0
				if !isDir || isReparsePoint || err != filepath.SkipDir {
					return err
				}
			}
		}
	}

	return nil
}

// fastWalk is a fork of filepath.Walk which does NOT guarantee an in-order
// walk, it also fixes a bug which allowed traversal of Directory Reparse
// Points in Go1.8 in order to maintain behaviour between Go1.8 and Go1.9
//
// It walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn.
// fastWalk does not follow symbolic links.
func fastWalk(root string, walkFn filepath.WalkFunc) error {
	info, err := os.Lstat(root)
	if err != nil {
		err = walkFn(root, nil, err)
	} else {
		err = walk(root, info, walkFn)
	}
	if err == filepath.SkipDir {
		return nil
	}
	return err
}

// readDirNames reads the directory named by dirname
func readDirNames(dirname string) ([]string, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}
