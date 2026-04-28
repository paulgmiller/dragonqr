package game

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Player struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	AdventurerName string    `json:"adventurer_name"`
	MaxHealth      int       `json:"max_health"`
	Health         int       `json:"health"`
	Attack         int       `json:"attack"`
	Armor          int       `json:"armor"`
	Items          []string  `json:"items"`
	Companions     []string  `json:"companions"`
	Defeated       []string  `json:"defeated"`
	Scanned        []string  `json:"scanned"`
	Clues          []Clue    `json:"clues"`
	Combat         *Combat   `json:"combat,omitempty"`
	DragonDefeated bool      `json:"dragon_defeated"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Clue struct {
	From string `json:"from"`
	Text string `json:"text"`
}

type Combat struct {
	CodeID      string        `json:"code_id"`
	EnemyHealth int           `json:"enemy_health"`
	IsDragon    bool          `json:"is_dragon"`
	Rounds      []CombatRound `json:"rounds"`
}

type CombatRound struct {
	PlayerRoll   int `json:"player_roll"`
	EnemyRoll    int `json:"enemy_roll"`
	PlayerDamage int `json:"player_damage"`
	EnemyDamage  int `json:"enemy_damage"`
	EnemyHealth  int `json:"enemy_health"`
	PlayerHealth int `json:"player_health"`
}

type Store struct {
	path    string
	players map[string]*Player
	mu      sync.Mutex
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path:    path,
		players: map[string]*Player{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Create(q *Quest, name string, adventurerName string) (*Player, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	p := &Player{
		ID:             id,
		Name:           name,
		AdventurerName: adventurerName,
		MaxHealth:      q.BaseHealth,
		Health:         q.BaseHealth,
		Attack:         q.BaseAttack,
		Armor:          q.BaseArmor,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	start := q.Start()
	p.markScanned(start.ID)
	p.addClue(start.ID, start.Clue)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.players[p.ID] = p
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return clonePlayer(p), nil
}

func (s *Store) Get(id string) (*Player, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.players[id]
	if !ok {
		return nil, false
	}
	return clonePlayer(p), true
}

func (s *Store) Update(id string, fn func(*Player) error) (*Player, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.players[id]
	if !ok {
		return nil, ErrPlayerNotFound
	}
	if err := fn(p); err != nil {
		return nil, err
	}
	p.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return clonePlayer(p), nil
}

var ErrPlayerNotFound = errors.New("player not found")

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read players: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	var players map[string]*Player
	if err := json.Unmarshal(data, &players); err != nil {
		return fmt.Errorf("parse players: %w", err)
	}
	if players != nil {
		s.players = players
	}
	return nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	data, err := json.MarshalIndent(s.players, "", "  ")
	if err != nil {
		return fmt.Errorf("encode players: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write players temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace players file: %w", err)
	}
	return nil
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func clonePlayer(p *Player) *Player {
	cp := *p
	cp.Items = append([]string(nil), p.Items...)
	cp.Companions = append([]string(nil), p.Companions...)
	cp.Defeated = append([]string(nil), p.Defeated...)
	cp.Scanned = append([]string(nil), p.Scanned...)
	cp.Clues = append([]Clue(nil), p.Clues...)
	if p.Combat != nil {
		combat := *p.Combat
		combat.Rounds = append([]CombatRound(nil), p.Combat.Rounds...)
		cp.Combat = &combat
	}
	return &cp
}

func (p *Player) HasScanned(id string) bool {
	return contains(p.Scanned, id)
}

func (p *Player) HasDefeated(id string) bool {
	return contains(p.Defeated, id)
}

func (p *Player) GearNames(q *Quest) []string {
	return p.codeTitles(q, p.Items, map[CodeType]bool{
		CodeWeapon: true,
		CodeArmor:  true,
	})
}

func (p *Player) CompanionNames(q *Quest) []string {
	return p.codeTitles(q, p.Companions, map[CodeType]bool{
		CodeCompanion: true,
	})
}

func (p *Player) CompanionCount(q *Quest) int {
	return len(p.codeTitles(q, p.Companions, map[CodeType]bool{
		CodeCompanion: true,
	}))
}

func (p *Player) codeTitles(q *Quest, stored []string, allowed map[CodeType]bool) []string {
	seen := map[string]bool{}
	var titles []string
	addCode := func(code Code) {
		if !allowed[code.Type] || seen[code.ID] {
			return
		}
		seen[code.ID] = true
		titles = append(titles, code.Title)
	}

	var unknown []string
	for _, value := range stored {
		if code, ok := q.Code(value); ok {
			addCode(code)
			continue
		}
		matched := false
		for _, code := range q.Codes {
			if code.Title == value {
				addCode(code)
				matched = true
				break
			}
		}
		if !matched && value != "" {
			unknown = append(unknown, value)
		}
	}
	for _, id := range p.Scanned {
		if code, ok := q.Code(id); ok {
			addCode(code)
		}
	}
	if len(titles) == 0 {
		for _, value := range unknown {
			if !seen[value] {
				seen[value] = true
				titles = append(titles, value)
			}
		}
	}
	return titles
}

func (p *Player) markScanned(id string) {
	if !contains(p.Scanned, id) {
		p.Scanned = append(p.Scanned, id)
	}
}

func (p *Player) addItem(id string) {
	if !contains(p.Items, id) {
		p.Items = append(p.Items, id)
	}
}

func (p *Player) addCompanion(id string) {
	if !contains(p.Companions, id) {
		p.Companions = append(p.Companions, id)
	}
}

func (p *Player) addDefeated(id string) {
	if !contains(p.Defeated, id) {
		p.Defeated = append(p.Defeated, id)
	}
}

func (p *Player) addClue(from string, text string) {
	if text == "" {
		return
	}
	for _, clue := range p.Clues {
		if clue.From == from {
			return
		}
	}
	p.Clues = append(p.Clues, Clue{From: from, Text: text})
}

func contains(values []string, value string) bool {
	for _, got := range values {
		if got == value {
			return true
		}
	}
	return false
}
