package srv

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
)

type fsSession struct {
	fsAdapter
	canonicalize func(string) string
	session      map[string]struct{}
}

func (fs *fsSession) mayClaim(filepath string) bool {
	if fs.isOwned(filepath) {
		return true
	}
	// ensure file don't already exist
	exists, err := fs.fsAdapter.Exists(filepath)
	if err != nil {
		return false
	}
	// if file doesnt exist, we may claim this path
	return !exists
}

// tryClaim will fail if file does not exist (so that we cannot claim unless we also
// successfully created the file - and our state is in sync with disk state)
func (fs *fsSession) tryClaim(filepath string) error {
	if fs.isOwned(filepath) {
		return nil
	}
	// ensure file don't already exist
	exists, err := fs.fsAdapter.Exists(filepath)
	if err != nil {
		return err
	}
	if !exists {
		return os.ErrPermission
	}

	canon := fs.canonicalize(filepath)
	fs.session[canon] = struct{}{}
	return nil
}

func (fs *fsSession) isOwned(filepath string) bool {
	// FIXME: what if parent isnt owned?
	canon := fs.canonicalize(filepath)
	_, exists := fs.session[canon]
	return exists
}

func (fs *fsSession) OpenFile(path string, flags sftp.FileOpenFlags, perm os.FileMode) (File, error) {
	osFlags := osOpenFlags(flags)
	wantsCreate := osFlags&os.O_CREATE != 0

	if !fs.isOwned(path) && !(wantsCreate && fs.mayClaim(path)) {
		return nil, os.ErrPermission
	}

	handle, err := fs.fsAdapter.impl.OpenFile(path, osFlags, perm)
	if err != nil {
		return handle, err
	}
	if wantsCreate {
		err := fs.tryClaim(path)
		if err != nil {
			return nil, err
		}
	}
	return handle, err
}

func (fs *fsSession) Exists(path string) (bool, error) {
	if !fs.isOwned(path) {
		return false, os.ErrPermission
	}
	_, err := fs.impl.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat failed: %w", err)
	}
	return true, nil
}

func (fs *fsSession) Rename(from, to string) error {
	if !(fs.isOwned(from) && fs.mayClaim(to)) {
		return os.ErrPermission
	}
	return fs.fsAdapter.Rename(from, to)
}

func (fs *fsSession) Rmdir(dirPath string) error {
	if !fs.isOwned(dirPath) {
		return os.ErrPermission
	}
	return fs.fsAdapter.Rmdir(dirPath)
}

func (fs *fsSession) Unlink(path string) error {
	if !fs.isOwned(path) {
		return os.ErrPermission
	}
	// FIXME:
	// IEEE 1003.1: implementations may opt out of allowing the unlinking of directories.
	// SFTP-v2: SSH_FXP_REMOVE may not remove directories.
	return fs.impl.Remove(path)
}

func (fs *fsSession) Mkdir(path string, perm os.FileMode) error {
	// FIXME: we probably want to allow traversal of file paths where
	// client thinks it's created the dir even if it already existed.
	// I THINK?
	/*
		if !fs.mayClaim(path) {
			return os.ErrPermission
		}
	*/
	// is mkir idempotent? Probably not because of potential perm differences?
	return fs.impl.Mkdir(path, perm)
}

func (fs *fsSession) Stat(file string) (os.FileInfo, error) {
	if !fs.isOwned(file) {
		return nil, os.ErrPermission
	}
	return fs.impl.Stat(file)
}

func (fs *fsSession) Link(file, target string) error {
	if !(fs.isOwned(file) && fs.mayClaim(target)) {
		return os.ErrPermission
	}
	return fs.impl.Link(file, target)
}

func (fs *fsSession) Symlink(target, file string) error {
	if !(fs.isOwned(file) && fs.mayClaim(target)) {
		return os.ErrPermission
	}
	return fs.impl.Symlink(file, target)
}

func (fs *fsSession) Readdir(dirPath string) ([]os.FileInfo, error) {
	if !(fs.isOwned(dirPath)) {
		return nil, os.ErrPermission
	}
	file, err := fs.impl.OpenFile(dirPath, os.O_RDONLY, 0400)
	if err != nil {
		return nil, err
	}
	dirinfo, err := file.Readdir(0)
	if err != nil {
		return nil, err
	}
	owned := make([]os.FileInfo, 0, len(dirinfo))
	for _, info := range dirinfo {
		fileName := info.Name()
		// dunno if name is a path or just basename
		baseName := filepath.Base(fileName)
		filePath := filepath.Join(dirPath, baseName)
		if !fs.isOwned(filePath) {
			continue
		}
		owned = append(owned, info)
	}
	return owned, nil
}

func (fs *fsSession) Readlink(path string) (string, error) {
	if !fs.isOwned(path) {
		return "", os.ErrPermission
	}
	linkPath, err := fs.impl.Readlink(path)
	if err != nil {
		return "", err
	}
	// TODO: is it correct to check ownership of whatever link is pointing to?
	if !fs.isOwned(path) {
		return "", os.ErrPermission
	}
	return linkPath, nil
}
