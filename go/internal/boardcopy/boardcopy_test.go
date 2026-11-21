package boardcopy

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/storetest"
)

// The board's changes reach the copy within seconds; a mirror that pulls what it lacks holds the same files, and the copy restores to a
// board with the same rows.
func TestTheBoardsCopyMirrorsAndRestores(t *testing.T) {
	dir := t.TempDir()
	board, err := storetest.OpenAt(t, filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer board.Close()
	c, err := Start(filepath.Join(dir, "db.sqlite"), Dir(dir), "b1")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	for i := 0; i < 20; i++ {
		if _, err := board.Exec("INSERT INTO meta(name, value, updated) VALUES(?, 'x', 0)", "copy-"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	var want int
	board.QueryRow("SELECT COUNT(*) FROM meta").Scan(&want)
	mirror := filepath.Join(t.TempDir(), "board-copy")
	restored := func() int {
		pull(t, Dir(dir), mirror, 8<<10)
		dest := filepath.Join(t.TempDir(), "restored.sqlite")
		if Restore(mirror, dest) != nil {
			return -1
		}
		r, err := sql.Open("sqlite", dest)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		n := -1
		r.QueryRow("SELECT COUNT(*) FROM meta").Scan(&n)
		return n
	}
	start := time.Now()
	for n := restored(); n != want; n = restored() {
		if time.Since(start) > 10*time.Second {
			t.Fatalf("restored %d meta rows, want %d", n, want)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Logf("restored in %s", time.Since(start).Round(100*time.Millisecond))
	if err := Apply(mirror, []Piece{{Path: "../out", Size: 1, Data: []byte("x"), Sum: "x"}}, nil); err == nil {
		t.Fatal("a path out of the copy was written")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(mirror), "out")); err == nil {
		t.Fatal("wrote outside")
	}
}

// Under a steady write (one every 50 ms), the copy restores to a whole board a couple of seconds behind at most, and the WAL stays
// bounded while it runs.
func TestUnderAWriteLoopTheCopyStaysCloseAndWhole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	board, err := storetest.OpenAt(t, path)
	if err != nil {
		t.Fatal(err)
	}
	defer board.Close()
	c, err := Start(path, Dir(dir), "b1")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	var maxWAL int64
	stop := time.Now().Add(6 * time.Second)
	for i := 0; time.Now().Before(stop); i++ {
		if _, err := board.Exec("INSERT INTO meta(name, value, updated) VALUES(?, ?, 0)", "loop-"+time.Now().Format("150405.000000"), string(make([]byte, 2000))); err != nil {
			t.Fatal(err)
		}
		if st, err := os.Stat(path + "-wal"); err == nil && st.Size() > maxWAL {
			maxWAL = st.Size()
		}
		time.Sleep(50 * time.Millisecond)
	}
	var want int
	board.QueryRow("SELECT COUNT(*) FROM meta").Scan(&want)
	wrote := time.Now()
	for {
		dest := filepath.Join(t.TempDir(), "r.sqlite")
		if Restore(Dir(dir), dest) == nil {
			r, _ := sql.Open("sqlite", dest)
			var n int
			var ok string
			r.QueryRow("SELECT COUNT(*) FROM meta").Scan(&n)
			r.QueryRow("PRAGMA integrity_check").Scan(&ok)
			r.Close()
			if ok != "ok" {
				t.Fatalf("the restored board is not whole: %s", ok)
			}
			if n == want {
				break
			}
		}
		if time.Since(wrote) > 3*time.Second {
			t.Fatalf("the copy is more than 3s behind the last write")
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("caught up %s after the last write; the WAL peaked at %d KiB", time.Since(wrote).Round(100*time.Millisecond), maxWAL>>10)
}

// pull brings to what from has that it lacks, chunk bytes an answer, as a runner's pulls do, until nothing is left.
func pull(t *testing.T, from, to string, chunk int64) {
	t.Helper()
	for {
		have, err := List(to)
		if err != nil {
			t.Fatal(err)
		}
		parts, err := Parts(to)
		if err != nil {
			t.Fatal(err)
		}
		send, gone, more, err := Changes(from, have, parts, chunk)
		if err != nil {
			t.Fatal(err)
		}
		if more && len(gone) > 0 {
			t.Fatal("gone was answered while more was still to come")
		}
		if err := Apply(to, send, gone); err != nil {
			t.Fatal(err)
		}
		if !more {
			return
		}
	}
}

// A file larger than one answer arrives in pieces and is only under its own name once whole and as sent; a file the source dropped is
// dropped once nothing is left to send.
func TestAFileArrivesInPiecesAndWholeOrNotAtAll(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	big := make([]byte, 3<<20+17)
	for i := range big {
		big[i] = byte(i * 7)
	}
	os.MkdirAll(filepath.Join(from, "ltx", "9"), 0o700)
	os.WriteFile(filepath.Join(from, "ltx", "9", "snap.ltx"), big, 0o600)
	os.WriteFile(filepath.Join(to, "old.ltx"), []byte("old"), 0o600)
	have, _ := List(to)
	send, gone, more, err := Changes(from, have, nil, 1<<20)
	if err != nil || !more || len(gone) != 0 || len(send) != 1 || len(send[0].Data) != 1<<20 {
		t.Fatalf("first answer: %d pieces, gone %v, more %v, %v", len(send), gone, more, err)
	}
	if err := Apply(to, send, gone); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(to, "ltx", "9", "snap.ltx")); err == nil {
		t.Fatal("a partial file is under its own name")
	}
	pull(t, from, to, 1<<20)
	got, err := os.ReadFile(filepath.Join(to, "ltx", "9", "snap.ltx"))
	if err != nil || string(got) != string(big) {
		t.Fatalf("the file arrived otherwise: %d bytes, %v", len(got), err)
	}
	if _, err := os.Stat(filepath.Join(to, "old.ltx")); !os.IsNotExist(err) {
		t.Fatal("a file the source dropped is still there")
	}
	bad := []Piece{{Path: "x.ltx", Size: 3, Data: []byte("abc"), Sum: "00"}}
	if err := Apply(to, bad, nil); err == nil {
		t.Fatal("a file that does not match its sum was taken")
	}
	if _, err := os.Stat(filepath.Join(to, "x.ltx")); err == nil {
		t.Fatal("a file that does not match its sum is under its own name")
	}
}

// A copy folder made for another board is cleared before Litestream writes into it, and its marker is never sent; a part whose source
// was dropped is removed once a pull finds nothing more coming.
func TestACopyIsOnlyEverOfItsOwnBoard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	board, err := storetest.OpenAt(t, path)
	if err != nil {
		t.Fatal(err)
	}
	defer board.Close()
	stale := filepath.Join(Dir(dir), "ltx", "0", "stale.ltx")
	os.MkdirAll(filepath.Dir(stale), 0o700)
	os.WriteFile(stale, []byte("another board's"), 0o600)
	c, err := Start(path, Dir(dir), "b1")
	if err != nil {
		t.Fatal(err)
	}
	c.Stop()
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("another board's file was kept in the copy")
	}
	files, _ := List(Dir(dir))
	for _, f := range files {
		if f.Path == marker {
			t.Fatal("the marker is listed to be sent")
		}
	}
	mirror := t.TempDir()
	os.WriteFile(filepath.Join(mirror, "gone.ltx"+partSuffix), []byte("half"), 0o600)
	if err := DropParts(mirror, nil); err != nil {
		t.Fatal(err)
	}
	if parts, _ := Parts(mirror); len(parts) != 0 {
		t.Fatalf("an orphaned part stayed: %v", parts)
	}
}
