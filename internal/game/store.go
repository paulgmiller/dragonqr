package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var ErrPlayerNotFound = errors.New("player not found")

type Store struct {
	db *gorm.DB
}

// NewStore accepts a SQLite filename or a PostgreSQL connection URL.
func NewStore(database string) (*Store, error) {
	var driver gorm.Dialector
	isPostgres := strings.HasPrefix(database, "postgresql://") || strings.HasPrefix(database, "postgres://")
	if isPostgres {
		driver = postgres.Open(database)
	} else {
		if database == "" {
			return nil, errors.New("database path is empty")
		}
		if database != ":memory:" && !strings.HasPrefix(database, "file:") {
			if err := os.MkdirAll(filepath.Dir(database), 0o755); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
		}
		driver = sqlite.Open(database)
	}
	db, err := gorm.Open(driver, &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time { return time.Now().UTC() },
		// Each write is one atomic statement; avoid explicit transactions that
		// would require application-level serialization retries in CockroachDB.
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	s := &Store{db: db}
	conn, err := db.DB()
	if err != nil {
		return nil, err
	}
	if !isPostgres {
		conn.SetMaxOpenConns(1)
		if err := db.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
			conn.Close()
			return nil, fmt.Errorf("configure SQLite: %w", err)
		}
	}
	if err := db.AutoMigrate(&Player{}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate players: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	conn, err := s.db.DB()
	if err != nil {
		return err
	}
	return conn.Close()
}

func (s *Store) Create(q *Quest, name, adventurerName string) (*Player, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	row := Player{
		ID: id, Name: name, AdventurerName: adventurerName,
		MaxHealth: q.BaseHealth, Health: q.BaseHealth,
		Attack: q.BaseAttack, Armor: q.BaseArmor,
	}
	start := q.Start()
	row.markScanned(start.ID)
	row.addClue(start.ID, start.Clue)
	if err := s.db.Create(&row).Error; err != nil {
		return nil, fmt.Errorf("create player: %w", err)
	}
	return &row, nil
}

func (s *Store) Get(id string) (*Player, error) {
	var p Player
	err := s.db.First(&p, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlayerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read player: %w", err)
	}
	return &p, nil
}

// Update retries concurrent changes using a version condition. The callback may
// run more than once and must only mutate the supplied player.
func (s *Store) Update(id string, fn func(*Player) error) (*Player, error) {
	for attempt := 0; attempt < 20; attempt++ {
		row, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		version := row.Version
		if err := fn(row); err != nil {
			return nil, err
		}
		row.ID = id
		result := s.db.Model(row).
			Select("*").Updates(row)
		if result.Error != nil {
			return nil, fmt.Errorf("update player: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			// Restore the loaded metadata and reflect the plugin's SQL increment.
			row.Version = version
			row.Version.Int64 = version.Int64 + 1
			return row, nil
		}
		time.Sleep(time.Duration(attempt+1) * time.Millisecond)
	}
	return nil, errors.New("player update exceeded concurrent retry limit")
}

// ImportJSON preserves existing SQL rows and leaves the source file untouched.
// It is safe to rerun after a partial import.
func (s *Store) ImportJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read legacy players: %w", err)
	}
	var players map[string]*Player
	if err := json.Unmarshal(data, &players); err != nil {
		return fmt.Errorf("parse legacy players: %w", err)
	}
	for id, p := range players {
		if p == nil || id == "" || p.ID != id {
			return fmt.Errorf("invalid legacy player %q", id)
		}
	}
	for _, p := range players {
		if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(p).Error; err != nil {
			return fmt.Errorf("import player %q: %w", p.ID, err)
		}
	}
	return nil
}
