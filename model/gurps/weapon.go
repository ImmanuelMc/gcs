/*
 * Copyright ©1998-2023 by Richard A. Wilkes. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, version 2.0. If a copy of the MPL was not distributed with
 * this file, You can obtain one at http://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as
 * defined by the Mozilla Public License, version 2.0.
 */

package gurps

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"strings"
	"unsafe"

	"github.com/google/uuid"
	"github.com/richardwilkes/gcs/v5/model/fxp"
	"github.com/richardwilkes/json"
	"github.com/richardwilkes/rpgtools/dice"
	"github.com/richardwilkes/toolbox/errs"
	"github.com/richardwilkes/toolbox/i18n"
	"github.com/richardwilkes/toolbox/txt"
	"github.com/richardwilkes/toolbox/xio"
)

var _ Node[*Weapon] = &Weapon{}

// Columns that can be used with the weapon method .CellData()
const (
	WeaponDescriptionColumn = iota
	WeaponUsageColumn
	WeaponSLColumn
	WeaponParryColumn
	WeaponBlockColumn
	WeaponDamageColumn
	WeaponReachColumn
	WeaponSTColumn
	WeaponAccColumn
	WeaponRangeColumn
	WeaponRoFColumn
	WeaponShotsColumn
	WeaponBulkColumn
	WeaponRecoilColumn
)

// WeaponOwner defines the methods required of a Weapon owner.
type WeaponOwner interface {
	fmt.Stringer
	OwningEntity() *Entity
	Description() string
	Notes() string
	FeatureList() Features
	TagList() []string
	RatedStrength() fxp.Int
}

// WeaponData holds the Weapon data that is written to disk.
type WeaponData struct {
	ID                 uuid.UUID       `json:"id"`
	Type               WeaponType      `json:"type"`
	Bipod              bool            `json:"bipod,omitempty"`
	Mounted            bool            `json:"mounted,omitempty"`
	MusketRest         bool            `json:"musket_rest,omitempty"`
	TwoHanded          bool            `json:"two_handed,omitempty"`
	UnreadyAfterAttack bool            `json:"unready_after_attack,omitempty"`
	Jet                bool            `json:"jet,omitempty"`
	Damage             WeaponDamage    `json:"damage"`
	Usage              string          `json:"usage,omitempty"`
	UsageNotes         string          `json:"usage_notes,omitempty"`
	Reach              string          `json:"reach,omitempty"`
	Parry              string          `json:"parry,omitempty"`
	Block              string          `json:"block,omitempty"`
	Range              string          `json:"range,omitempty"`
	RateOfFire         string          `json:"rate_of_fire,omitempty"`
	Shots              string          `json:"shots,omitempty"`
	Bulk               string          `json:"bulk,omitempty"`
	Recoil             string          `json:"recoil,omitempty"`
	WeaponAcc          fxp.Int         `json:"weapon_acc,omitempty"`
	ScopeAcc           fxp.Int         `json:"scope_acc,omitempty"`
	MinST              fxp.Int         `json:"min_st,omitempty"`
	Defaults           []*SkillDefault `json:"defaults,omitempty"`
}

// Weapon holds the stats for a weapon.
type Weapon struct {
	WeaponData
	Owner WeaponOwner
}

// ExtractWeaponsOfType filters the input list down to only those weapons of the given type.
func ExtractWeaponsOfType(desiredType WeaponType, list []*Weapon) []*Weapon {
	var result []*Weapon
	for _, w := range list {
		if w.Type == desiredType {
			result = append(result, w)
		}
	}
	return result
}

// SeparateWeapons returns separate lists for melee and ranged weapons found in the input list.
func SeparateWeapons(list []*Weapon) (melee, ranged []*Weapon) {
	for _, w := range list {
		switch w.Type {
		case MeleeWeaponType:
			melee = append(melee, w)
		case RangedWeaponType:
			ranged = append(ranged, w)
		default:
		}
	}
	return melee, ranged
}

// NewWeapon creates a new weapon of the given type.
func NewWeapon(owner WeaponOwner, weaponType WeaponType) *Weapon {
	w := &Weapon{
		WeaponData: WeaponData{
			ID:   NewUUID(),
			Type: weaponType,
			Damage: WeaponDamage{
				WeaponDamageData: WeaponDamageData{
					Type:                      "cr",
					ArmorDivisor:              fxp.One,
					FragmentationArmorDivisor: fxp.One,
				},
			},
		},
		Owner: owner,
	}
	switch weaponType {
	case MeleeWeaponType:
		w.Reach = "1"
		w.Damage.StrengthType = ThrustStrengthDamage
	case RangedWeaponType:
		w.RateOfFire = "1"
		w.Damage.Base = dice.New("1d")
	default:
	}
	return w
}

// Clone implements Node.
func (w *Weapon) Clone(_ *Entity, _ *Weapon, preserveID bool) *Weapon {
	other := *w
	if !preserveID {
		other.ID = uuid.New()
	}
	other.Damage = *other.Damage.Clone(&other)
	if other.Defaults != nil {
		other.Defaults = make([]*SkillDefault, 0, len(w.Defaults))
		for _, one := range w.Defaults {
			d := *one
			other.Defaults = append(other.Defaults, &d)
		}
	}
	return &other
}

// Less returns true if this weapon should be sorted above the other weapon.
func (w *Weapon) Less(other *Weapon) bool {
	s1 := w.String()
	s2 := other.String()
	if txt.NaturalLess(s1, s2, true) {
		return true
	}
	if s1 != s2 {
		return false
	}
	if txt.NaturalLess(w.Usage, other.Usage, true) {
		return true
	}
	if w.Usage != other.Usage {
		return false
	}
	if txt.NaturalLess(w.UsageNotes, other.UsageNotes, true) {
		return true
	}
	if w.UsageNotes != other.UsageNotes {
		return false
	}
	return uintptr(unsafe.Pointer(w)) < uintptr(unsafe.Pointer(other)) //nolint:gosec // Just need a tie-breaker
}

// HashCode returns a hash value for this weapon's resolved state.
// nolint:errcheck // Not checking errors on writes to a bytes.Buffer
func (w *Weapon) HashCode() uint32 {
	h := fnv.New32()
	_, _ = h.Write([]byte(w.ID.String()))
	_, _ = h.Write([]byte{byte(w.Type)})
	_, _ = h.Write([]byte(w.String()))
	_, _ = h.Write([]byte(w.UsageNotes))
	_, _ = h.Write([]byte(w.Usage))
	_ = binary.Write(h, binary.LittleEndian, w.SkillLevel(nil))
	_, _ = h.Write([]byte(w.Parry))
	_, _ = h.Write([]byte(w.Block))
	_, _ = h.Write([]byte(w.Damage.ResolvedDamage(nil)))
	_, _ = h.Write([]byte(w.Reach))
	_, _ = h.Write([]byte(w.Range))
	_, _ = h.Write([]byte(w.RateOfFire))
	_, _ = h.Write([]byte(w.Shots))
	_, _ = h.Write([]byte(w.Bulk))
	_, _ = h.Write([]byte(w.Recoil))
	_ = binary.Write(h, binary.LittleEndian, w.Jet)
	_ = binary.Write(h, binary.LittleEndian, w.WeaponAcc)
	_ = binary.Write(h, binary.LittleEndian, w.ScopeAcc)
	_ = binary.Write(h, binary.LittleEndian, w.MinST)
	_ = binary.Write(h, binary.LittleEndian, w.Bipod)
	_ = binary.Write(h, binary.LittleEndian, w.Mounted)
	_ = binary.Write(h, binary.LittleEndian, w.MusketRest)
	_ = binary.Write(h, binary.LittleEndian, w.TwoHanded)
	_ = binary.Write(h, binary.LittleEndian, w.UnreadyAfterAttack)
	return h.Sum32()
}

// MarshalJSON implements json.Marshaler.
func (w *Weapon) MarshalJSON() ([]byte, error) {
	type calc struct {
		Level         fxp.Int `json:"level,omitempty"`
		Parry         string  `json:"parry,omitempty"`
		Block         string  `json:"block,omitempty"`
		Range         string  `json:"range,omitempty"`
		Damage        string  `json:"damage,omitempty"`
		ResolvedMinST fxp.Int `json:"resolved_min_st,omitempty"`
	}
	data := struct {
		WeaponData
		Calc calc `json:"calc"`
	}{
		WeaponData: w.WeaponData,
		Calc: calc{
			Level:         w.SkillLevel(nil).Max(0),
			Damage:        w.Damage.ResolvedDamage(nil),
			ResolvedMinST: w.ResolvedMinimumStrength(nil),
		},
	}
	switch w.Type {
	case MeleeWeaponType:
		data.Calc.Parry = w.ResolvedParry(nil)
		data.Calc.Block = w.ResolvedBlock(nil)
	case RangedWeaponType:
		data.Calc.Range = w.ResolvedRange()
	default:
	}
	return json.Marshal(&data)
}

// UnmarshalJSON implements json.Unmarshaler.
func (w *Weapon) UnmarshalJSON(data []byte) error {
	type oldWeaponData struct {
		WeaponData
		OldAccuracy        string `json:"accuracy,omitempty"`
		OldMinimumStrength string `json:"strength"`
	}
	var wdata oldWeaponData
	if err := json.Unmarshal(data, &wdata); err != nil {
		return err
	}
	w.WeaponData = wdata.WeaponData
	if wdata.OldAccuracy != "" {
		if w.Jet = strings.ToLower(strings.TrimSpace(wdata.OldAccuracy)) == "jet"; !w.Jet {
			parts := strings.Split(strings.TrimPrefix(strings.ReplaceAll(wdata.OldAccuracy, " ", ""), "+"), "+")
			var err error
			if w.WeaponAcc, err = fxp.FromString(parts[0]); err != nil {
				return err
			}
			if len(parts) > 1 {
				if w.ScopeAcc, err = fxp.FromString(parts[1]); err != nil {
					return err
				}
			}
		}
	}
	if wdata.OldMinimumStrength != "" {
		w.Bipod = strings.Contains(wdata.OldMinimumStrength, "B")
		w.Mounted = strings.Contains(wdata.OldMinimumStrength, "M")
		w.MusketRest = strings.Contains(wdata.OldMinimumStrength, "R")
		w.TwoHanded = strings.Contains(wdata.OldMinimumStrength, "†")
		w.UnreadyAfterAttack = strings.Contains(wdata.OldMinimumStrength, "‡")
		if w.UnreadyAfterAttack {
			w.TwoHanded = true
		}
		started := false
		value := 0
		for _, ch := range wdata.OldMinimumStrength {
			if ch >= '0' && ch <= '9' {
				value *= 10
				value += int(ch - '0')
				started = true
			} else if started {
				break
			}
		}
		w.MinST = fxp.From(value)
	}
	var zero uuid.UUID
	if w.WeaponData.ID == zero {
		w.WeaponData.ID = NewUUID()
	}
	return nil
}

// UUID returns the UUID of this data.
func (w *Weapon) UUID() uuid.UUID {
	return w.ID
}

// Kind returns the kind of data.
func (w *Weapon) Kind() string {
	return w.Type.String()
}

func (w *Weapon) String() string {
	if w.Owner == nil {
		return ""
	}
	return w.Owner.Description()
}

// Notes returns the notes for this weapon.
func (w *Weapon) Notes() string {
	var buffer strings.Builder
	if w.Owner != nil {
		buffer.WriteString(w.Owner.Notes())
	}
	if strings.TrimSpace(w.UsageNotes) != "" {
		if buffer.Len() != 0 {
			buffer.WriteByte('\n')
		}
		buffer.WriteString(w.UsageNotes)
	}
	return buffer.String()
}

// SetOwner sets the owner and ensures sub-components have their owners set.
func (w *Weapon) SetOwner(owner WeaponOwner) {
	w.Owner = owner
	w.Damage.Owner = w
}

// Entity returns the owning entity, if any.
func (w *Weapon) Entity() *Entity {
	if w.Owner == nil {
		return nil
	}
	entity := w.Owner.OwningEntity()
	if entity == nil {
		return nil
	}
	return entity
}

// PC returns the owning PC, if any.
func (w *Weapon) PC() *Entity {
	if entity := w.Entity(); entity != nil && entity.Type == PC {
		return entity
	}
	return nil
}

// SkillLevel returns the resolved skill level.
func (w *Weapon) SkillLevel(tooltip *xio.ByteBuffer) fxp.Int {
	pc := w.PC()
	if pc == nil {
		return 0
	}
	var primaryTooltip *xio.ByteBuffer
	if tooltip != nil {
		primaryTooltip = &xio.ByteBuffer{}
	}
	adj := w.skillLevelBaseAdjustment(pc, primaryTooltip) + w.skillLevelPostAdjustment(pc, primaryTooltip)
	best := fxp.Min
	for _, def := range w.Defaults {
		if level := def.SkillLevelFast(pc, false, nil, true); level != fxp.Min {
			level += adj
			if best < level {
				best = level
			}
		}
	}
	if best == fxp.Min {
		return 0
	}
	if tooltip != nil && primaryTooltip != nil && primaryTooltip.Len() != 0 {
		if tooltip.Len() != 0 {
			tooltip.WriteByte('\n')
		}
		tooltip.WriteString(primaryTooltip.String())
	}
	if best < 0 {
		best = 0
	}
	return best
}

func (w *Weapon) skillLevelBaseAdjustment(entity *Entity, tooltip *xio.ByteBuffer) fxp.Int {
	var adj fxp.Int
	if minST := w.ResolvedMinimumStrength(nil) - entity.StrikingStrength(); minST > 0 {
		adj -= minST
		if tooltip != nil {
			tooltip.WriteByte('\n')
			tooltip.WriteString(w.String())
			tooltip.WriteString(" [")
			tooltip.WriteString((-minST).String())
			tooltip.WriteString(i18n.Text(" to skill level due to minimum ST requirement"))
			tooltip.WriteByte(']')
		}
	}
	nameQualifier := w.String()
	for _, bonus := range entity.NamedWeaponSkillBonusesFor(nameQualifier, w.Usage, w.Owner.TagList(), tooltip) {
		adj += bonus.AdjustedAmount()
	}
	for _, f := range w.Owner.FeatureList() {
		adj += w.extractSkillBonusForThisWeapon(f, tooltip)
	}
	if t, ok := w.Owner.(*Trait); ok {
		Traverse(func(mod *TraitModifier) bool {
			for _, f := range mod.Features {
				adj += w.extractSkillBonusForThisWeapon(f, tooltip)
			}
			return false
		}, true, true, t.Modifiers...)
	}
	if eqp, ok := w.Owner.(*Equipment); ok {
		Traverse(func(mod *EquipmentModifier) bool {
			for _, f := range mod.Features {
				adj += w.extractSkillBonusForThisWeapon(f, tooltip)
			}
			return false
		}, true, true, eqp.Modifiers...)
	}
	return adj
}

func (w *Weapon) skillLevelPostAdjustment(entity *Entity, tooltip *xio.ByteBuffer) fxp.Int {
	if w.Type.EnsureValid() == MeleeWeaponType && strings.Contains(w.Parry, "F") {
		return w.EncumbrancePenalty(entity, tooltip)
	}
	return 0
}

// EncumbrancePenalty returns the current encumbrance penalty.
func (w *Weapon) EncumbrancePenalty(entity *Entity, tooltip *xio.ByteBuffer) fxp.Int {
	if entity == nil {
		return 0
	}
	penalty := entity.EncumbranceLevel(true).Penalty()
	if penalty != 0 && tooltip != nil {
		tooltip.WriteByte('\n')
		tooltip.WriteString(i18n.Text("Encumbrance"))
		tooltip.WriteString(" [")
		tooltip.WriteString(penalty.StringWithSign())
		tooltip.WriteByte(']')
	}
	return penalty
}

func (w *Weapon) extractSkillBonusForThisWeapon(f Feature, tooltip *xio.ByteBuffer) fxp.Int {
	if sb, ok := f.(*SkillBonus); ok {
		if sb.SelectionType.EnsureValid() == ThisWeaponSkillSelectionType {
			if sb.SpecializationCriteria.Matches(w.Usage) {
				sb.AddToTooltip(tooltip)
				return sb.AdjustedAmount()
			}
		}
	}
	return 0
}

// ResolvedParry returns the resolved parry level.
func (w *Weapon) ResolvedParry(tooltip *xio.ByteBuffer) string {
	return w.resolvedValue(w.Parry, ParryID, tooltip)
}

// ResolvedBlock returns the resolved block level.
func (w *Weapon) ResolvedBlock(tooltip *xio.ByteBuffer) string {
	return w.resolvedValue(w.Block, BlockID, tooltip)
}

// ResolvedRange returns the range, fully resolved for the user's ST, if possible.
func (w *Weapon) ResolvedRange() string {
	var st fxp.Int
	if w.Owner != nil {
		st = w.Owner.RatedStrength()
	}
	if st == 0 {
		if pc := w.PC(); pc != nil {
			st = pc.ThrowingStrength()
		}
	}
	if st == 0 {
		return w.Range
	}
	var savedRange string
	calcRange := w.Range
	for calcRange != savedRange {
		savedRange = calcRange
		calcRange = w.resolveRange(calcRange, st)
	}
	return calcRange
}

func (w *Weapon) resolvedValue(input, baseDefaultType string, tooltip *xio.ByteBuffer) string {
	pc := w.PC()
	if pc == nil {
		return input
	}
	var buffer strings.Builder
	skillLevel := fxp.Max
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := scanner.Text()
		if buffer.Len() != 0 {
			buffer.WriteByte('\n')
		}
		if line != "" {
			maximum := len(line)
			i := 0
			for i < maximum && line[i] == ' ' {
				i++
			}
			if i < maximum {
				ch := line[i]
				neg := false
				modifier := 0
				found := false
				if ch == '-' || ch == '+' {
					neg = ch == '-'
					i++
					if i < maximum {
						ch = line[i]
					}
				}
				for i < maximum && ch >= '0' && ch <= '9' {
					found = true
					modifier *= 10
					modifier += int(ch - '0')
					i++
					if i < maximum {
						ch = line[i]
					}
				}
				if found {
					if skillLevel == fxp.Max {
						var primaryTooltip, secondaryTooltip *xio.ByteBuffer
						if tooltip != nil {
							primaryTooltip = &xio.ByteBuffer{}
						}
						preAdj := w.skillLevelBaseAdjustment(pc, primaryTooltip)
						postAdj := w.skillLevelPostAdjustment(pc, primaryTooltip)
						adj := fxp.Three
						if baseDefaultType == ParryID {
							adj += pc.ParryBonus
						} else {
							adj += pc.BlockBonus
						}
						best := fxp.Min
						for _, def := range w.Defaults {
							level := def.SkillLevelFast(pc, false, nil, true)
							if level == fxp.Min {
								continue
							}
							level += preAdj
							if baseDefaultType != def.Type() {
								level = (level.Div(fxp.Two) + adj).Trunc()
							}
							level += postAdj
							var possibleTooltip *xio.ByteBuffer
							if def.Type() == SkillID && def.Name == "Karate" {
								if tooltip != nil {
									possibleTooltip = &xio.ByteBuffer{}
								}
								level += w.EncumbrancePenalty(pc, possibleTooltip)
							}
							if best < level {
								best = level
								secondaryTooltip = possibleTooltip
							}
						}
						if best != fxp.Min && tooltip != nil {
							if primaryTooltip != nil && primaryTooltip.Len() != 0 {
								if tooltip.Len() != 0 {
									tooltip.WriteByte('\n')
								}
								tooltip.WriteString(primaryTooltip.String())
							}
							if secondaryTooltip != nil && secondaryTooltip.Len() != 0 {
								if tooltip.Len() != 0 {
									tooltip.WriteByte('\n')
								}
								tooltip.WriteString(secondaryTooltip.String())
							}
						}
						skillLevel = best.Max(0)
					}
					if neg {
						modifier = -modifier
					}
					num := (skillLevel + fxp.From(modifier)).Trunc().String()
					if i < maximum {
						buffer.WriteString(num)
						line = line[i:]
					} else {
						line = num
					}
				}
			}
		}
		buffer.WriteString(line)
	}
	return buffer.String()
}

func (w *Weapon) resolveRange(inRange string, st fxp.Int) string {
	where := strings.IndexByte(inRange, 'x')
	if where == -1 {
		return inRange
	}
	last := where + 1
	maximum := len(inRange)
	if last < maximum && inRange[last] == ' ' {
		last++
	}
	if last >= maximum {
		return inRange
	}
	ch := inRange[last]
	found := false
	decimal := false
	started := last
	for (!decimal && ch == '.') || (ch >= '0' && ch <= '9') {
		found = true
		if ch == '.' {
			decimal = true
		}
		last++
		if last >= maximum {
			break
		}
		ch = inRange[last]
	}
	if !found {
		return inRange
	}
	value, err := fxp.FromString(inRange[started:last])
	if err != nil {
		return inRange
	}
	var buffer strings.Builder
	if where > 0 {
		buffer.WriteString(inRange[:where])
	}
	buffer.WriteString(value.Mul(st).Trunc().String())
	if last < maximum {
		buffer.WriteString(inRange[last:])
	}
	return buffer.String()
}

// ResolvedAccuracy returns the resolved weapon and scope accuracies for this weapon.
func (w *Weapon) ResolvedAccuracy(tooltip *xio.ByteBuffer) (weapon, scope fxp.Int) {
	if w.Jet {
		return 0, 0
	}
	pc := w.PC()
	if pc == nil {
		return w.WeaponAcc, w.ScopeAcc
	}
	weaponAcc := w.WeaponAcc
	scopeAcc := w.ScopeAcc
	for _, bonus := range w.collectWeaponBonuses(1, tooltip, WeaponAccBonusFeatureType, WeaponScopeAccBonusFeatureType) {
		switch bonus.Type {
		case WeaponAccBonusFeatureType:
			weaponAcc += bonus.AdjustedAmount()
		case WeaponScopeAccBonusFeatureType:
			scopeAcc += bonus.AdjustedAmount()
		default:
		}
	}
	return weaponAcc.Max(0), scopeAcc.Max(0)
}

// ResolvedMinimumStrength returns the resolved minimum strength required to use this weapon, or 0 if there is none.
func (w *Weapon) ResolvedMinimumStrength(tooltip *xio.ByteBuffer) fxp.Int {
	if w.Owner != nil {
		if st := w.Owner.RatedStrength().Max(0); st != 0 {
			return st
		}
	}
	minST := w.MinST
	for _, bonus := range w.collectWeaponBonuses(1, tooltip, WeaponMinSTBonusFeatureType) {
		minST += bonus.AdjustedAmount()
	}
	return minST.Max(0)
}

func (w *Weapon) collectWeaponBonuses(dieCount int, tooltip *xio.ByteBuffer, allowedFeatureTypes ...FeatureType) []*WeaponBonus {
	pc := w.PC()
	if pc == nil {
		return nil
	}
	allowed := make(map[FeatureType]bool, len(allowedFeatureTypes))
	for _, one := range allowedFeatureTypes {
		allowed[one] = true
	}
	var bestDef *SkillDefault
	best := fxp.Min
	for _, one := range w.Defaults {
		if one.SkillBased() {
			if level := one.SkillLevelFast(pc, false, nil, true); best < level {
				best = level
				bestDef = one
			}
		}
	}
	bonusSet := make(map[*WeaponBonus]bool)
	tags := w.Owner.TagList()
	if bestDef != nil {
		pc.AddWeaponWithSkillBonusesFor(bestDef.Name, bestDef.Specialization, tags, dieCount, tooltip, bonusSet, allowed)
	}
	nameQualifier := w.String()
	pc.AddNamedWeaponBonusesFor(nameQualifier, w.Usage, tags, dieCount, tooltip, bonusSet, allowed)
	for _, f := range w.Owner.FeatureList() {
		w.extractWeaponBonus(f, bonusSet, allowed, fxp.From(dieCount), tooltip)
	}
	if t, ok := w.Owner.(*Trait); ok {
		Traverse(func(mod *TraitModifier) bool {
			var bonus Bonus
			for _, f := range mod.Features {
				if bonus, ok = f.(Bonus); ok {
					bonus.SetSubOwner(mod)
				}
				w.extractWeaponBonus(f, bonusSet, allowed, fxp.From(dieCount), tooltip)
			}
			return false
		}, true, true, t.Modifiers...)
	}
	if eqp, ok := w.Owner.(*Equipment); ok {
		Traverse(func(mod *EquipmentModifier) bool {
			var bonus Bonus
			for _, f := range mod.Features {
				if bonus, ok = f.(Bonus); ok {
					bonus.SetSubOwner(mod)
				}
				w.extractWeaponBonus(f, bonusSet, allowed, fxp.From(dieCount), tooltip)
			}
			return false
		}, true, true, eqp.Modifiers...)
	}
	if len(bonusSet) == 0 {
		return nil
	}
	result := make([]*WeaponBonus, 0, len(bonusSet))
	for bonus := range bonusSet {
		result = append(result, bonus)
	}
	return result
}

func (w *Weapon) extractWeaponBonus(f Feature, set map[*WeaponBonus]bool, allowedFeatureTypes map[FeatureType]bool, dieCount fxp.Int, tooltip *xio.ByteBuffer) {
	if allowedFeatureTypes[f.FeatureType()] {
		if bonus, ok := f.(*WeaponBonus); ok {
			level := bonus.LeveledAmount.Level
			if bonus.Type == WeaponBonusFeatureType {
				bonus.LeveledAmount.Level = dieCount
			} else {
				bonus.LeveledAmount.Level = bonus.DerivedLevel()
			}
			switch bonus.SelectionType {
			case WithRequiredSkillWeaponSelectionType:
			case ThisWeaponWeaponSelectionType:
				if bonus.SpecializationCriteria.Matches(w.Usage) {
					if _, exists := set[bonus]; !exists {
						set[bonus] = true
						bonus.AddToTooltip(tooltip)
					}
				}
			case WithNameWeaponSelectionType:
				if bonus.NameCriteria.Matches(w.String()) && bonus.SpecializationCriteria.Matches(w.Usage) &&
					bonus.TagsCriteria.MatchesList(w.Owner.TagList()...) {
					if _, exists := set[bonus]; !exists {
						set[bonus] = true
						bonus.AddToTooltip(tooltip)
					}
				}
			default:
				errs.Log(errs.New("unknown selection type"), "type", int(bonus.SelectionType))
			}
			bonus.LeveledAmount.Level = level
		}
	}
}

// FillWithNameableKeys adds any nameable keys found in this Weapon to the provided map.
func (w *Weapon) FillWithNameableKeys(m map[string]string) {
	for _, one := range w.Defaults {
		one.FillWithNameableKeys(m)
	}
}

// ApplyNameableKeys replaces any nameable keys found in this Weapon with the corresponding values in the provided map.
func (w *Weapon) ApplyNameableKeys(m map[string]string) {
	for _, one := range w.Defaults {
		one.ApplyNameableKeys(m)
	}
}

// Container returns true if this is a container.
func (w *Weapon) Container() bool {
	return false
}

// Open returns true if this node is currently open.
func (w *Weapon) Open() bool {
	return false
}

// SetOpen sets the current open state for this node.
func (w *Weapon) SetOpen(_ bool) {
}

// Enabled returns true if this node is enabled.
func (w *Weapon) Enabled() bool {
	return true
}

// Parent returns the parent.
func (w *Weapon) Parent() *Weapon {
	return nil
}

// SetParent sets the parent.
func (w *Weapon) SetParent(_ *Weapon) {
}

// HasChildren returns true if this node has children.
func (w *Weapon) HasChildren() bool {
	return false
}

// NodeChildren returns the children of this node, if any.
func (w *Weapon) NodeChildren() []*Weapon {
	return nil
}

// SetChildren sets the children of this node.
func (w *Weapon) SetChildren(_ []*Weapon) {
}

// CellData returns the cell data information for the given column.
func (w *Weapon) CellData(columnID int, data *CellData) {
	var buffer xio.ByteBuffer
	data.Type = TextCellType
	switch columnID {
	case WeaponDescriptionColumn:
		data.Primary = w.String()
		data.Secondary = w.Notes()
	case WeaponUsageColumn:
		data.Primary = w.Usage
	case WeaponSLColumn:
		data.Primary = w.SkillLevel(&buffer).String()
	case WeaponParryColumn:
		data.Primary = w.ResolvedParry(&buffer)
	case WeaponBlockColumn:
		data.Primary = w.ResolvedBlock(&buffer)
	case WeaponDamageColumn:
		data.Primary = w.Damage.ResolvedDamage(&buffer)
	case WeaponReachColumn:
		data.Primary = w.Reach
	case WeaponSTColumn:
		data.Primary = w.CombinedMinST()
		var tooltip strings.Builder
		if st := w.Owner.RatedStrength(); st > 0 {
			fmt.Fprintf(&tooltip, i18n.Text("The weapon has a rated ST of %v, which is used instead of the user's ST for calculations."), st)
		}
		minST := w.ResolvedMinimumStrength(&buffer)
		if minST > 0 {
			if tooltip.Len() != 0 {
				tooltip.WriteString("\n\n")
			}
			fmt.Fprintf(&tooltip, i18n.Text("The weapon has a minimum ST of %v. If your ST is less than this, you will suffer a -1 to weapon skill per point of ST you lack and lose one extra FP at the end of any fight that lasts long enough to cost FP."), minST)
		}
		if w.Bipod {
			if tooltip.Len() != 0 {
				tooltip.WriteString("\n\n")
			}
			tooltip.WriteString(i18n.Text("Has an attached bipod. When used from a prone position, "))
			reducedST := minST.Mul(fxp.Two).Div(fxp.Three).Ceil()
			if reducedST > 0 && reducedST != minST {
				fmt.Fprintf(&tooltip, i18n.Text("reduces the ST requirement to %v and"), reducedST)
			}
			tooltip.WriteString(i18n.Text("treats the attack as braced (add +1 to Accuracy)."))
		}
		if w.Mounted {
			if tooltip.Len() != 0 {
				tooltip.WriteString("\n\n")
			}
			tooltip.WriteString(i18n.Text("Mounted. Ignore listed ST and Bulk when firing from its mount. Takes at least 3 Ready maneuvers to unmount or remount the weapon."))
		}
		if w.MusketRest {
			if tooltip.Len() != 0 {
				tooltip.WriteString("\n\n")
			}
			tooltip.WriteString(i18n.Text("Uses a Musket Rest. Any aimed shot fired while stationary and standing up is automatically braced (add +1 to Accuracy)."))
		}
		if w.TwoHanded || w.UnreadyAfterAttack {
			if tooltip.Len() != 0 {
				tooltip.WriteString("\n\n")
			}
			if w.UnreadyAfterAttack {
				fmt.Fprintf(&tooltip, i18n.Text("Requires two hands and becomes unready after you attack with it. If you have at least ST %v, you can used it two-handed without it becoming unready. If you have at least ST %v, you can use it one-handed with no readiness penalty."), minST.Mul(fxp.OneAndAHalf).Ceil(), minST.Mul(fxp.Three).Ceil())
			} else {
				fmt.Fprintf(&tooltip, i18n.Text("Requires two hands. If you have at least ST %v, you can use it one-handed, but it becomes unready after you attack with it. If you have at least ST %v, you can use it one-handed with no readiness penalty."), minST.Mul(fxp.OneAndAHalf).Ceil(), minST.Mul(fxp.Two).Ceil())
			}
		}
		data.Tooltip = tooltip.String()
	case WeaponAccColumn:
		data.Primary = w.CombinedAcc(&buffer)
	case WeaponRangeColumn:
		data.Primary = w.ResolvedRange()
	case WeaponRoFColumn:
		data.Primary = w.RateOfFire
	case WeaponShotsColumn:
		data.Primary = w.Shots
	case WeaponBulkColumn:
		data.Primary = w.Bulk
	case WeaponRecoilColumn:
		data.Primary = w.Recoil
	case PageRefCellAlias:
		data.Type = PageRefCellType
	}
	if buffer.Len() > 0 {
		if data.Tooltip != "" {
			data.Tooltip += "\n\n"
		}
		data.Tooltip = i18n.Text("Includes modifiers from:") + buffer.String()
	}
}

// CombinedAcc returns the combined string used in the GURPS weapon tables for accuracy.
func (w *Weapon) CombinedAcc(tooltip *xio.ByteBuffer) string {
	if w.Type != RangedWeaponType {
		return ""
	}
	if w.Jet {
		return i18n.Text("Jet")
	}
	weaponAcc, scopeAcc := w.ResolvedAccuracy(tooltip)
	if scopeAcc != 0 {
		return weaponAcc.String() + scopeAcc.StringWithSign()
	}
	return weaponAcc.String()
}

// CombinedMinST returns the combined string used in the GURPS weapon tables for minimum ST.
func (w *Weapon) CombinedMinST() string {
	var buffer strings.Builder
	if minST := w.ResolvedMinimumStrength(nil); minST > 0 {
		buffer.WriteString(minST.String())
	}
	if w.Bipod {
		buffer.WriteByte('B')
	}
	if w.Mounted {
		buffer.WriteByte('M')
	}
	if w.MusketRest {
		buffer.WriteByte('R')
	}
	if w.TwoHanded || w.UnreadyAfterAttack {
		if w.UnreadyAfterAttack {
			buffer.WriteRune('‡')
		} else {
			buffer.WriteRune('†')
		}
	}
	return buffer.String()
}

// OwningEntity returns the owning Entity.
func (w *Weapon) OwningEntity() *Entity {
	return w.Entity()
}

// SetOwningEntity sets the owning entity and configures any sub-components as needed.
func (w *Weapon) SetOwningEntity(_ *Entity) {
}

// CopyFrom implements node.EditorData.
func (w *Weapon) CopyFrom(t *Weapon) {
	*w = *t.Clone(t.Entity(), nil, true)
}

// ApplyTo implements node.EditorData.
func (w *Weapon) ApplyTo(t *Weapon) {
	*t = *w.Clone(t.Entity(), nil, true)
}
