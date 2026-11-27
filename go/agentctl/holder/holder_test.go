package holder

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// cli is a stand-in CLI: it prints a secret line, then echoes each input line as JSON until stdin ends, then prints bye.
const cli = `printf '{"secret":"S"}\n'; while IFS= read -r l; do printf '{"echo":"%s"}\n' "$l"; done; printf 'bye\n'`

func start(t *testing.T, cfg Config) (string, chan int) {
	t.Helper()
	dir, err := os.MkdirTemp("", "hld")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	cfg.Socket = filepath.Join(dir, "s")
	cfg.Argv = []string{"/bin/sh", "-c", cli}
	if cfg.Linger == 0 {
		cfg.Linger = 5 * time.Second
	}
	done := make(chan int, 1)
	go func() {
		code, err := Run(context.Background(), cfg)
		if err != nil {
			t.Error(err)
		}
		done <- code
	}()
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(cfg.Socket); err == nil {
			return cfg.Socket, done
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no socket")
	return "", nil
}

func next(t *testing.T, r *Reader) Record {
	t.Helper()
	rec, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestTheSecretLineIsNeverKeptAndReadyTakesItsPlace(t *testing.T) {
	sock, done := start(t, Config{Init: [][]byte{[]byte("hi")}, Secret: `"secret"`})
	r, err := Open(context.Background(), sock, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if rec := next(t, r); rec.Holder != "ready" || len(rec.Line) != 0 {
		t.Fatalf("%+v", rec)
	}
	if rec := next(t, r); string(rec.Line) != `{"echo":"hi"}` {
		t.Fatalf("%s", rec.Line)
	}
	if err := CloseInput(sock); err != nil {
		t.Fatal(err)
	}
	if rec := next(t, r); string(rec.Line) != `"bye"` {
		t.Fatalf("a line that is not JSON is kept as text: %s", rec.Line)
	}
	if rec := next(t, r); rec.Holder != "exit" || rec.Code != 0 {
		t.Fatalf("%+v", rec)
	}
	_ = r.Ack(4)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the holder did not end once everything was read")
	}
}

func TestANewReaderGoesOnFromItsCursorAndAckedRecordsAreGone(t *testing.T) {
	sock, _ := start(t, Config{})
	if err := Write(sock, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := Write(sock, []byte("b")); err != nil {
		t.Fatal(err)
	}
	r, _ := Open(context.Background(), sock, 0)
	var seen []string
	for len(seen) < 3 {
		seen = append(seen, string(next(t, r).Line))
	}
	if strings.Join(seen, " ") != `{"secret":"S"} {"echo":"a"} {"echo":"b"}` {
		t.Fatal(seen)
	}
	_ = r.Ack(2)
	time.Sleep(100 * time.Millisecond) // the ack is read on its own connection
	// a second reader (the daemon after a restart) takes over from seq 1: what was acked is not sent again
	r2, err := Open(context.Background(), sock, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	if rec := next(t, r2); rec.Seq != 3 || string(rec.Line) != `{"echo":"b"}` {
		t.Fatalf("%+v %s", rec, rec.Line)
	}
	if _, err := r.Next(); err == nil {
		t.Fatal("the first reader was not let go")
	}
	_ = Write(sock, []byte("c"))
	if rec := next(t, r2); string(rec.Line) != `{"echo":"c"}` {
		t.Fatalf("%s", rec.Line)
	}
}

func TestOverTheBoundTheOldestGoAndTheReaderIsToldHowMany(t *testing.T) {
	sock, _ := start(t, Config{MaxBytes: 3 * 80})
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		_ = Write(sock, []byte(s))
	}
	time.Sleep(300 * time.Millisecond)
	r, _ := Open(context.Background(), sock, 0)
	defer r.Close()
	rec := next(t, r)
	if rec.Holder != "gap" || rec.Lost == 0 {
		t.Fatalf("%+v", rec)
	}
	if rec2 := next(t, r); rec2.Seq != rec.Seq+rec.Lost+1 {
		t.Fatalf("after the gap: %+v", rec2)
	}
}

func TestTheHolderEndsAfterLingerWithNobodyReading(t *testing.T) {
	sock, done := start(t, Config{Linger: 100 * time.Millisecond})
	_ = CloseInput(sock)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("it waited for a reader past its linger")
	}
}

func TestASocketAKilledHolderLeftIsNotALiveOneAndAServingOneIsNotTakenOver(t *testing.T) {
	sock, _ := start(t, Config{})
	if !Alive(sock) {
		t.Fatal("a serving holder is alive")
	}
	if _, err := Run(context.Background(), Config{Argv: []string{"/bin/true"}, Socket: sock}); err == nil {
		t.Fatal("a second holder took the socket of a serving one")
	}
	if !Alive(sock) {
		t.Fatal("the refused one removed the serving one's socket")
	}
	os.WriteFile(PidFile(sock), []byte("999999"), 0o600)
	if Alive(sock) {
		t.Fatal("a socket whose holder is gone counts as alive")
	}
}

func TestAnOverLongLineIsDroppedWithAGapAndReadingGoesOn(t *testing.T) {
	br := bufio.NewReaderSize(strings.NewReader("ok\n"+strings.Repeat("x", maxLine+10)+"\nafter\n"), 16)
	var got []string
	for {
		line, long, err := readLine(br)
		if long {
			got = append(got, "<gap>")
		} else if len(line) > 0 {
			got = append(got, string(line))
		}
		if err != nil {
			break
		}
	}
	if strings.Join(got, ",") != "ok,<gap>,after" {
		t.Fatal(got)
	}
}
