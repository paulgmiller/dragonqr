package game

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func storeQuest(t *testing.T) *Quest {
	t.Helper()
	q := &Quest{Title: "Test Quest", BaseHealth: 10, BaseAttack: 2, BaseArmor: 1, StartCode: "start", DragonCode: "dragon",
		Codes: []Code{
			{ID: "start", Title: "Start", Type: CodeStart, Clue: "Find the sword"},
			{ID: "dragon", Title: "Dragon", Type: CodeDragon, Enemy: Enemy{Health: 10}},
		}}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	return q
}

func TestStorePersistsPlayerAndCombat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "players.db")
	s := openTestStore(t, path)
	createdAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s.db.Config.NowFunc = func() time.Time { return createdAt }
	p, err := s.Create(storeQuest(t), "Name", "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if !p.CreatedAt.Equal(createdAt) || !p.UpdatedAt.Equal(createdAt) {
		t.Fatalf("creation timestamps = %v, %v; want %v", p.CreatedAt, p.UpdatedAt, createdAt)
	}
	updatedAt := createdAt.Add(time.Hour)
	s.db.Config.NowFunc = func() time.Time { return updatedAt }
	p, err = s.Update(p.ID, func(p *Player) error {
		p.Items = []string{"sword", "Legacy Armor"}
		p.Companions = []string{"fox"}
		p.Defeated = []string{"bat"}
		p.Combat = &Combat{CodeID: "dragon", EnemyHealth: 8, IsDragon: true,
			Rounds: []CombatRound{{PlayerRoll: 3, EnemyRoll: 2, PlayerHealth: 5}}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !p.CreatedAt.Equal(createdAt) || !p.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("update timestamps = %v, %v; want %v, %v", p.CreatedAt, p.UpdatedAt, createdAt, updatedAt)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	got, err := s.Get(p.ID)
	if err != nil || !reflect.DeepEqual(got, p) {
		t.Fatalf("reopened player = %+v, error = %v; want %+v", got, err, p)
	}
	// Returned values must not mutate persisted state.
	got.Items[0] = "changed"
	got.Combat.EnemyHealth = 99
	got, err = s.Get(p.ID)
	if err != nil || !reflect.DeepEqual(got, p) {
		t.Fatalf("read changed persistent state: %+v, %v", got, err)
	}
	_, err = s.Update(p.ID, func(p *Player) error {
		p.Combat = nil
		p.Health = 0
		p.Armor = 0
		p.Items = nil
		p.DragonDefeated = false
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.Get(p.ID)
	if err != nil || got.Combat != nil || got.Health != 0 || got.Armor != 0 || len(got.Items) != 0 {
		t.Fatalf("zero values did not persist: %+v, %v", got, err)
	}
}

func TestStoreUpdateFailureAndMissingPlayer(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "players.db"))
	p, err := s.Create(storeQuest(t), "Hero", "Hero")
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("rejected change")
	_, err = s.Update(p.ID, func(p *Player) error {
		p.Health = 0
		p.Scanned = append(p.Scanned, "sword")
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v", err)
	}
	got, err := s.Get(p.ID)
	if err != nil || !reflect.DeepEqual(got, p) {
		t.Fatalf("failed update changed player: %+v, %v", got, err)
	}
	if _, err := s.Get("missing"); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("missing Get error = %v", err)
	}
	if _, err := s.Update("missing", func(*Player) error { t.Fatal("callback called"); return nil }); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("missing Update error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(p.ID); err == nil || errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("database failure treated as missing player: %v", err)
	}
}

func TestStoreConcurrentUpdatesAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "players.db")
	stores := []*Store{openTestStore(t, path), openTestStore(t, path)}
	p, err := stores[0].Create(storeQuest(t), "Hero", "Hero")
	if err != nil {
		t.Fatal(err)
	}
	var ready sync.WaitGroup
	ready.Add(2)
	release := make(chan struct{})
	errs := make(chan error, 2)
	for _, s := range stores {
		go func(s *Store) {
			first := true
			_, err := s.Update(p.ID, func(p *Player) error {
				if first {
					first = false
					ready.Done()
					<-release
				}
				p.Attack++
				return nil
			})
			errs <- err
		}(s)
	}
	ready.Wait()
	close(release)
	for range stores {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	got, err := stores[0].Get(p.ID)
	if err != nil || got.Attack != p.Attack+2 {
		t.Fatalf("concurrent progress lost: %+v, %v", got, err)
	}
}

func TestStoreImportJSONPreservesProgress(t *testing.T) {
	dir := t.TempDir()
	s := openTestStore(t, filepath.Join(dir, "players.db"))
	legacy := &Player{ID: "legacy", AdventurerName: "Hero", Health: 4,
		CreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
		Items:     []string{"Old Sword"}, Companions: []string{"Old Fox"},
		Scanned: []string{"start", "sword"}, Clues: []Clue{{From: "start", Text: "Go"}},
		Combat: &Combat{CodeID: "bat", EnemyHealth: 2, Rounds: []CombatRound{{PlayerRoll: 6}}}}
	missingTimestamps := &Player{ID: "missing-timestamps", AdventurerName: "Hero"}
	data, err := json.Marshal(map[string]*Player{legacy.ID: legacy, missingTimestamps.ID: missingTimestamps})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "players.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.ImportJSON(path); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Version = got.Version // SQL locking metadata is absent from legacy JSON.
	if !reflect.DeepEqual(got, legacy) {
		t.Fatalf("imported player = %+v, %v; want %+v", got, err, legacy)
	}
	imported, err := s.Get(missingTimestamps.ID)
	if err != nil {
		t.Fatal(err)
	}
	if imported.CreatedAt.IsZero() || imported.UpdatedAt.IsZero() {
		t.Fatalf("missing import timestamps were not populated: %+v", imported)
	}
	_, err = s.Update(legacy.ID, func(p *Player) error { p.Health = 8; p.Combat = nil; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ImportJSON(path); err != nil {
		t.Fatal(err)
	}
	got, err = s.Get(legacy.ID)
	if err != nil || got.Health != 8 || got.Combat != nil {
		t.Fatalf("repeat import overwrote progress: %+v, %v", got, err)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || string(unchanged) != string(data) {
		t.Fatalf("legacy file changed: %v", err)
	}
}
