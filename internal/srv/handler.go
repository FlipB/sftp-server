package srv

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
)

func newUserHandler(root string) *userRootHandler {
	ur := &userRootHandler{
		// fs:
		dperm:    0770,
		fperm:    0660,
		logger:   func(f string, a ...interface{}) { fmt.Printf("sftp: "+f+"\n", a...) },
		rootPath: root,
	}
	fs := fsAdapter{
		impl: fsImpl{
			sanitizer: ur.pathSanitizer,
			logger:    func(f string, a ...interface{}) { fmt.Printf("fs: "+f+"\n", a...) },
		},
	}
	ur.fs = fs
	return ur
}

type userRootHandler struct {
	fs     fsAdapter
	fperm  os.FileMode
	dperm  os.FileMode
	logger func(format string, args ...interface{})

	rootPath string
}

func (ur *userRootHandler) SftpHandler() sftp.Handlers {
	return sftp.Handlers{
		FileGet:  ur,
		FileCmd:  ur,
		FileList: ur,
		FilePut:  ur,
	}
}

func (ur userRootHandler) pathSanitizer(path string) (string, error) {
	orgPath := path
	if !filepath.IsAbs(path) {
		path = filepath.Join(ur.rootPath, path)
	}
	if filepath.IsAbs(path) {
		path = filepath.Join(ur.rootPath, path)
	}
	path = filepath.Clean(path)
	if !strings.HasPrefix(path, filepath.Clean(ur.rootPath)) {
		return ur.rootPath, fmt.Errorf("invalid file path %q", orgPath)
	}
	return path, nil
}

func (ur userRootHandler) log(format string, args ...interface{}) {
	if ur.logger == nil {
		return
	}
	ur.logger(format, args...)
}

func (ur *userRootHandler) Fileread(req *sftp.Request) (io.ReaderAt, error) {
	ur.log("Fileread request %q", req.Filepath)

	flags := req.Pflags()
	if !flags.Read {
		// sanity check
		return nil, os.ErrInvalid
	}

	return ur.fs.OpenFile(req.Filepath, req.Pflags(), ur.fperm)
}

func (ur *userRootHandler) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	ur.log("Filewrite request %q", req.Filepath)

	flags := req.Pflags()
	if !flags.Write {
		// sanity check
		return nil, os.ErrInvalid
	}

	// FIXME: handle newFileAttrFlags() args?
	requestPerm := req.Attributes().FileMode().Perm()
	ur.log("Filewrite parm = %v", requestPerm)

	return ur.fs.OpenFile(req.Filepath, req.Pflags(), ur.fperm)
}

func (ur *userRootHandler) Filecmd(req *sftp.Request) error {
	ur.log("Filecmd request %s %q", req.Method, req.Filepath)

	switch req.Method {
	case "Setstat":
		file, err := ur.fs.OpenFile(req.Filepath, sftp.FileOpenFlags{Write: true}, ur.fperm)
		if err != nil {
			return err
		}

		if req.AttrFlags().Size {
			return file.Truncate(int64(req.Attributes().Size))
		}

		return nil

	case "Rename":
		// SFTP-v2: "It is an error if there already exists a file with the name specified by newpath."
		// This varies from the POSIX specification, which allows limited replacement of target files.
		exists, err := ur.fs.Exists(req.Target)
		if err != nil {
			return err
		}
		if exists {
			return os.ErrExist
		}

		return ur.fs.Rename(req.Filepath, req.Target)

	case "Rmdir":
		return ur.fs.Rmdir(req.Filepath)

	case "Remove":
		// IEEE 1003.1 remove explicitly can unlink files and remove empty directories.
		// We use instead here the semantics of unlink, which is allowed to be restricted against directories.
		return ur.fs.Unlink(req.Filepath)

	case "Mkdir":
		return ur.fs.Mkdir(req.Filepath, ur.dperm)

	case "Link":
		return ur.fs.Link(req.Filepath, req.Target)

	case "Symlink":
		// NOTE: r.Filepath is the target, and r.Target is the linkpath.
		return ur.fs.Symlink(req.Filepath, req.Target)
	}

	return errors.New("unsupported")
}

func (ur *userRootHandler) Filelist(req *sftp.Request) (sftp.ListerAt, error) {
	ur.log("Filelist request %s %q", req.Method, req.Filepath)

	switch req.Method {
	case "List":
		files, err := ur.fs.Readdir(req.Filepath)
		if err != nil {
			return nil, err
		}
		return listerat(files), nil

	case "Stat":
		file, err := ur.fs.Stat(req.Filepath)
		if err != nil {
			return nil, err
		}
		return listerat{file}, nil

	case "Readlink":
		symlink, err := ur.fs.Readlink(req.Filepath)
		if err != nil {
			return nil, err
		}
		file, err := ur.fs.Stat(symlink)
		if err != nil {
			return nil, err
		}

		// SFTP-v2: The server will respond with a SSH_FXP_NAME packet containing only
		// one name and a dummy attributes value.
		return listerat{
			file,
		}, nil
	}

	return nil, errors.New("unsupported")
}

type listerat []os.FileInfo

// Modeled after strings.Reader's ReadAt() implementation
func (f listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	var n int
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}
	n = copy(ls, f[offset:])
	if n < len(ls) {
		return n, io.EOF
	}
	return n, nil
}

/*
 Do i need these below?
*/

const (
	sshFileXferAttrSize        = 0x00000001
	sshFileXferAttrUIDGID      = 0x00000002
	sshFileXferAttrPermissions = 0x00000004
	sshFileXferAttrACmodTime   = 0x00000008
	sshFileXferAttrExtented    = 0x80000000

	sshFileXferAttrAll = sshFileXferAttrSize | sshFileXferAttrUIDGID | sshFileXferAttrPermissions |
		sshFileXferAttrACmodTime | sshFileXferAttrExtented
)

// NOTE: can probably be read directly on request.
func newFileAttrFlags(flags uint32) sftp.FileAttrFlags {
	return sftp.FileAttrFlags{
		Size:        (flags & sshFileXferAttrSize) != 0,
		UidGid:      (flags & sshFileXferAttrUIDGID) != 0,
		Permissions: (flags & sshFileXferAttrPermissions) != 0,
		Acmodtime:   (flags & sshFileXferAttrACmodTime) != 0,
	}
}
