package game

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type CodeType string

const (
	CodeStart     CodeType = "start"
	CodeWeapon    CodeType = "weapon"
	CodeArmor     CodeType = "armor"
	CodeCompanion CodeType = "companion"
	CodeEnemy     CodeType = "enemy"
	CodeHealing   CodeType = "healing"
	CodeClue      CodeType = "clue"
	CodeDragon    CodeType = "dragon"
)

type Quest struct {
	Title              string             `yaml:"title"`
	Intro              string             `yaml:"intro"`
	StartCode          string             `yaml:"start_code"`
	DragonCode         string             `yaml:"dragon_code"`
	BaseHealth         int                `yaml:"base_health"`
	BaseAttack         int                `yaml:"base_attack"`
	BaseArmor          int                `yaml:"base_armor"`
	DragonRequirements DragonRequirements `yaml:"dragon_requirements"`
	VictoryText        string             `yaml:"victory_text"`
	Codes              []Code             `yaml:"codes"`

	byID map[string]Code
}

type DragonRequirements struct {
	MinAttack     int      `yaml:"min_attack"`
	MinArmor      int      `yaml:"min_armor"`
	MinCompanions int      `yaml:"min_companions"`
	Defeated      []string `yaml:"defeated"`
}

type Code struct {
	ID            string   `yaml:"id"`
	Type          CodeType `yaml:"type"`
	Title         string   `yaml:"title"`
	Label         string   `yaml:"label"`
	Description   string   `yaml:"description"`
	Clue          string   `yaml:"clue"`
	OrganizerNote string   `yaml:"organizer_note"`
	ImagePrompt   string   `yaml:"image_prompt"`
	Effects       Effects  `yaml:"effects"`
	Rewards       Effects  `yaml:"rewards"`
	Enemy         Enemy    `yaml:"enemy"`
}

type Effects struct {
	Health int `yaml:"health"`
	Attack int `yaml:"attack"`
	Armor  int `yaml:"armor"`
}

type Enemy struct {
	Health int `yaml:"health"`
	Attack int `yaml:"attack"`
	Armor  int `yaml:"armor"`
}

func LoadQuest(path string) (*Quest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read quest: %w", err)
	}

	var q Quest
	if err := yaml.Unmarshal(data, &q); err != nil {
		return nil, fmt.Errorf("parse quest: %w", err)
	}
	if err := q.Validate(); err != nil {
		return nil, err
	}
	return &q, nil
}

func (q *Quest) Validate() error {
	if q.Title == "" {
		return errors.New("quest title is required")
	}
	if q.StartCode == "" {
		return errors.New("start_code is required")
	}
	if q.DragonCode == "" {
		return errors.New("dragon_code is required")
	}
	if q.BaseHealth <= 0 {
		return errors.New("base_health must be positive")
	}
	if q.BaseAttack < 0 || q.BaseArmor < 0 {
		return errors.New("base_attack and base_armor must not be negative")
	}

	q.byID = make(map[string]Code, len(q.Codes))
	starts := 0
	dragons := 0
	for i, code := range q.Codes {
		if code.ID == "" {
			return fmt.Errorf("codes[%d] id is required", i)
		}
		if code.Title == "" {
			return fmt.Errorf("code %q title is required", code.ID)
		}
		if _, exists := q.byID[code.ID]; exists {
			return fmt.Errorf("duplicate code id %q", code.ID)
		}
		if !validCodeType(code.Type) {
			return fmt.Errorf("code %q has invalid type %q", code.ID, code.Type)
		}
		if code.Type == CodeStart {
			starts++
		}
		if code.Type == CodeDragon {
			dragons++
		}
		if code.Type == CodeEnemy || code.Type == CodeDragon {
			if code.Enemy.Health <= 0 {
				return fmt.Errorf("code %q enemy health must be positive", code.ID)
			}
			if code.Enemy.Attack < 0 || code.Enemy.Armor < 0 {
				return fmt.Errorf("code %q enemy attack and armor must not be negative", code.ID)
			}
		}
		if code.Effects.Health < 0 || code.Effects.Attack < 0 || code.Effects.Armor < 0 {
			return fmt.Errorf("code %q effects must not be negative", code.ID)
		}
		if code.Rewards.Health < 0 || code.Rewards.Attack < 0 || code.Rewards.Armor < 0 {
			return fmt.Errorf("code %q rewards must not be negative", code.ID)
		}
		q.byID[code.ID] = code
	}
	if starts != 1 {
		return fmt.Errorf("quest must have exactly one start code, found %d", starts)
	}
	if dragons != 1 {
		return fmt.Errorf("quest must have exactly one dragon code, found %d", dragons)
	}
	if _, ok := q.byID[q.StartCode]; !ok {
		return fmt.Errorf("start_code %q does not match a code id", q.StartCode)
	}
	if q.byID[q.StartCode].Type != CodeStart {
		return fmt.Errorf("start_code %q must reference a start code", q.StartCode)
	}
	if _, ok := q.byID[q.DragonCode]; !ok {
		return fmt.Errorf("dragon_code %q does not match a code id", q.DragonCode)
	}
	if q.byID[q.DragonCode].Type != CodeDragon {
		return fmt.Errorf("dragon_code %q must reference a dragon code", q.DragonCode)
	}
	for _, id := range q.DragonRequirements.Defeated {
		code, ok := q.byID[id]
		if !ok {
			return fmt.Errorf("dragon requirement references missing code %q", id)
		}
		if code.Type != CodeEnemy {
			return fmt.Errorf("dragon requirement %q must reference an enemy code", id)
		}
	}
	return nil
}

func (q *Quest) Code(id string) (Code, bool) {
	code, ok := q.byID[id]
	return code, ok
}

func (q *Quest) Start() Code {
	return q.byID[q.StartCode]
}

func (q *Quest) Dragon() Code {
	return q.byID[q.DragonCode]
}

func validCodeType(t CodeType) bool {
	switch t {
	case CodeStart, CodeWeapon, CodeArmor, CodeCompanion, CodeEnemy, CodeHealing, CodeClue, CodeDragon:
		return true
	default:
		return false
	}
}
