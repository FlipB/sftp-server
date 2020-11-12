package srv

import (
	"io"
	"io/ioutil"
	"net"
	"testing"
)

type FakeListener chan interface{}

// Accept waits for and returns the next connection to the listener.
func (l FakeListener) Accept() (net.Conn, error) {
	conn := <-l
	if err, ok := conn.(error); ok {
		return nil, err
	}
	return conn.(net.Conn), nil
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l FakeListener) Close() error {
	// Highly unusual to close on the receiving side of a channel.
	for {
		select {
		case _, open := <-l:
			if !open {
				// channel is already closed.
				return &net.OpError{} // FIXME make use of closed Conn error
			}
		default:
			// channel should now be drained (any listeners unblocked)
			close(l)
			return nil
		}
	}

}

// Addr returns the listener's network address.
func (l FakeListener) Addr() net.Addr {
	return &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 22,
		Zone: "",
	}
}

// Connect simulates an incoming connection. If Err is not nil the error is passed to the next Listener.
// Connect may block if fake listener is not buffered or the buffer is full.
func (l FakeListener) Connect(err error) net.Conn {
	if err != nil {
		l <- err
		return nil
	}
	alice, bob := net.Pipe()
	l <- alice
	return bob
}

var _ net.Listener = make(FakeListener, 0)

func TestServer_ServeSocket(t *testing.T) {
	type fields struct {
		debug          io.Writer
		conf           config
		serveChan      chan<- struct{}
		activeConns    int64
		onIdleCallback func(*Server)
	}
	type args struct {
		listener net.Listener
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "simple",
			args: args{
				listener: make(FakeListener, 1),
			},
			fields: fields{
				debug: ioutil.Discard,
				onIdleCallback: func(s *Server) {
					s.Close()
				},
				conf: config{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				debug:          tt.fields.debug,
				conf:           tt.fields.conf,
				serveChan:      tt.fields.serveChan,
				activeConns:    tt.fields.activeConns,
				onIdleCallback: tt.fields.onIdleCallback,
			}
			if err := s.ServeSocket(tt.args.listener); (err != nil) != tt.wantErr {
				t.Errorf("Server.ServeSocket() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
