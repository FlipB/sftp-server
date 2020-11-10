package srv

import (
	"io"
	"os"
)

// File is a file abstraction - implemented by *os.File
type File interface {
	//io.Reader
	//io.Writer
	//io.Seeker
	io.ReaderAt
	io.WriterAt
	io.Closer

	Readdir(n int) ([]os.FileInfo, error)
	Truncate(size int64) error
}

// fsImpl provides file system access. Thinnest possible Facade over `os` package.
// Also allows for sanitizing filepaths.
// All errors from underlying implementation are retuned as is.
type fsImpl struct {
	sanitizer func(string) (string, error)
	logger    func(format string, args ...interface{})
}

func (fs fsImpl) log(f string, args ...interface{}) {
	if fs.logger == nil {
		return
	}
	fs.logger(f, args...)
}

func (fs fsImpl) sanitize(prevErr error, path string) (string, error) {
	if prevErr != nil {
		return "", prevErr
	}
	if fs.sanitizer == nil {
		return path, nil
	}
	sanitizedPath, err := fs.sanitizer(path)
	fs.log("sanitize error = %v, path = %s, sanitized = %s", err, path, sanitizedPath)
	return sanitizedPath, err
}

func (fs fsImpl) OpenFile(path string, flags int, perm os.FileMode) (File, error) {
	path, err := fs.sanitize(nil, path)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, flags, perm)
	fs.log("OpenFile path = %q, err = %v", path, err)
	return file, err
}

func (fs fsImpl) Stat(path string) (os.FileInfo, error) {
	path, err := fs.sanitize(nil, path)
	if err != nil {
		return nil, err
	}
	fileinfo, err := os.Stat(path)
	fs.log("Stat path = %q, err = %v", path, err)
	return fileinfo, err
}

func (fs fsImpl) Readlink(path string) (string, error) {
	path, err := fs.sanitize(nil, path)
	if err != nil {
		return "", err
	}
	linkPath, err := os.Readlink(path)
	fs.log("Readlink path = %q, err = %v", path, err)
	return linkPath, err
}

func (fs fsImpl) Rename(from, to string) error {
	from, err := fs.sanitize(nil, from)
	to, err = fs.sanitize(nil, to)
	if err != nil {
		return err
	}
	err = os.Rename(from, to)
	fs.log("Rename from = %q, to = %q, err = %v", from, to, err)
	return err
}

func (fs *fsImpl) Remove(dirPath string) error {
	dirPath, err := fs.sanitize(nil, dirPath)
	if err != nil {
		return err
	}
	err = os.Remove(dirPath)
	fs.log("Remove path = %q, err = %v", dirPath, err)
	return err
}

func (fs fsImpl) Mkdir(path string, perm os.FileMode) error {
	path, err := fs.sanitize(nil, path)
	if err != nil {
		return err
	}
	err = os.Mkdir(path, perm)
	fs.log("Mkdir path = %q, err = %v", path, err)
	return err
}

func (fs fsImpl) Link(file, target string) error {
	file, err := fs.sanitize(nil, file)
	target, err = fs.sanitize(nil, target)
	if err != nil {
		return err
	}
	err = os.Link(file, target)
	fs.log("Link file = %q, target = %q, err = %v", file, target, err)
	return err
}

func (fs fsImpl) Symlink(file, target string) error {
	file, err := fs.sanitize(nil, file)
	target, err = fs.sanitize(nil, target)
	if err != nil {
		return err
	}
	err = os.Symlink(file, target)
	fs.log("Symlink file = %q, target = %q, err = %v", file, target, err)
	return err
}
