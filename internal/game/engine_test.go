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
	}
	ready, missing := DragonReady(&q, p)
	if !ready {
		t.Fatalf("DragonReady() = false, missing %q", missing)
	}

	code, _ := q.Code("dragon")
	result := ApplyScan(&q, p, code)
	if !result.Victory || !p.DragonDefeated {
		t.Fatalf("dragon victory = %v, defeated = %v; want both true", result.Victory, p.DragonDefeated)
	}
}
