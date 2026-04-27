package game

import "testing"

func TestQuestValidateAcceptsSampleShape(t *testing.T) {
	q := testQuest()
	if err := q.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := q.Start().ID; got != "start" {
		t.Fatalf("Start().ID = %q, want start", got)
	}
	if got := q.Dragon().ID; got != "dragon" {
		t.Fatalf("Dragon().ID = %q, want dragon", got)
	}
}

func TestQuestValidateRejectsDuplicateCodeIDs(t *testing.T) {
	q := testQuest()
	q.Codes = append(q.Codes, Code{ID: "sword", Type: CodeWeapon, Title: "Second Sword"})
	if err := q.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate id error")
	}
}

func TestQuestValidateRejectsMissingDragonRequirement(t *testing.T) {
	q := testQuest()
	q.DragonRequirements.Defeated = []string{"missing"}
	if err := q.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing requirement error")
	}
}

func testQuest() Quest {
	return Quest{
		Title:      "Test Quest",
		Intro:      "Intro",
		StartCode:  "start",
		DragonCode: "dragon",
		BaseHealth: 10,
		BaseAttack: 2,
		BaseArmor:  0,
		DragonRequirements: DragonRequirements{
			MinAttack:     5,
			MinArmor:      1,
			MinCompanions: 1,
			Defeated:      []string{"goblin"},
		},
		VictoryText: "Victory",
		Codes: []Code{
			{ID: "start", Type: CodeStart, Title: "Start", Clue: "Find a sword."},
			{ID: "sword", Type: CodeWeapon, Title: "Sword", Effects: Effects{Attack: 3}},
			{ID: "shield", Type: CodeArmor, Title: "Shield", Effects: Effects{Armor: 1}},
			{ID: "friend", Type: CodeCompanion, Title: "Friend", Effects: Effects{Attack: 1}},
			{ID: "goblin", Type: CodeEnemy, Title: "Goblin", Enemy: Enemy{Health: 3, Attack: 1}, Rewards: Effects{Health: 1}},
			{ID: "dragon", Type: CodeDragon, Title: "Dragon", Enemy: Enemy{Health: 5, Attack: 1}},
		},
	}
}
