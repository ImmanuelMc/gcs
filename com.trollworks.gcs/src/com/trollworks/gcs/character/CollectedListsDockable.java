/*
 * Copyright ©1998-2020 by Richard A. Wilkes. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, version 2.0. If a copy of the MPL was not distributed with
 * this file, You can obtain one at http://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as
 * defined by the Mozilla Public License, version 2.0.
 */

package com.trollworks.gcs.character;

import com.trollworks.gcs.advantage.Advantage;
import com.trollworks.gcs.datafile.DataFile;
import com.trollworks.gcs.datafile.DataFileDockable;
import com.trollworks.gcs.equipment.Equipment;
import com.trollworks.gcs.equipment.EquipmentList;
import com.trollworks.gcs.menu.RetargetableFocus;
import com.trollworks.gcs.notes.Note;
import com.trollworks.gcs.preferences.Preferences;
import com.trollworks.gcs.skill.Skill;
import com.trollworks.gcs.skill.Technique;
import com.trollworks.gcs.spell.RitualMagicSpell;
import com.trollworks.gcs.spell.Spell;
import com.trollworks.gcs.ui.scale.Scales;
import com.trollworks.gcs.ui.widget.Toolbar;
import com.trollworks.gcs.ui.widget.outline.ListOutline;
import com.trollworks.gcs.ui.widget.outline.ListRow;
import com.trollworks.gcs.ui.widget.outline.Outline;
import com.trollworks.gcs.ui.widget.outline.OutlineModel;
import com.trollworks.gcs.ui.widget.outline.Row;
import com.trollworks.gcs.ui.widget.outline.RowItemRenderer;
import com.trollworks.gcs.ui.widget.outline.RowIterator;
import com.trollworks.gcs.ui.widget.outline.RowPostProcessor;
import com.trollworks.gcs.ui.widget.search.Search;
import com.trollworks.gcs.ui.widget.search.SearchTarget;
import com.trollworks.gcs.utility.I18n;
import com.trollworks.gcs.utility.notification.NotifierTarget;

import java.awt.BorderLayout;
import java.awt.EventQueue;
import java.awt.KeyboardFocusManager;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import javax.swing.JComboBox;
import javax.swing.ListCellRenderer;
import javax.swing.undo.StateEdit;

public abstract class CollectedListsDockable extends DataFileDockable implements SearchTarget, RetargetableFocus, NotifierTarget {
    private JComboBox<Scales> mScaleCombo;
    private Search            mSearch;

    public CollectedListsDockable(DataFile dataFile) {
        super(dataFile);
    }

    protected Toolbar createToolbar() {
        Toolbar toolbar = new Toolbar();
        mScaleCombo = new JComboBox<>(Scales.values());
        mScaleCombo.setSelectedItem(Preferences.getInstance().getInitialUIScale());
        mScaleCombo.addActionListener((event) -> {
            Scales scale = (Scales) mScaleCombo.getSelectedItem();
            if (scale == null) {
                scale = Scales.ACTUAL_SIZE;
            }
            getCollectedLists().setScale(scale.getScale());
        });
        toolbar.add(mScaleCombo);
        mSearch = new Search(this);
        toolbar.add(mSearch, Toolbar.LAYOUT_FILL);
        add(toolbar, BorderLayout.NORTH);
        return toolbar;
    }

    public abstract CollectedLists getCollectedLists();

    @Override
    public boolean isJumpToSearchAvailable() {
        return mSearch.isEnabled() && mSearch != KeyboardFocusManager.getCurrentKeyboardFocusManager().getPermanentFocusOwner();
    }

    @Override
    public void jumpToSearchField() {
        mSearch.requestFocus();
    }

    @Override
    public ListCellRenderer<Object> getSearchRenderer() {
        return new RowItemRenderer();
    }

    @Override
    public List<Object> search(String filter) {
        List<Object> list = new ArrayList<>();
        filter = filter.toLowerCase();
        CollectedLists lists = getCollectedLists();
        searchOne(lists.getAdvantageOutline(), filter, list);
        searchOne(lists.getSkillOutline(), filter, list);
        searchOne(lists.getSpellOutline(), filter, list);
        searchOne(lists.getEquipmentOutline(), filter, list);
        searchOne(lists.getOtherEquipmentOutline(), filter, list);
        searchOne(lists.getNoteOutline(), filter, list);
        return list;
    }

    private static void searchOne(ListOutline outline, String text, List<Object> list) {
        for (ListRow row : new RowIterator<ListRow>(outline.getModel())) {
            if (row.contains(text, true)) {
                list.add(row);
            }
        }
    }

    @Override
    public void searchSelect(List<Object> selection) {
        CollectedLists               lists   = getCollectedLists();
        Map<OutlineModel, List<Row>> map     = new HashMap<>();
        Outline                      primary = null;
        List<Row>                    list;

        lists.getAdvantageOutline().getModel().deselect();
        lists.getSkillOutline().getModel().deselect();
        lists.getSpellOutline().getModel().deselect();
        lists.getEquipmentOutline().getModel().deselect();
        lists.getOtherEquipmentOutline().getModel().deselect();
        lists.getNoteOutline().getModel().deselect();

        for (Object obj : selection) {
            Row          row    = (Row) obj;
            Row          parent = row.getParent();
            OutlineModel model  = row.getOwner();

            while (parent != null) {
                parent.setOpen(true);
                model = parent.getOwner();
                parent = parent.getParent();
            }
            list = map.get(model);
            if (list == null) {
                list = new ArrayList<>();
                list.add(row);
                map.put(model, list);
            } else {
                list.add(row);
            }
            if (primary == null) {
                primary = lists.getAdvantageOutline();
                if (model != primary.getModel()) {
                    primary = lists.getSkillOutline();
                    if (model != primary.getModel()) {
                        primary = lists.getSpellOutline();
                        if (model != primary.getModel()) {
                            primary = lists.getEquipmentOutline();
                            if (model != primary.getModel()) {
                                primary = lists.getOtherEquipmentOutline();
                                if (model != primary.getModel()) {
                                    primary = lists.getNoteOutline();
                                    if (model != primary.getModel()) {
                                        primary = null;
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        for (Map.Entry<OutlineModel, List<Row>> entry : map.entrySet()) {
            entry.getKey().select(entry.getValue(), false);
        }

        if (primary != null) {
            Outline outline = primary;
            EventQueue.invokeLater(() -> outline.scrollSelectionIntoView());
            primary.requestFocus();
        }
    }

    /**
     * Adds rows to the display.
     *
     * @param rows The rows to add.
     */
    public void addRows(List<Row> rows) {
        CollectedLists              lists       = getCollectedLists();
        Map<ListOutline, StateEdit> map         = new HashMap<>();
        Map<Outline, List<Row>>     selMap      = new HashMap<>();
        Map<Outline, List<ListRow>> nameMap     = new HashMap<>();
        ListOutline                 outline     = null;
        String                      addRowsText = I18n.Text("Add Rows");
        for (Row row : rows) {
            if (row instanceof Advantage) {
                outline = lists.getAdvantageOutline();
                if (!map.containsKey(outline)) {
                    map.put(outline, new StateEdit(outline.getModel(), addRowsText));
                }
                row = new Advantage(getDataFile(), (Advantage) row, true);
                addCompleteRow(outline, row, selMap);
            } else if (row instanceof Technique) {
                outline = lists.getSkillOutline();
                if (!map.containsKey(outline)) {
                    map.put(outline, new StateEdit(outline.getModel(), addRowsText));
                }
                row = new Technique(getDataFile(), (Technique) row, true);
                addCompleteRow(outline, row, selMap);
            } else if (row instanceof Skill) {
                outline = lists.getSkillOutline();
                if (!map.containsKey(outline)) {
                    map.put(outline, new StateEdit(outline.getModel(), addRowsText));
                }
                row = new Skill(getDataFile(), (Skill) row, true, true);
                addCompleteRow(outline, row, selMap);
            } else if (row instanceof RitualMagicSpell) {
                outline = lists.getSpellOutline();
                if (!map.containsKey(outline)) {
                    map.put(outline, new StateEdit(outline.getModel(), addRowsText));
                }
                row = new RitualMagicSpell(getDataFile(), (RitualMagicSpell) row, true, true);
                addCompleteRow(outline, row, selMap);
            } else if (row instanceof Spell) {
                outline = lists.getSpellOutline();
                if (!map.containsKey(outline)) {
                    map.put(outline, new StateEdit(outline.getModel(), addRowsText));
                }
                row = new Spell(getDataFile(), (Spell) row, true, true);
                addCompleteRow(outline, row, selMap);
            } else if (row instanceof Equipment) {
                outline = row.getOwner().getProperty(EquipmentList.TAG_OTHER_ROOT) != null ? lists.getOtherEquipmentOutline() : lists.getEquipmentOutline();
                if (!map.containsKey(outline)) {
                    map.put(outline, new StateEdit(outline.getModel(), addRowsText));
                }
                row = new Equipment(getDataFile(), (Equipment) row, true);
                addCompleteRow(outline, row, selMap);
            } else if (row instanceof Note) {
                outline = lists.getNoteOutline();
                if (!map.containsKey(outline)) {
                    map.put(outline, new StateEdit(outline.getModel(), addRowsText));
                }
                row = new Note(getDataFile(), (Note) row, true);
                addCompleteRow(outline, row, selMap);
            } else {
                row = null;
            }
            //noinspection ConstantConditions
            if (row instanceof ListRow) {
                List<ListRow> process = nameMap.get(outline);
                if (process == null) {
                    process = new ArrayList<>();
                    nameMap.put(outline, process);
                }
                addRowsToBeProcessed(process, (ListRow) row);
            }
        }
        for (Map.Entry<ListOutline, StateEdit> entry : map.entrySet()) {
            ListOutline  anOutline = entry.getKey();
            OutlineModel model     = anOutline.getModel();
            model.select(selMap.get(anOutline), false);
            StateEdit edit = entry.getValue();
            edit.end();
            anOutline.postUndo(edit);
            anOutline.scrollSelectionIntoView();
            anOutline.requestFocus();
        }
        if (!nameMap.isEmpty()) {
            EventQueue.invokeLater(new RowPostProcessor(nameMap));
        }
    }

    private void addRowsToBeProcessed(List<ListRow> list, ListRow row) {
        int count = row.getChildCount();
        list.add(row);
        for (int i = 0; i < count; i++) {
            addRowsToBeProcessed(list, (ListRow) row.getChild(i));
        }
    }

    private void addCompleteRow(Outline outline, Row row, Map<Outline, List<Row>> selMap) {
        List<Row> selection = selMap.get(outline);
        addCompleteRow(outline.getModel(), row);
        outline.contentSizeMayHaveChanged();
        if (selection == null) {
            selection = new ArrayList<>();
            selMap.put(outline, selection);
        }
        selection.add(row);
    }

    private void addCompleteRow(OutlineModel outlineModel, Row row) {
        outlineModel.addRow(row);
        if (row.isOpen() && row.hasChildren()) {
            for (Row child : row.getChildren()) {
                addCompleteRow(outlineModel, child);
            }
        }
    }

    @Override
    public int getNotificationPriority() {
        return 0;
    }
}
