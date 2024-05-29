// Copyright (c) 1998-2024 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

export function apiPrefix(path: string) {
	return encodeURI((import.meta.env.DEV ? 'http://localhost:8422' : '') + '/api' + path);
}

export function refPrefix(prefix: string) {
	return encodeURI((import.meta.env.DEV ? 'http://localhost:8422' : '') + '/pdf/web/viewer.html?file=/ref/' + encodeURI(prefix));
}