package srv

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/go-systemd/activation"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Server struct {
	debug          io.Writer
	conf           config
	serveChan      chan<- struct{}
	activeConns    int64
	onIdleCallback func(*Server)
}

type config struct {
	User        user
	HostKeyPub  string
	HostKeyPriv string
	DataDir     string

	MaxDataBytes int64
}

type user struct {
	Name string
	// Password is salted + hashed with sha256
	Password   string
	QuotaBytes int64
}

func (u user) credentialMatch(userName string, userPass []byte) bool {
	if userName != u.Name {
		return false
	}
	h := sha256.New()
	h.Write([]byte(userName))
	h.Write(userPass)

	return bytes.Equal(h.Sum(nil), []byte(u.Password))
}

func NewServer(rootDirPath, hostKeyPath, userName, userNameAndPasswordSha256 string, idleCb func(*Server)) (*Server, error) {

	//
	info, err := os.Stat(rootDirPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("error opening root dir %q: %w", rootDirPath, err)
	}
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("root %q does not exist", rootDirPath)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root path %q is not a directory", rootDirPath)
	}

	priv, pub, err := readOrCreateSSHKeys(hostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("error getting SSH keys: %w", err)
	}

	return &Server{
		debug:          os.Stdout,
		onIdleCallback: idleCb,
		conf: config{
			User: user{
				Name:       userName,
				Password:   userNameAndPasswordSha256,
				QuotaBytes: 0,
			},
			DataDir:      rootDirPath,
			HostKeyPriv:  priv,
			HostKeyPub:   pub,
			MaxDataBytes: 0,
		},
	}, nil
}

func readOrCreateSSHKeys(keyPath string) (priv string, pub string, err error) {

	if keyPath == "-" {
		// read from stdin instead of from file.
		return readKeysFromStdin()
	}

	privInfo, err := os.Stat(keyPath)
	if err != nil && !os.IsNotExist(err) {
		return "", "", fmt.Errorf("error opening file %q: %w", keyPath, err)
	}
	_, err2 := os.Stat(keyPath + ".pub")
	if err2 != nil && !os.IsNotExist(err2) {
		return "", "", fmt.Errorf("error opening file %q: %w", keyPath+".pub", err2)
	}
	if (os.IsNotExist(err) && os.IsNotExist(err2)) != (os.IsNotExist(err) || os.IsNotExist(err2)) {
		// only one file doesnt exist.
		return "", "", fmt.Errorf("key at %q not found", keyPath)
	}
	needsGeneration := os.IsNotExist(err) && os.IsNotExist(err2)
	if !needsGeneration {
		if !(privInfo.Mode() == 0600 || privInfo.Mode() == 0400) {
			return "", "", fmt.Errorf("key at %q has too permissive permissions", keyPath)
		}
	}

	if needsGeneration {
		if err := generateAndWriteSSHKeys(keyPath); err != nil {
			return "", "", fmt.Errorf("error generating and saving ssh keys at %q: %w", keyPath, err)
		}
	}

	privKey, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return "", "", fmt.Errorf("error reading private key %q: %w", keyPath, err)
	}
	pubKey, err := ioutil.ReadFile(keyPath + ".pub")
	if err != nil {
		return "", "", fmt.Errorf("error reading public key %q: %w", keyPath+".pub", err)
	}

	return string(privKey), string(pubKey), nil
}

func generateAndWriteSSHKeys(keyPath string) (err error) {

	privKeyPEM, pubKeyPEM, err := GenerateSSHKeysAsPEM()
	if err != nil {
		return err
	}

	privateKeyPath := keyPath
	if err := ioutil.WriteFile(privateKeyPath, privKeyPEM, 0400); err != nil {
		return err
	}

	publicKeyPath := keyPath + ".pub"
	if err := ioutil.WriteFile(publicKeyPath, pubKeyPEM, 0444); err != nil {
		return err
	}
	return nil
}

// GenerateSSHKeysAsPEM returns PEM encoded keys or an error
func GenerateSSHKeysAsPEM() (priv []byte, pub []byte, err error) {

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}
	privKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		},
	)

	pubKey := privateKey.Public()
	pubKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: x509.MarshalPKCS1PublicKey(pubKey.(*rsa.PublicKey)),
		},
	)

	return privKeyPEM, pubKeyPEM, nil
}

func readKeysFromStdin() (string, string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(scanPemRSABlock)

	privateKeyPEM := ""
	publicKeyPEM := ""
	for scanner.Scan() {

		pemBlockMaybe := scanner.Bytes()
		block, _ := pem.Decode(bytes.TrimSpace(pemBlockMaybe))

		switch block.Type {
		case "RSA PRIVATE KEY":
			privateKeyPEM = string(pemBlockMaybe)
		case "RSA PUBLIC KEY":
			publicKeyPEM = string(pemBlockMaybe)
		}
		if len(publicKeyPEM) != 0 && len(privateKeyPEM) != 0 {
			break
		}
	}
	err := scanner.Err()
	if err != nil {
		return privateKeyPEM, publicKeyPEM, fmt.Errorf("error reading SSH Host keys from stdin: %w", err)
	}
	return privateKeyPEM, publicKeyPEM, err
}

func scanPemRSABlock(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	//fmt.Printf("SCANNED: %s\n", data)
	// Attempt to scan a full PRIVATE KEY PEM block if possible
	if i := bytes.Index(data, []byte("-----BEGIN RSA PRIVATE KEY-----")); i > 0 {
		// junk before header start - return it unless whitespace
		// if it's only whitespace we should not return here.
		if len(bytes.TrimSpace(data[0:i])) > 0 {
			return i, data[0:i], nil
		}
	}
	if i := bytes.Index(data, []byte("-----BEGIN RSA PRIVATE KEY-----")); i >= 0 {
		// We're at a header start. Read until we get the end.
		if j := bytes.Index(data, []byte("-----END RSA PRIVATE KEY-----")); j == 0 {
			blockLen := j + len("-----END RSA PRIVATE KEY-----")
			return blockLen, bytes.TrimSpace(data[0:blockLen]), nil
		}
	}

	// Attempt to scan a full PUBLIC KEY PEM block if possible
	if i := bytes.Index(data, []byte("-----BEGIN RSA PUBLIC KEY-----")); i > 0 {
		// junk before header start.
		// if it's only whitespace we should not return here.
		if len(bytes.TrimSpace(data[0:i])) > 0 {
			return i, data[0:i], nil
		}
	}
	if i := bytes.Index(data, []byte("-----BEGIN RSA PUBLIC KEY-----")); i >= 0 {
		// We're at a header start. Read until we get the end.
		if j := bytes.Index(data, []byte("-----END RSA PUBLIC KEY-----")); j == 0 {
			blockLen := j + len("-----END RSA PUBLIC KEY-----")
			return blockLen, bytes.TrimSpace(data[0:blockLen]), nil
		}
	}

	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}

func (s *Server) log(format string, a ...interface{}) {
	if s.debug == nil {
		return
	}
	_, _ = fmt.Fprintf(s.debug, format, a...)
	_, _ = fmt.Fprintf(s.debug, "\n")
}

func (s *Server) passwordCallback(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
	constTime := time.After(500 * time.Millisecond)

	var perm *ssh.Permissions
	err := fmt.Errorf("password rejected for %q", c.User())

	if s.conf.User.credentialMatch(c.User(), pass) {
		perm = nil
		err = nil
	}

	<-constTime
	s.log("user %q password check, err = %v\n", c.User(), err)
	return perm, err
}

// NumConns returns the number of active connections
func (s *Server) NumConns() int64 {
	return atomic.LoadInt64(&s.activeConns)
}

func (s *Server) connect() {
	_ = atomic.AddInt64(&s.activeConns, 1)
}

func (s *Server) disconnect() {
	conns := atomic.AddInt64(&s.activeConns, -1)
	if conns == 0 && s.onIdleCallback != nil {
		go s.onIdleCallback(s)
	}
}

// Close stops serving
func (s *Server) Close() error {
	// FIXME make thread safe
	if s.serveChan == nil {
		return nil // not serving
	}
	close(s.serveChan)
	s.serveChan = nil
	return nil
}

// ServeSystemdSocket serves socket from systemd (for socket activated services)
func (s *Server) ServeSystemdSocket() error {
	listeners, err := activation.Listeners()
	if err != nil {
		return fmt.Errorf("error getting listeners from systemd: %w", err)
	}
	if len(listeners) != 1 {
		return fmt.Errorf("expected only 1 listener from systemd, got %d", len(listeners))
	}

	return s.ServeSocket(listeners[0])
}

// ServeSocket serves a listener
func (s *Server) ServeSocket(listener net.Listener) error {

	if s.serveChan != nil {
		return fmt.Errorf("already serving")
	}
	stopChan := make(chan struct{})
	s.serveChan = stopChan
	defer func() {
		stopChan = nil
	}()

	// wsftp for "write sftp" as in write only, i guess. I should rename this.
	sshConfig := &ssh.ServerConfig{
		ServerVersion:    "SSH-2.0-wsftp-v0.0.1",
		PasswordCallback: s.passwordCallback,
	}
	private, err := ssh.ParsePrivateKey([]byte(s.conf.HostKeyPriv))
	if err != nil {
		s.log("error parsing private key: %v", err)
		return err
	}
	sshConfig.AddHostKey(private)

	go func() {
		<-stopChan
		listener.Close()
	}()

	serversWg := sync.WaitGroup{}
serveLoop:
	for {
		nConn, err := listener.Accept()
		select {
		case <-stopChan:
			break serveLoop
		default:
		}
		if err != nil {
			s.log("error accepting connection: %v", err)
			time.Sleep(time.Second)
			continue
		}

		s.connect()
		serversWg.Add(1)
		go func() {
			err = s.handleSSHConnection(nConn, sshConfig, stopChan)
			if err != nil {
				s.log("error listening: %v", err)
			}
			s.disconnect()
			serversWg.Done()
		}()
	}

	serversWg.Wait()

	return nil
}

// Serve endpoint (interface ip and port string)
func (s *Server) Serve(endpoint string) error {

	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		s.log("error listening on %q: %v", endpoint, err)
		return err
	}
	s.log("Listening on %v", listener.Addr())

	return s.ServeSocket(listener)
}

func (s *Server) handleSSHConnection(nConn net.Conn, sshConfig *ssh.ServerConfig, stopChan <-chan struct{}) error {

	// Before use, a handshake must be performed on the incoming net.Conn.
	sconn, chans, reqs, err := ssh.NewServerConn(nConn, sshConfig)
	if err != nil {
		s.log("error performing SSH handshake with %q: %v", nConn.RemoteAddr().String(), err)
		return err
	}
	s.log("login from %q", sconn.User())

	// The incoming Request channel must be serviced.
	go ssh.DiscardRequests(reqs)

	// Service the incoming Channel channel.
	for newChannel := range chans {
		// Channels have a type, depending on the application level
		// protocol intended. In the case of an SFTP session, this is "subsystem"
		// with a payload string of "<length=4>sftp"
		s.log("incoming channel: %s", newChannel.ChannelType())
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			s.log("rejecting unknown channel type %s", newChannel.ChannelType())
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			s.log("error accepting channel: %v", err)
			return err
		}
		s.log("accepted channel")

		// Sessions have out-of-band requests such as "shell",
		// "pty-req" and "env".  Here we handle only the
		// "subsystem" request.
		go func(in <-chan *ssh.Request) {
			for req := range in {
				s.log("Request type: %s", req.Type)
				ok := false
				switch req.Type {
				case "subsystem":
					s.log("Subsystem: %s", req.Payload[4:])
					if string(req.Payload[4:]) == "sftp" {
						ok = true
					}
				}
				s.log("Rejected request of type %s", req.Type)
				req.Reply(ok, nil)
			}
		}(requests)

		handler, err := s.getHandlerForUser(sconn.User())
		if err != nil {
			s.log("error getting handler for user %s. Terminating connection.", sconn.User())
			break
		}
		server := sftp.NewRequestServer(channel, handler)
		go func() {
			<-stopChan
			server.Close()
		}()
		if err := server.Serve(); err == io.EOF {
			err := server.Close()
			if err != nil {
				s.log("error closing server on client disconnect: %v", err)
			}
			s.log("client %q disconnected", nConn.RemoteAddr())
			break
		} else if err != nil {
			err := server.Close()
			if err != nil {
				s.log("error closing server post error: %v", err)
			}
			s.log("sftp server ended with error: %v", err)
			break
		}
		break
	}
	err = sconn.Close()
	if err != nil {
		s.log("error closing SSH connection: %v", err)
	}

	return err
}

func (s *Server) getHandlerForUser(userName string) (sftp.Handlers, error) {
	user := s.conf.User
	s.log("Returning handler for user %s", user.Name)

	handler := newUserHandler(s.conf.DataDir)

	return handler.SftpHandler(), nil
}
