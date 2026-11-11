package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"bracketleru/internal/bracket"
)

// turnamen yang gak diubah lebih dari TTL harus kehapus waktu dibuka / di-list
func TestFileStoreExpiry(t *testing.T) {
	ctx := context.Background()
	st, _ := NewFileStore(t.TempDir())
	mk := func(id string, age time.Duration) {
		tr, _ := bracket.New("x", "", bracket.Single, []string{"a", "b"}, bracket.Settings{}, false)
		tr.ID, tr.OwnerID = id, "u1"
		tr.UpdatedAt = time.Now().Add(-age)
		data, _ := json.Marshal(tr)
		os.WriteFile(st.path(id), data, 0o644)
	}
	mk("baru1234", time.Hour)
	mk("lama1234", TTL+time.Hour)
	mk("lama5678", TTL+time.Minute)

	if _, err := st.Get(ctx, "baru1234"); err != nil {
		t.Fatal("turnamen baru harusnya ada:", err)
	}
	if _, err := st.Get(ctx, "lama1234"); !errors.Is(err, ErrNotFound) {
		t.Fatal("turnamen lama harusnya kehapus, err =", err)
	}
	if _, err := os.Stat(st.path("lama1234")); !os.IsNotExist(err) {
		t.Fatal("file turnamen lama masih ada")
	}
	list, _ := st.ListByOwner(ctx, "u1", 10)
	if len(list) != 1 || list[0].ID != "baru1234" {
		t.Fatalf("list harusnya cuma baru1234, dapat %d", len(list))
	}
	if _, err := os.Stat(st.path("lama5678")); !os.IsNotExist(err) {
		t.Fatal("ListByOwner harusnya ikut hapus yang kedaluwarsa")
	}
	// Save nge-reset umur
	tr, _ := st.Get(ctx, "baru1234")
	tr.UpdatedAt = time.Now().Add(-TTL * 2)
	st.Save(ctx, tr)
	if _, err := st.Get(ctx, "baru1234"); err != nil {
		t.Fatal("habis Save harusnya umurnya ke-reset:", err)
	}
}
