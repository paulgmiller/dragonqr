package game

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"gorm.io/plugin/optimisticlock"
)

type Player struct {
	Version        optimisticlock.Version `json:"-"` // Optimistic locking; excluded from legacy player JSON.
	ID             string                 `json:"id" gorm:"primaryKey"`
	Name           string                 `json:"name"`
	AdventurerName string                 `json:"adventurer_name"`
	MaxHealth      int                    `json:"max_health"`
	Health         int                    `json:"health"`
	Attack         int                    `json:"attack"`
	Armor          int                    `json:"armor"`
	Items          []string               `json:"items" gorm:"serializer:json"`
	Companions     []string               `json:"companions" gorm:"serializer:json"`
	Defeated       []string               `json:"defeated" gorm:"serializer:json"`
	Scanned        []string               `json:"scanned" gorm:"serializer:json"`
	Clues          []Clue                 `json:"clues" gorm:"serializer:json"`
	Combat         *Combat                `json:"combat,omitempty" gorm:"serializer:json"`
	DragonDefeated bool                   `json:"dragon_defeated"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
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

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
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
