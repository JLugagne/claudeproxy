package usage

import (
	"database/sql"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

type publisherFunc func(domain.Usage)

func (f publisherFunc) Publish(u domain.Usage) { f(u) }

func TestStorePublishesCommittedRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")

	var (
		mu        sync.Mutex
		published []domain.Usage
		visible   []int
	)
	// The callback checks that the row is already readable when it is
	// published: the batch must be committed first.
	fake := publisherFunc(func(u domain.Usage) {
		count := -1
		if db, err := sql.Open("sqlite", path); err == nil {
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM usage`).Scan(&n); err == nil {
				count = n
			}
			_ = db.Close()
		}
		mu.Lock()
		published = append(published, u)
		visible = append(visible, count)
		mu.Unlock()
	})

	store, err := Open(path, slog.New(slog.DiscardHandler), WithPublisher(fake))
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Usage{
		Time:        time.Unix(1700000000, 0),
		Attribution: domain.Attribution{Project: "owner/repo", Session: "s1"},
		Provider:    "openrouter",
		Status:      200,
	}
	store.Record(want)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(published) != 1 {
		t.Fatalf("published %d rows, want 1", len(published))
	}
	if published[0].Project != want.Project || published[0].Session != want.Session || published[0].Status != want.Status {
		t.Errorf("published %+v, want %+v", published[0], want)
	}
	if visible[0] != 1 {
		t.Errorf("row count at publish time = %d, want 1 (the row must be committed first)", visible[0])
	}
}
