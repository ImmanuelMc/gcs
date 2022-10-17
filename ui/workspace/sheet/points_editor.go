/*
 * Copyright ©1998-2022 by Richard A. Wilkes. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, version 2.0. If a copy of the MPL was not distributed with
 * this file, You can obtain one at http://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as
 * defined by the Mozilla Public License, version 2.0.
 */

package sheet

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/richardwilkes/gcs/v5/model/fxp"
	"github.com/richardwilkes/gcs/v5/model/gurps"
	"github.com/richardwilkes/gcs/v5/model/jio"
	"github.com/richardwilkes/gcs/v5/res"
	"github.com/richardwilkes/gcs/v5/ui/widget"
	"github.com/richardwilkes/gcs/v5/ui/workspace"
	"github.com/richardwilkes/gcs/v5/ui/workspace/editors"
	"github.com/richardwilkes/toolbox"
	"github.com/richardwilkes/toolbox/i18n"
	"github.com/richardwilkes/toolbox/log/jot"
	"github.com/richardwilkes/unison"
	"golang.org/x/exp/slices"
)

var (
	_ unison.Dockable            = &pointsEditor{}
	_ unison.TabCloser           = &pointsEditor{}
	_ widget.ModifiableRoot      = &pointsEditor{}
	_ unison.UndoManagerProvider = &pointsEditor{}
	_ widget.GroupedCloser       = &pointsEditor{}
	_ widget.Rebuildable         = &pointsEditor{}
)

type pointsEditor struct {
	unison.Panel
	owner            widget.Rebuildable
	entity           *gurps.Entity
	previousDockable unison.Dockable
	previousFocusKey string
	undoMgr          *unison.UndoManager
	applyButton      *unison.Button
	cancelButton     *unison.Button
	content          *unison.Panel
	before           []*gurps.PointsRecord
	current          []*gurps.PointsRecord
	promptForSave    bool
}

func displayPointsEditor(owner widget.Rebuildable, entity *gurps.Entity) {
	ws, dc, found := workspace.Activate(func(d unison.Dockable) bool {
		if e, ok := d.(*pointsEditor); ok {
			return e.owner == owner && entity == e.entity
		}
		return false
	})
	if found || ws == nil {
		return
	}
	e := &pointsEditor{
		owner:   owner,
		entity:  entity,
		before:  gurps.ClonePointsRecordList(entity.PointsRecord),
		current: gurps.ClonePointsRecordList(entity.PointsRecord),
	}
	e.Self = e
	sort.Slice(e.current, func(i, j int) bool { return e.current[i].When.After(e.current[j].When) })

	if dc != nil {
		if e.previousDockable = dc.CurrentDockable(); !toolbox.IsNil(e.previousDockable) {
			if focus := e.previousDockable.AsPanel().Window().Focus(); focus != nil {
				if unison.Ancestor[unison.Dockable](focus) == e.previousDockable {
					e.previousFocusKey = focus.RefKey
				}
			}
		}
	}

	e.undoMgr = unison.NewUndoManager(100, func(err error) { jot.Error(err) })
	e.SetLayout(&unison.FlexLayout{Columns: 1})
	e.AddChild(e.createToolbar())
	e.content = unison.NewPanel()
	e.content.SetBorder(unison.NewEmptyBorder(unison.NewUniformInsets(unison.StdHSpacing * 2)))
	e.content.SetLayout(&unison.FlexLayout{
		Columns:  4,
		HSpacing: unison.StdHSpacing,
		VSpacing: unison.StdVSpacing,
	})
	e.content.KeyDownCallback = func(keyCode unison.KeyCode, mod unison.Modifiers, repeat bool) bool {
		switch {
		case mod.OSMenuCmdModifierDown() && (keyCode == unison.KeyReturn || keyCode == unison.KeyNumPadEnter):
			if e.applyButton.Enabled() {
				e.applyButton.Click()
			}
			return true
		case mod == 0 && keyCode == unison.KeyEscape:
			if e.cancelButton.Enabled() {
				e.cancelButton.Click()
			}
			return true
		default:
			return false
		}
	}
	e.initContent()
	scroller := unison.NewScrollPanel()
	scroller.SetContent(e.content, unison.HintedFillBehavior, unison.FillBehavior)
	scroller.SetLayoutData(&unison.FlexLayoutData{
		HAlign: unison.FillAlignment,
		VAlign: unison.FillAlignment,
		HGrab:  true,
		VGrab:  true,
	})
	e.AddChild(scroller)
	e.ClientData()[workspace.AssociatedUUIDKey] = e.entity.ID
	e.promptForSave = true
	scroller.Content().AsPanel().ValidateScrollRoot()
	group := editors.EditorGroup
	if dc != nil && dc.Group == group {
		dc.Stack(e, -1)
	} else if dc = ws.DocumentDock.ContainerForGroup(group); dc != nil {
		dc.Stack(e, -1)
	} else {
		var targetLayoutNode unison.DockLayoutNode
		ws.DocumentDock.DockTo(e, targetLayoutNode, unison.RightSide)
		if dc = unison.Ancestor[*unison.DockContainer](e); dc != nil && dc.Group == "" {
			dc.Group = group
		}
	}
	if children := e.content.Children(); len(children) != 0 {
		children[3].RequestFocus()
	}
}

func (e *pointsEditor) createToolbar() unison.Paneler {
	toolbar := unison.NewPanel()
	toolbar.SetLayoutData(&unison.FlexLayoutData{
		HAlign: unison.FillAlignment,
		HGrab:  true,
	})
	toolbar.SetBorder(unison.NewCompoundBorder(unison.NewLineBorder(unison.DividerColor, 0, unison.Insets{Bottom: 1},
		false), unison.NewEmptyBorder(unison.StdInsets())))

	e.applyButton = unison.NewSVGButton(res.CheckmarkSVG)
	e.applyButton.Tooltip = unison.NewTooltipWithSecondaryText(i18n.Text("Apply Changes"),
		fmt.Sprintf(i18n.Text("%v%v or %v%v"), unison.OSMenuCmdModifier(), unison.KeyReturn, unison.OSMenuCmdModifier(),
			unison.KeyNumPadEnter))
	e.applyButton.SetEnabled(false)
	e.applyButton.ClickCallback = func() {
		e.apply()
		e.promptForSave = false
		e.AttemptClose()
	}
	toolbar.AddChild(e.applyButton)

	e.cancelButton = unison.NewSVGButton(res.NotSVG)
	e.cancelButton.Tooltip = unison.NewTooltipWithSecondaryText(i18n.Text("Discard Changes"), unison.KeyEscape.String())
	e.cancelButton.SetEnabled(false)
	e.cancelButton.ClickCallback = func() {
		e.promptForSave = false
		e.AttemptClose()
	}
	toolbar.AddChild(e.cancelButton)

	toolbar.AddChild(widget.NewToolbarSeparator(unison.StdHSpacing))

	addButton := unison.NewSVGButton(res.CircledAddSVG)
	addButton.Tooltip = unison.NewTooltipWithText(i18n.Text("Add Entry"))
	addButton.ClickCallback = e.addEntry
	toolbar.AddChild(addButton)

	toolbar.SetLayout(&unison.FlexLayout{
		Columns:  len(toolbar.Children()),
		HSpacing: unison.StdHSpacing,
	})
	return toolbar
}

func (e *pointsEditor) initContent() {
	for _, rec := range e.current {
		e.createRow(rec, -1)
	}
}

func (e *pointsEditor) createRow(rec *gurps.PointsRecord, index int) {
	deleteButton := unison.NewSVGButton(res.TrashSVG)
	deleteButton.Tooltip = unison.NewTooltipWithText(i18n.Text("Remove Entry"))
	deleteButton.ClickCallback = func() { e.removeEntry(rec) }
	e.content.AddChildAtIndex(deleteButton, index)
	if index != -1 {
		index++
	}

	var when *widget.StringField
	whenText := i18n.Text("When")
	when = widget.NewStringField(nil, "", whenText,
		func() string { return rec.When.String() },
		func(value string) {
			t, err := jio.NewTimeFrom(value)
			if err != nil {
				return
			}
			rec.When = t
			widget.MarkModified(e.content)
		})
	when.ValidateCallback = func() bool {
		_, err := jio.NewTimeFrom(when.Text())
		return err == nil
	}
	when.Watermark = whenText
	when.SetMinimumTextWidthUsing(jio.Now().String() + "abcdefg")
	when.SetLayoutData(&unison.FlexLayoutData{HAlign: unison.FillAlignment})
	e.content.AddChildAtIndex(when, index)
	if index != -1 {
		index++
	}

	pts := widget.NewDecimalField(nil, "", i18n.Text("Points"),
		func() fxp.Int { return rec.Points },
		func(value fxp.Int) {
			rec.Points = value
			widget.MarkModified(e.content)
		}, fxp.Min, fxp.Max, true, false)
	e.content.AddChildAtIndex(pts, index)
	if index != -1 {
		index++
	}

	reasonText := i18n.Text("Reason")
	reason := widget.NewStringField(nil, "", reasonText,
		func() string { return rec.Reason },
		func(value string) {
			rec.Reason = value
			widget.MarkModified(e.content)
		})
	reason.Watermark = reasonText
	e.content.AddChildAtIndex(reason, index)
}

func (e *pointsEditor) addEntry() {
	rec := &gurps.PointsRecord{When: jio.Now()}
	e.current = slices.Insert(e.current, 0, rec)
	e.createRow(rec, 0)
	e.content.Pack()
	e.content.MarkForRedraw()
	widget.MarkModified(e.content)
	e.content.Children()[2].RequestFocus()
}

func (e *pointsEditor) removeEntry(rec *gurps.PointsRecord) {
	for i, one := range e.current {
		if one == rec {
			e.current = slices.Delete(e.current, i, i+1)
			i *= 4
			for j := 3; j >= 0; j-- {
				e.content.RemoveChildAtIndex(i + j)
			}
			e.content.Pack()
			widget.MarkForLayoutWithinDockable(e.content)
			e.content.MarkForRedraw()
			widget.MarkModified(e.content)
			break
		}
	}
}

func (e *pointsEditor) TitleIcon(suggestedSize unison.Size) unison.Drawable {
	return &unison.DrawableSVG{
		SVG:  res.EditSVG,
		Size: suggestedSize,
	}
}

func (e *pointsEditor) Title() string {
	return fmt.Sprintf(i18n.Text("Points Record for %s"), e.owner.String())
}

func (e *pointsEditor) String() string {
	return e.Title()
}

func (e *pointsEditor) Tooltip() string {
	return ""
}

func (e *pointsEditor) Modified() bool {
	modified := !reflect.DeepEqual(e.before, e.current)
	e.applyButton.SetEnabled(modified)
	e.cancelButton.SetEnabled(modified)
	return modified
}

func (e *pointsEditor) MarkModified(_ unison.Paneler) {
	if dc := unison.Ancestor[*unison.DockContainer](e); dc != nil {
		dc.UpdateTitle(e)
	}
	widget.DeepSync(e)
}

func (e *pointsEditor) Rebuild(_ bool) {
	e.MarkModified(nil)
	e.MarkForLayoutRecursively()
	e.MarkForRedraw()
}

func (e *pointsEditor) CloseWithGroup(other unison.Paneler) bool {
	return e.owner != nil && e.owner == other
}

func (e *pointsEditor) MayAttemptClose() bool {
	return workspace.MayAttemptCloseOfGroup(e)
}

func (e *pointsEditor) AttemptClose() bool {
	if !workspace.CloseGroup(e) {
		return false
	}
	if dc := unison.Ancestor[*unison.DockContainer](e); dc != nil {
		if e.promptForSave && !reflect.DeepEqual(e.before, e.current) {
			switch unison.YesNoCancelDialog(fmt.Sprintf(i18n.Text("Save changes made to\n%s?"), e.Title()), "") {
			case unison.ModalResponseDiscard:
			case unison.ModalResponseOK:
				e.apply()
			case unison.ModalResponseCancel:
				return false
			}
		}
		dc.Close(e)
		if !toolbox.IsNil(e.previousDockable) {
			if dc = unison.Ancestor[*unison.DockContainer](e.previousDockable); dc != nil {
				dc.SetCurrentDockable(e.previousDockable)
				if e.previousFocusKey != "" {
					if p := e.previousDockable.AsPanel().FindRefKey(e.previousFocusKey); p != nil {
						p.RequestFocus()
					}
				}
			}
		}
	}
	return true
}

func (e *pointsEditor) UndoManager() *unison.UndoManager {
	return e.undoMgr
}

func (e *pointsEditor) apply() {
	e.Window().FocusNext() // Intentionally move the focus to ensure any pending edits are flushed
	owner := e.owner
	entity := e.entity
	if mgr := unison.UndoManagerFor(e.owner); mgr != nil {
		mgr.Add(&unison.UndoEdit[[]*gurps.PointsRecord]{
			ID:       unison.NextUndoID(),
			EditName: i18n.Text("Point Record Changes"),
			UndoFunc: func(edit *unison.UndoEdit[[]*gurps.PointsRecord]) {
				entity.SetPointsRecord(edit.BeforeData)
				owner.Rebuild(false)
			},
			RedoFunc: func(edit *unison.UndoEdit[[]*gurps.PointsRecord]) {
				entity.SetPointsRecord(edit.AfterData)
				owner.Rebuild(false)
			},
			BeforeData: e.before,
			AfterData:  e.current,
		})
	}
	entity.SetPointsRecord(e.current)
	owner.Rebuild(true)
}
