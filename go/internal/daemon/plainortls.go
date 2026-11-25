package daemon

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

// plainOrTLS serves TLS and plain HTTP on one listener: a connection that opens with a TLS handshake is given cfg's certificate, any
// other is served plain, as the Tailscale door always was (the tailnet encrypts it), so an app paired on ws:// keeps working.
func plainOrTLS(ln net.Listener, cfg *tls.Config) net.Listener {
	l := &sniffer{Listener: ln, cfg: cfg, conns: make(chan net.Conn), done: make(chan struct{})}
	go l.run()
	return l
}

type sniffer struct {
	net.Listener
	cfg   *tls.Config
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
	err   error // why the inner listener stopped, set before done closes
}

// tlsHandshake is the first byte of a TLS record that carries a handshake.
const tlsHandshake = 0x16

// run accepts until the listener is closed. Any other error (out of descriptors) is waited out as http.Server does, up to a second.
func (l *sniffer) run() {
	var wait time.Duration
	for {
		c, err := l.Listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			l.stop(err)
			return
		}
		if err != nil {
			wait = min(max(2*wait, 5*time.Millisecond), time.Second)
			select {
			case <-time.After(wait):
			case <-l.done:
				return
			}
			continue
		}
		wait = 0
		go l.sort(c) // the first byte is read off the accept loop: a client that sends nothing holds up no one else
	}
}

func (l *sniffer) sort(c net.Conn) {
	c.SetReadDeadline(time.Now().Add(rpc.RecvTimeout))
	r := bufio.NewReader(c)
	first, err := r.Peek(1)
	c.SetReadDeadline(time.Time{})
	if err != nil {
		c.Close()
		return
	}
	var out net.Conn = &peeked{Conn: c, r: r}
	if first[0] == tlsHandshake {
		out = tls.Server(out, l.cfg)
	}
	select {
	case l.conns <- out:
	case <-l.done:
		c.Close()
	}
}

func (l *sniffer) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		if l.err != nil {
			return nil, l.err
		}
		return nil, net.ErrClosed
	}
}

func (l *sniffer) Close() error { return l.stop(nil) }

// stop closes the listener once; err is why, when the inner listener stopped by itself.
func (l *sniffer) stop(err error) error {
	var cerr error
	l.once.Do(func() {
		l.err = err
		cerr = l.Listener.Close()
		close(l.done)
	})
	return cerr
}

// peeked is a connection whose first bytes were read into r to sort it, and are read again from there.
type peeked struct {
	net.Conn
	r *bufio.Reader
}

func (p *peeked) Read(b []byte) (int, error) { return p.r.Read(b) }
