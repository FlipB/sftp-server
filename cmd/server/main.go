package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/flipb/sftp-server/internal/srv"
)

var (
	userName string

	userPassPlaintext         string
	userNameAndPasswordSha256 string

	endpoint    string
	rootPath    string
	hostKeyPath string
)

func main() {
	flag.StringVar(&rootPath, "root", "./sftproot", "root directory to serve over SFTP")
	flag.StringVar(&endpoint, "endpoint", "", "endpoint to serve SFTP on (mutually exclusive with socket arg)")
	flag.StringVar(&hostKeyPath, "hostkey", "./id_rsa", "SSH hostkey to use for SFTP server (written to if not existing). If - PEM key is read from stdin.")
	flag.StringVar(&userName, "user", "root", "name of SFTP user")
	flag.StringVar(&userPassPlaintext, "plaintextPassword", "", "plaintext password of SFTP user (discouraged)")
	flag.StringVar(&userNameAndPasswordSha256, "passwordHash", "", "user name and password hashed with sha256 encoded as hex")
	justHash := flag.Bool("hash", false, "return hashed username and password (for use with -passwordHash) and exit.")
	justGenerate := flag.Bool("generate", false, "generate SSH host key and write to stdout and exit.")
	idleExit := flag.Bool("exit", false, "exit when idle")
	systemdSocket := flag.Bool("socket", false, "serve systemd socket (mutually exclusive with endpoint arg)")

	flag.Parse()

	if *justGenerate {
		priv, pub, err := srv.GenerateSSHKeysAsPEM()
		if err != nil {
			log.Fatalf("error generating SSH keys: %v", err)
		}
		_, _ = io.Copy(os.Stdout, bytes.NewReader(priv))
		_, _ = io.Copy(os.Stdout, bytes.NewReader(pub))
		os.Exit(0)
	}

	if len(userName) < 4 || len(userName) > 32 {
		log.Fatalf("user name %q too long or short", userName)
	}
	if userPassPlaintext == "" && userNameAndPasswordSha256 == "" {
		log.Fatalf("user password required")
	}
	if userNameAndPasswordSha256 != "" {
		hashBytes, err := hex.DecodeString(userNameAndPasswordSha256)
		if err != nil {
			log.Fatalf("error decoding hash as hexa decimal string: %v", err)
		}
		userNameAndPasswordSha256 = string(hashBytes)
	}
	if userNameAndPasswordSha256 == "" && len(userPassPlaintext) > 0 {
		userNameAndPasswordSha256 = plaintextToHash(userName, userPassPlaintext)
	}
	if *justHash {
		fmt.Printf("%0x\n", userNameAndPasswordSha256)
		os.Exit(0)
	}

	var idleCb func(*srv.Server)
	if *idleExit {
		idleCb = stopIfIdle
	}

	if len(endpoint) == 0 && !*systemdSocket {
		log.Fatalf("no socket or endpoint specified")
		os.Exit(1)
	}
	if len(endpoint) > 0 && *systemdSocket {
		log.Fatalf("both socket or endpoint specified")
		os.Exit(1)
	}

	sftpSrv, err := srv.NewServer(rootPath, hostKeyPath, userName, userNameAndPasswordSha256, idleCb)
	if err != nil {
		log.Fatalf("unable to initalize server: %v", err)
	}

	if *systemdSocket {
		if err := sftpSrv.ServeSystemdSocket(); err != nil {
			log.Fatalf("error serving systemd socket: %v", err)
		}
	} else {
		if err := sftpSrv.Serve(endpoint); err != nil {
			log.Fatalf("error serving endpoint %q: %v", endpoint, err)
		}
	}

	return
}

func stopIfIdle(s *srv.Server) {
	<-time.After(10 * time.Second)
	if s.NumConns() == 0 {
		s.Close()
	}
}

func plaintextToHash(userName, password string) string {
	h := sha256.New()
	h.Write([]byte(userName))
	h.Write([]byte(password))

	return string(h.Sum(nil))
}
