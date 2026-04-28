package game

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

type ScanResult struct {
	Code        Code
	Player      *Player
	Title       string
	Body        string
	Clue        string
	AlreadySeen bool
	Victory     bool
	Ready       bool
	Blocked     bool
	Defeated    bool
	Combat      *Combat
	LastRound   *CombatRound
}

func ApplyScan(q *Quest, p *Player, code Code) ScanResult {
	result := ScanResult{
		Code:   code,
		Player: p,
		Title:  code.Title,
		Body:   code.Description,
		Clue:   code.Clue,
	}

	if p.Combat != nil {
		if p.Combat.CodeID == code.ID {
			result.Combat = p.Combat
			result.Body = "The fight is still underway. Roll again when you are ready."
			return result
		}
		result.Blocked = true
		result.Title = "Finish the fight"
		result.Body = "You are already in a fight. Finish that enemy before scanning another code."
		return result
	}

	if p.Health <= 0 && code.Type != CodeHealing && code.Type != CodeStart {
		result.Blocked = true
		result.Title = "Find a healer"
		result.Body = "You are out of health. Find a healer code before continuing."
		return result
	}

	if code.Type == CodeStart {
		p.markScanned(code.ID)
		p.addClue(code.ID, code.Clue)
		result.Body = "Your adventure is already underway."
		return result
	}

	if code.Type == CodeDragon {
		return applyDragon(q, p, code, result)
	}

	if p.HasScanned(code.ID) && !(code.Type == CodeHealing && p.Health <= 0) {
		result.AlreadySeen = true
		result.Body = "You have already claimed this part of the quest."
		return result
	}

	switch code.Type {
	case CodeWeapon:
		applyEffects(p, code.Effects)
		p.addItem(code.ID)
		result.Body = fmt.Sprintf("%s Attack +%d.", code.Description, code.Effects.Attack)
	case CodeArmor:
		applyEffects(p, code.Effects)
		p.addItem(code.ID)
		result.Body = fmt.Sprintf("%s Armor +%d.", code.Description, code.Effects.Armor)
	case CodeCompanion:
		applyEffects(p, code.Effects)
		p.addCompanion(code.ID)
		result.Body = fmt.Sprintf("%s %s joins your party.", code.Description, code.Title)
	case CodeHealing:
		before := p.Health
		restoreHealth(p, code.Effects.Health)
		result.Body = fmt.Sprintf("%s Health restored from %d to %d.", code.Description, before, p.Health)
	case CodeClue:
		applyEffects(p, code.Effects)
		result.Body = code.Description
	case CodeEnemy:
		result = startCombat(p, code, result)
		return result
	}

	p.markScanned(code.ID)
	p.addClue(code.ID, code.Clue)
	return result
}

func applyDragon(q *Quest, p *Player, code Code, result ScanResult) ScanResult {
	ready, missing := DragonReady(q, p)
	result.Ready = ready
	if p.DragonDefeated {
		result.Victory = true
		result.Body = q.VictoryText
		return result
	}
	if !ready {
		result.Body = "The dragon's smoke is too thick. Gather more strength first: " + missing
		return result
	}

	return startCombat(p, code, result)
}

func startCombat(p *Player, code Code, result ScanResult) ScanResult {
	p.Combat = &Combat{
		CodeID:      code.ID,
		EnemyHealth: code.Enemy.Health,
		IsDragon:    code.Type == CodeDragon,
	}
	result.Combat = p.Combat
	result.Body = fmt.Sprintf("%s Roll to attack.", code.Description)
	return result
}

var ErrNoActiveCombat = errors.New("no active combat")

func RollCombat(q *Quest, p *Player) (ScanResult, error) {
	return rollCombat(q, p, rand.IntN(6)+1, rand.IntN(6)+1)
}

func RollCombatWithRolls(q *Quest, p *Player, playerRoll int, enemyRoll int) (ScanResult, error) {
	return rollCombat(q, p, playerRoll, enemyRoll)
}

func rollCombat(q *Quest, p *Player, playerRoll int, enemyRoll int) (ScanResult, error) {
	if p.Combat == nil {
		return ScanResult{}, ErrNoActiveCombat
	}
	code, ok := q.Code(p.Combat.CodeID)
	if !ok {
		return ScanResult{}, fmt.Errorf("active combat references missing code %q", p.Combat.CodeID)
	}
	if playerRoll < 1 || playerRoll > 6 || enemyRoll < 1 || enemyRoll > 6 {
		return ScanResult{}, fmt.Errorf("combat rolls must be between 1 and 6")
	}

	combat := p.Combat
	playerDamage := max(1, playerRoll+p.Attack-code.Enemy.Armor)
	combat.EnemyHealth = max(0, combat.EnemyHealth-playerDamage)

	enemyDamage := 0
	if combat.EnemyHealth > 0 {
		enemyDamage = max(1, enemyRoll+code.Enemy.Attack-p.Armor)
		p.Health = max(0, p.Health-enemyDamage)
	}

	round := CombatRound{
		PlayerRoll:   playerRoll,
		EnemyRoll:    enemyRoll,
		PlayerDamage: playerDamage,
		EnemyDamage:  enemyDamage,
		EnemyHealth:  combat.EnemyHealth,
		PlayerHealth: p.Health,
	}
	combat.Rounds = append(combat.Rounds, round)
	result := ScanResult{
		Code:      code,
		Player:    p,
		Title:     code.Title,
		Body:      fmt.Sprintf("You rolled %d and dealt %d damage. %s rolled %d and dealt %d damage.", playerRoll, playerDamage, code.Title, enemyRoll, enemyDamage),
		Clue:      "",
		Ready:     code.Type == CodeDragon,
		Combat:    combat,
		LastRound: &round,
	}

	if combat.EnemyHealth <= 0 {
		p.Combat = nil
		p.markScanned(code.ID)
		p.addClue(code.ID, code.Clue)
		result.Combat = nil
		result.Clue = code.Clue
		result.Defeated = true
		if code.Type == CodeDragon {
			p.DragonDefeated = true
			result.Victory = true
			result.Body = fmt.Sprintf("You defeated %s with %d health remaining. %s", code.Title, p.Health, q.VictoryText)
			return result, nil
		}
		applyEffects(p, code.Rewards)
		p.addDefeated(code.ID)
		result.Body = fmt.Sprintf("You defeated %s. Rewards claimed.", code.Title)
		return result, nil
	}

	if p.Health <= 0 {
		p.Combat = nil
		result.Combat = nil
		result.Blocked = true
		result.Body = fmt.Sprintf("%s reduced your health to zero. Find a healer code before continuing.", code.Title)
		return result, nil
	}

	return result, nil
}

func DragonReady(q *Quest, p *Player) (bool, string) {
	missing := ""
	req := q.DragonRequirements
	if p.Attack < req.MinAttack {
		missing += fmt.Sprintf("attack %d/%d; ", p.Attack, req.MinAttack)
	}
	if p.Armor < req.MinArmor {
		missing += fmt.Sprintf("armor %d/%d; ", p.Armor, req.MinArmor)
	}
	companionCount := p.CompanionCount(q)
	if companionCount < req.MinCompanions {
		missing += fmt.Sprintf("companions %d/%d; ", companionCount, req.MinCompanions)
	}
	for _, id := range req.Defeated {
		if !p.HasDefeated(id) {
			if code, ok := q.Code(id); ok {
				missing += fmt.Sprintf("defeat %s; ", code.Title)
			} else {
				missing += fmt.Sprintf("defeat %s; ", id)
			}
		}
	}
	if missing == "" {
		return true, ""
	}
	return false, missing[:len(missing)-2]
}

func applyEffects(p *Player, effects Effects) {
	p.Attack += effects.Attack
	p.Armor += effects.Armor
	if effects.Health > 0 {
		p.MaxHealth += effects.Health
		p.Health += effects.Health
		if p.Health > p.MaxHealth {
			p.Health = p.MaxHealth
		}
	}
}

func restoreHealth(p *Player, amount int) {
	if amount <= 0 {
		return
	}
	p.Health += amount
	if p.Health > p.MaxHealth {
		p.Health = p.MaxHealth
	}
}
