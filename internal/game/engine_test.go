package game

import "testing"

func TestApplyScanAppliesPowerupOnlyOnce(t *testing.T) {
	q := testQuest()
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	p := &Player{ID: "p1", MaxHealth: 10, Health: 10, Attack: 2}
	code, _ := q.Code("sword")

	ApplyScan(&q, p, code)
	ApplyScan(&q, p, code)

	if p.Attack != 5 {
		t.Fatalf("Attack = %d, want 5", p.Attack)
	}
	if len(p.Items) != 1 {
		t.Fatalf("Items length = %d, want 1", len(p.Items))
	}
}

func TestApplyScanEnemyCannotBeFarmed(t *testing.T) {
	q := testQuest()
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	p := &Player{ID: "p1", MaxHealth: 10, Health: 10, Attack: 5}
	code, _ := q.Code("goblin")

	ApplyScan(&q, p, code)
	if _, err := RollCombatWithRolls(&q, p, 1, 1); err != nil {
		t.Fatal(err)
	}
	healthAfterFirst := p.MaxHealth
	ApplyScan(&q, p, code)

	if p.MaxHealth != healthAfterFirst {
		t.Fatalf("MaxHealth = %d, want unchanged %d", p.MaxHealth, healthAfterFirst)
	}
	if len(p.Defeated) != 1 {
		t.Fatalf("Defeated length = %d, want 1", len(p.Defeated))
	}
}

func TestDragonReadinessAndVictory(t *testing.T) {
	q := testQuest()
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	p := &Player{
		ID:        "p1",
		MaxHealth: 10,
		Health:    10,
		Attack:    2,
		Armor:     0,
	}
	ready, _ := DragonReady(&q, p)
	if ready {
		t.Fatal("DragonReady() = true, want false")
	}

	for _, id := range []string{"sword", "shield", "friend", "goblin"} {
		code, _ := q.Code(id)
		ApplyScan(&q, p, code)
		if p.Combat != nil {
			if _, err := RollCombatWithRolls(&q, p, 1, 1); err != nil {
				t.Fatal(err)
			}
		}
	}
	ready, missing := DragonReady(&q, p)
	if !ready {
		t.Fatalf("DragonReady() = false, missing %q", missing)
	}

	code, _ := q.Code("dragon")
	result := ApplyScan(&q, p, code)
	if result.Combat == nil {
		t.Fatal("dragon scan did not start combat")
	}
	result, err := RollCombatWithRolls(&q, p, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Victory || !p.DragonDefeated {
		t.Fatalf("dragon victory = %v, defeated = %v; want both true", result.Victory, p.DragonDefeated)
	}
}

func TestCombatDefeatRequiresHealingCode(t *testing.T) {
	q := testQuest()
	q.Codes = append(q.Codes, Code{ID: "potion", Type: CodeHealing, Title: "Potion", Effects: Effects{Health: 5}})
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	p := &Player{ID: "p1", MaxHealth: 10, Health: 3, Attack: 0, Armor: 0}
	enemy, _ := q.Code("goblin")
	ApplyScan(&q, p, enemy)
	result, err := RollCombatWithRolls(&q, p, 1, 6)
	if err != nil {
		t.Fatal(err)
	}
	if p.Health != 0 || !result.Blocked {
		t.Fatalf("health = %d, blocked = %v; want zero health and blocked", p.Health, result.Blocked)
	}

	sword, _ := q.Code("sword")
	result = ApplyScan(&q, p, sword)
	if !result.Blocked {
		t.Fatal("scan while at zero health was not blocked")
	}

	potion, _ := q.Code("potion")
	result = ApplyScan(&q, p, potion)
	if result.Blocked || p.Health != 5 {
		t.Fatalf("healing blocked = %v, health = %d; want unblocked and 5", result.Blocked, p.Health)
	}
}

func TestInventoryDisplaysCurrentTitlesFromStableIDs(t *testing.T) {
	q := testQuest()
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	p := &Player{
		ID:         "p1",
		Items:      []string{"sword"},
		Companions: []string{"Old Friend Name"},
		Scanned:    []string{"friend"},
	}
	q.byID["sword"] = Code{ID: "sword", Type: CodeWeapon, Title: "Renamed Sword"}
	q.byID["friend"] = Code{ID: "friend", Type: CodeCompanion, Title: "Renamed Friend"}
	for i := range q.Codes {
		if q.Codes[i].ID == "sword" {
			q.Codes[i].Title = "Renamed Sword"
		}
		if q.Codes[i].ID == "friend" {
			q.Codes[i].Title = "Renamed Friend"
		}
	}

	gear := p.GearNames(&q)
	if len(gear) != 1 || gear[0] != "Renamed Sword" {
		t.Fatalf("GearNames = %#v, want renamed sword", gear)
	}
	companions := p.CompanionNames(&q)
	if len(companions) != 1 || companions[0] != "Renamed Friend" {
		t.Fatalf("CompanionNames = %#v, want renamed friend", companions)
	}
}
