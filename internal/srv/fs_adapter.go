package srv

import (
	"fmt"
	"os"

	"github.com/pkg/sftp"
)

type fsAdapter struct {
	impl fsImpl
}

func osOpenFlags(pflags sftp.FileOpenFlags) int {
	var osFlags int
	if pflags.Append {
		osFlags = osFlags | os.O_APPEND
	}
	if pflags.Creat {
		osFlags = osFlags | os.O_CREATE
	}
	if pflags.Excl {
		osFlags = osFlags | os.O_EXCL
	}
	if pflags.Read && pflags.Write {
		osFlags = osFlags | os.O_RDWR
	} else if pflags.Read {
		osFlags = osFlags | os.O_RDONLY
	} else if pflags.Write {
		osFlags = osFlags | os.O_WRONLY
	}
	if pflags.Trunc {
		osFlags = osFlags | os.O_TRUNC
	}
	return osFlags
}

func (fs *fsAdapter) OpenFile(pathname string, flags sftp.FileOpenFlags, perm os.FileMode) (File, error) {
	return fs.impl.OpenFile(pathname, osOpenFlags(flags), perm)
}

func (fs *fsAdapter) Exists(path string) (bool, error) {
	_, err := fs.impl.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat call failed: %w", err)
	}
	return true, nil
}

func (fs *fsAdapter) Rename(from, to string) error {
	// FIXME: account for symlinks

	// IEEE 1003.1: if oldpath and newpath are the same directory entry,
	// then return no error, and perform no further action.

	return fs.impl.Rename(from, to)
}

// Rmdir removes empty directory
func (fs *fsAdapter) Rmdir(dirPath string) error {
	stat, err := fs.impl.Stat(dirPath)
	if os.IsNotExist(err) {
		return nil // doesnt exist, pretend we successfully removed it
	}
	if err != nil {
		return fmt.Errorf("stat call failed: %w", err)
	}
	if !stat.IsDir() {
		return os.ErrInvalid
	}
	// FIXME:
	// IEEE 1003.1: If pathname is a symlink, then rmdir should fail with ENOTDIR.

	// IEEE 1003.1: if oldpath is a directory, and newpath exists,
	// then newpath must be a directory, and empty.
	// It is to be removed prior to rename.

	// IEEE 1003.1: if oldpath is not a directory, and newpath exists,
	// then newpath may not be a directory.

	return fs.impl.Remove(dirPath)
}

func (fs *fsAdapter) Unlink(path string) error {
	// FIXME:
	// IEEE 1003.1: implementations may opt out of allowing the unlinking of directories.
	// SFTP-v2: SSH_FXP_REMOVE may not remove directories.
	return fs.impl.Remove(path)
}

func (fs *fsAdapter) Mkdir(path string, perm os.FileMode) error {
	return fs.impl.Mkdir(path, perm)
}

func (fs *fsAdapter) Stat(file string) (os.FileInfo, error) {
	return fs.impl.Stat(file)
}

func (fs *fsAdapter) Link(file, target string) error {
	return fs.impl.Link(file, target)
}

func (fs *fsAdapter) Symlink(target, file string) error {
	return fs.impl.Symlink(file, target)
}

func (fs *fsAdapter) Readdir(dirPath string) ([]os.FileInfo, error) {
	file, err := fs.impl.OpenFile(dirPath, os.O_RDONLY, 0400)
	if err != nil {
		return nil, err
	}
	return file.Readdir(0)
}

func (fs *fsAdapter) Readlink(path string) (string, error) {
	return fs.impl.Readlink(path)
}
