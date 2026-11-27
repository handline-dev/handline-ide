package holder

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"
)

// Reader is one connection reading a holder's records.
type Reader struct {
	c  net.Conn
	br *bufio.Reader
}

// Open connects to the holder at socket and asks for the records after seq from.
func Open(ctx context.Context, socket string, from int64) (*Reader, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, err
	}
	if err := send(c, Request{Read: &from}); err != nil {
		c.Close()
		return nil, err
	}
	return &Reader{c: c, br: bufio.NewReader(c)}, nil
}

// Next is the next record; an error means the connection is gone (the holder ended, or another reader took its place).
func (r *Reader) Next() (Record, error) {
	line, err := r.br.ReadBytes('\n')
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := json.Unmarshal(line, &rec); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// Ack lets the holder drop every record up to seq.
func (r *Reader) Ack(seq int64) error { return send(r.c, Request{Ack: seq}) }

func (r *Reader) Close() error { return r.c.Close() }

// Write gives the CLI one line on its stdin.
func Write(socket string, line []byte) error {
	s := string(line)
	return ask(socket, Request{Write: &s})
}

// CloseInput ends the CLI's stdin: it finishes its turn and exits.
func CloseInput(socket string) error { return ask(socket, Request{Close: true}) }

func ask(socket string, req Request) error {
	c, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	if err := send(c, req); err != nil {
		return err
	}
	line, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		return err
	}
	var rep Reply
	if err := json.Unmarshal(line, &rep); err != nil {
		return err
	}
	if rep.Err != "" {
		return errors.New(rep.Err)
	}
	return nil
}

func send(c net.Conn, req Request) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = c.Write(append(b, '\n'))
	return err
}
