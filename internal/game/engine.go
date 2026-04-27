package game

import "fmt"

type ScanResult struct {
	Code        Code
	Player      *Player
	Title       string
	Body        string
	Clue        string
	AlreadySeen bool
	Victory     bool
	Ready       bool
}

func ApplyScan(q *Quest, p *Player, code Code) ScanResult {
	result := ScanResult{
		Code:   code,
		Player: p,
		Title:  code.Title,
		Body:   code.Description,
		Clue:   code.Clue,
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

	if p.HasScanned(code.ID) {
		result.AlreadySeen = true
		result.Body = "You have already claimed this part of the quest."
		return result
	}

	switch code.Type {
	case CodeWeapon:
		applyEffects(p, code.Effects)
		p.addItem(code.Title)
		result.Body = fmt.Sprintf("%s Attack +%d.", code.Description, code.Effects.Attack)
	case CodeArmor:
		applyEffects(p, code.Effects)
		p.addItem(code.Title)
		result.Body = fmt.Sprintf("%s Armor +%d.", code.Description, code.Effects.Armor)
	case CodeCompanion:
		applyEffects(p, code.Effects)
		p.addCompanion(code.Title)
		result.Body = fmt.Sprintf("%s %s joins your party.", code.Description, code.Title)
	case CodeHealing:
		before := p.Health
		applyEffects(p, code.Effects)
		result.Body = fmt.Sprintf("%s Health restored from %d to %d.", code.Description, before, p.Health)
	case CodeClue:
		applyEffects(p, code.Effects)
		result.Body = code.Description
	case CodeEnemy:
		victory, damage := fight(p, code.Enemy)
		if victory {
			applyEffects(p, code.Rewards)
			p.addDefeated(code.ID)
			result.Body = fmt.Sprintf("You defeated %s and took %d damage.", code.Title, damage)
		} else {
			result.Body = fmt.Sprintf("%s was too strong this time. You escaped with 1 health. Find more gear or healing, then come back.", code.Title)
		}
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
		p.markScanned(code.ID)
		p.addClue(code.ID, code.Clue)
		return result
	}

	victory, damage := fight(p, code.Enemy)
	p.markScanned(code.ID)
	p.addClue(code.ID, code.Clue)
	if victory {
		p.DragonDefeated = true
		result.Victory = true
		result.Body = fmt.Sprintf("You survived the final battle with %d health after taking %d damage. %s", p.Health, damage, q.VictoryText)
		return result
	}
	result.Body = "The dragon knocks you back, but you escape with 1 health. Seek any missed clues, friends, or healing before trying again."
	return result
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
	if len(p.Companions) < req.MinCompanions {
		missing += fmt.Sprintf("companions %d/%d; ", len(p.Companions), req.MinCompanions)
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

func fight(p *Player, enemy Enemy) (bool, int) {
	playerDamage := max(1, p.Attack-enemy.Armor)
	enemyDamage := max(1, enemy.Attack-p.Armor)
	roundsToWin := ceilDiv(enemy.Health, playerDamage)
	roundsToLose := ceilDiv(p.Health, enemyDamage)
	if roundsToWin <= roundsToLose {
		damage := enemyDamage * (roundsToWin - 1)
		p.Health = max(1, p.Health-damage)
		return true, damage
	}
	damage := enemyDamage * roundsToLose
	p.Health = 1
	return false, damage
}

func ceilDiv(a int, b int) int {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}
