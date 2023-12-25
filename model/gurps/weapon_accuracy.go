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
	"encoding/binary"
	"hash"
	"strings"

	"github.com/richardwilkes/gcs/v5/model/fxp"
	"github.com/richardwilkes/toolbox/i18n"
	"github.com/richardwilkes/toolbox/xio"
)

// WeaponAccuracy holds the accuracy data for a weapon.
type WeaponAccuracy struct {
	Base  fxp.Int
	Scope fxp.Int
}

// ParseWeaponAccuracy parses a string into a WeaponAccuracy.
func ParseWeaponAccuracy(s string) WeaponAccuracy {
	var wa WeaponAccuracy
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "+")
	parts := strings.Split(s, "+")
	wa.Base, _ = fxp.Extract(parts[0])
	if len(parts) > 1 {
		wa.Scope, _ = fxp.Extract(parts[1])
	}
	wa.Validate()
	return wa
}

// nolint:errcheck // Not checking errors on writes to a bytes.Buffer
func (wa WeaponAccuracy) hash(h hash.Hash32) {
	_ = binary.Write(h, binary.LittleEndian, wa.Base)
	_ = binary.Write(h, binary.LittleEndian, wa.Scope)
}

// Resolve any bonuses that apply.
func (wa WeaponAccuracy) Resolve(w *Weapon, modifiersTooltip *xio.ByteBuffer) WeaponAccuracy {
	if w.ResolveBoolFlag(JetWeaponSwitchType, w.Jet) {
		return WeaponAccuracy{}
	}
	result := wa
	if pc := w.PC(); pc != nil {
		for _, bonus := range w.collectWeaponBonuses(1, modifiersTooltip, WeaponAccBonusFeatureType, WeaponScopeAccBonusFeatureType) {
			switch bonus.Type {
			case WeaponAccBonusFeatureType:
				result.Base += bonus.AdjustedAmount()
			case WeaponScopeAccBonusFeatureType:
				result.Scope += bonus.AdjustedAmount()
			default:
			}
		}
	}
	result.Validate()
	return result
}

// String returns a string suitable for presentation, matching the standard GURPS weapon table entry format for this
// data. Call .Resolve() prior to calling this method if you want the resolved values.
func (wa WeaponAccuracy) String(w *Weapon) string {
	if w.ResolveBoolFlag(JetWeaponSwitchType, w.Jet) {
		return i18n.Text("Jet")
	}
	if wa.Scope != 0 {
		return wa.Base.String() + wa.Scope.StringWithSign()
	}
	return wa.Base.String()
}

// Validate ensures that the data is valid.
func (wa *WeaponAccuracy) Validate() {
	wa.Base = wa.Base.Max(0)
	wa.Scope = wa.Scope.Max(0)
}
