/*
 * Copyright ©1998-2021 by Richard A. Wilkes. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, version 2.0. If a copy of the MPL was not distributed with
 * this file, You can obtain one at http://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as
 * defined by the Mozilla Public License, version 2.0.
 */

package com.trollworks.gcs.settings;

import com.trollworks.gcs.attribute.Attribute;
import com.trollworks.gcs.attribute.AttributeDef;
import com.trollworks.gcs.attribute.AttributeListPanel;
import com.trollworks.gcs.attribute.AttributeSet;
import com.trollworks.gcs.character.GURPSCharacter;
import com.trollworks.gcs.datafile.DataChangeListener;
import com.trollworks.gcs.library.Library;
import com.trollworks.gcs.menu.file.CloseHandler;
import com.trollworks.gcs.preferences.Preferences;
import com.trollworks.gcs.ui.UIUtilities;
import com.trollworks.gcs.ui.layout.PrecisionLayout;
import com.trollworks.gcs.ui.layout.PrecisionLayoutAlignment;
import com.trollworks.gcs.ui.layout.PrecisionLayoutData;
import com.trollworks.gcs.ui.widget.BaseWindow;
import com.trollworks.gcs.ui.widget.FontAwesomeButton;
import com.trollworks.gcs.ui.widget.StdFileDialog;
import com.trollworks.gcs.ui.widget.WindowUtils;
import com.trollworks.gcs.utility.FileType;
import com.trollworks.gcs.utility.I18n;
import com.trollworks.gcs.utility.Log;
import com.trollworks.gcs.utility.PathUtils;
import com.trollworks.gcs.utility.SafeFileUpdater;
import com.trollworks.gcs.utility.json.JsonWriter;

import java.awt.BorderLayout;
import java.awt.Container;
import java.awt.Dimension;
import java.awt.EventQueue;
import java.awt.Point;
import java.awt.Window;
import java.awt.event.WindowEvent;
import java.io.BufferedWriter;
import java.io.File;
import java.io.FileWriter;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.DirectoryStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import javax.swing.JMenuItem;
import javax.swing.JPanel;
import javax.swing.JPopupMenu;
import javax.swing.JScrollPane;

/** A window for editing attribute settings. */
public class AttributeSettingsWindow extends BaseWindow implements CloseHandler, DataChangeListener {
    private static Map<UUID, AttributeSettingsWindow> INSTANCES = new HashMap<>();
    private        GURPSCharacter                     mCharacter;
    private        AttributeListPanel                 mListPanel;
    private        FontAwesomeButton                  mMenuButton;
    private        JScrollPane                        mScroller;
    private        boolean                            mResetEnabled;
    private        boolean                            mUpdatePending;

    /** Displays the attribute settings window. */
    public static void display(GURPSCharacter gchar) {
        if (!UIUtilities.inModalState()) {
            AttributeSettingsWindow wnd;
            synchronized (INSTANCES) {
                UUID key = gchar == null ? null : gchar.getID();
                wnd = INSTANCES.get(key);
                if (wnd == null) {
                    wnd = new AttributeSettingsWindow(gchar);
                    INSTANCES.put(key, wnd);
                }
            }
            wnd.setVisible(true);
        }
    }

    /** Closes the AttributeSettingsWindow for the given character if it is open. */
    public static void closeFor(GURPSCharacter gchar) {
        for (Window window : Window.getWindows()) {
            if (window.isShowing() && window instanceof AttributeSettingsWindow) {
                AttributeSettingsWindow wnd = (AttributeSettingsWindow) window;
                if (wnd.mCharacter == gchar) {
                    wnd.attemptClose();
                }
            }
        }
    }

    private static String createTitle(GURPSCharacter gchar) {
        return gchar == null ? I18n.Text("Default Attributes") : String.format(I18n.Text("Attributes for %s"), gchar.getProfile().getName());
    }

    private AttributeSettingsWindow(GURPSCharacter gchar) {
        super(createTitle(gchar));
        mCharacter = gchar;
        Container content = getContentPane();
        JPanel    header  = new JPanel(new PrecisionLayout().setColumns(2).setMargins(5, 10, 5, 10).setHorizontalSpacing(10));
        header.add(new FontAwesomeButton("\uf055", I18n.Text("Add Attribute"), () -> mListPanel.addAttribute()));
        mMenuButton = new FontAwesomeButton("\uf0c9", I18n.Text("Menu"), this::actionMenu);
        header.add(mMenuButton, new PrecisionLayoutData().setGrabHorizontalSpace(true).setHorizontalAlignment(PrecisionLayoutAlignment.END));
        content.add(header, BorderLayout.NORTH);
        mListPanel = new AttributeListPanel(getAttributes(), () -> {
            adjustResetButton();
            if (mCharacter != null) {
                mCharacter.notifyOfChange();
            }
        });
        mScroller = new JScrollPane(mListPanel);
        mScroller.setBorder(null);
        content.add(mScroller, BorderLayout.CENTER);
        adjustResetButton();
        if (gchar != null) {
            gchar.addChangeListener(this);
            Preferences.getInstance().addChangeListener(this);
        }
        Dimension min1 = getMinimumSize();
        setMinimumSize(new Dimension(Math.max(min1.width, 600), min1.height));
        WindowUtils.packAndCenterWindowOn(this, null);
        EventQueue.invokeLater(() -> mScroller.getViewport().setViewPosition(new Point(0, 0)));
    }

    private Map<String, AttributeDef> getAttributes() {
        return SheetSettings.get(mCharacter).getAttributes();
    }

    private void actionMenu() {
        JPopupMenu menu = new JPopupMenu();
        menu.add(createMenuItem(I18n.Text("Import…"), this::importData, true));
        menu.add(createMenuItem(I18n.Text("Export…"), this::exportData, true));
        menu.addSeparator();
        menu.add(createMenuItem(mCharacter == null ? I18n.Text("Factory Default Attributes") : I18n.Text("Default Attributes"), this::reset, mResetEnabled));
        Preferences.getInstance(); // Just to ensure the libraries list is initialized
        for (Library lib : Library.LIBRARIES) {
            Path dir = lib.getPath().resolve("Attributes");
            if (Files.isDirectory(dir)) {
                List<AttributeSet> list = new ArrayList<>();
                // IMPORTANT: On Windows, calling any of the older methods to list the contents of a
                // directory results in leaving state around that prevents future move & delete
                // operations. Only use this style of access for directory listings to avoid that.
                try (DirectoryStream<Path> stream = Files.newDirectoryStream(dir)) {
                    for (Path path : stream) {
                        try {
                            list.add(new AttributeSet(path));
                        } catch (IOException ioe) {
                            Log.error("unable to load " + path, ioe);
                        }
                    }
                } catch (IOException exception) {
                    Log.error(exception);
                }
                if (!list.isEmpty()) {
                    Collections.sort(list);
                    menu.addSeparator();
                    menu.add(createMenuItem(dir.getParent().getFileName().toString(), null, false));
                    for (AttributeSet choice : list) {
                        menu.add(createMenuItem(choice.toString(), () -> {
                            Map<String, AttributeDef> attrs = choice.getAttributes();
                            if (attrs != null) {
                                reset(attrs);
                            }
                        }, true));
                    }
                }
            }
        }
        menu.show(mMenuButton, 0, 0);
    }

    private JMenuItem createMenuItem(String title, Runnable onSelection, boolean enabled) {
        JMenuItem item = new JMenuItem(title);
        item.addActionListener((evt) -> onSelection.run());
        item.setEnabled(enabled);
        return item;
    }

    private void importData() {
        Path path = StdFileDialog.showOpenDialog(this, I18n.Text("Import…"),
                FileType.ATTRIBUTE_SETTINGS.getFilter());
        if (path != null) {
            try {
                AttributeSet set = new AttributeSet(path);
                reset(set.getAttributes());
            } catch (IOException ioe) {
                Log.error(ioe);
                WindowUtils.showError(this, I18n.Text("Unable to import attribute settings."));
            }
        }
    }

    private void exportData() {
        Path path = StdFileDialog.showSaveDialog(this, I18n.Text("Export…"),
                Preferences.getInstance().getLastDir().resolve(I18n.Text("attribute_settings")),
                FileType.ATTRIBUTE_SETTINGS.getFilter());
        if (path != null) {
            SafeFileUpdater transaction = new SafeFileUpdater();
            transaction.begin();
            try {
                File         file = transaction.getTransactionFile(path.toFile());
                AttributeSet set  = new AttributeSet(PathUtils.getLeafName(path, false), mListPanel.getAttributes());
                try (JsonWriter w = new JsonWriter(new BufferedWriter(new FileWriter(file, StandardCharsets.UTF_8)), "\t")) {
                    set.toJSON(w);
                }
                transaction.commit();
            } catch (Exception exception) {
                Log.error(exception);
                transaction.abort();
                WindowUtils.showError(this, I18n.Text("Unable to export attribute settings."));
            }
        }
    }

    private void reset() {
        Map<String, AttributeDef> attributes;
        if (mCharacter == null) {
            attributes = AttributeDef.createStandardAttributes();
        } else {
            attributes = AttributeDef.cloneMap(Preferences.getInstance().getSheetSettings().getAttributes());
        }
        reset(attributes);
        adjustResetButton();
    }

    private void reset(Map<String, AttributeDef> attributes) {
        mListPanel.reset(attributes);
        mListPanel.getAdjustCallback().run();
        revalidate();
        repaint();
        EventQueue.invokeLater(() -> mScroller.getViewport().setViewPosition(new Point(0, 0)));
    }

    private void adjustResetButton() {
        Map<String, AttributeDef> prefsAttributes = Preferences.getInstance().getSheetSettings().getAttributes();
        if (mCharacter == null) {
            mResetEnabled = !prefsAttributes.equals(AttributeDef.createStandardAttributes());
        } else {
            Map<String, Attribute> oldAttributes = mCharacter.getAttributes();
            Map<String, Attribute> newAttributes = new HashMap<>();
            for (String key : mCharacter.getSheetSettings().getAttributes().keySet()) {
                Attribute attribute = oldAttributes.get(key);
                newAttributes.put(key, attribute != null ? attribute : new Attribute(key));
            }
            if (!oldAttributes.equals(newAttributes)) {
                oldAttributes.clear();
                oldAttributes.putAll(newAttributes);
                mCharacter.notifyOfChange();
            }
            mListPanel.adjustButtons();
            mResetEnabled = !mCharacter.getSheetSettings().getAttributes().equals(prefsAttributes);
        }
    }

    @Override
    public boolean mayAttemptClose() {
        return true;
    }

    @Override
    public boolean attemptClose() {
        windowClosing(new WindowEvent(this, WindowEvent.WINDOW_CLOSING));
        return true;
    }

    @Override
    public void dispose() {
        synchronized (INSTANCES) {
            INSTANCES.remove(mCharacter == null ? null : mCharacter.getID());
        }
        if (mCharacter != null) {
            mCharacter.removeChangeListener(this);
            Preferences.getInstance().removeChangeListener(this);
        }
        super.dispose();
    }

    @Override
    public void dataWasChanged() {
        if (!mUpdatePending) {
            mUpdatePending = true;
            EventQueue.invokeLater(() -> {
                setTitle(createTitle(mCharacter));
                adjustResetButton();
                mUpdatePending = false;
            });
        }
    }
}