/*
 * Copyright ©1998-2024 by Richard A. Wilkes. All rights reserved.
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
	"crypto/sha256"
	"strings"
	"sync"

	"github.com/richardwilkes/gcs/v5/model/fxp"
)

// Minimums and defaults for web server settings.
var (
	MinimumShutdownGracePeriod = fxp.Int(0)
	DefaultShutdownGracePeriod = fxp.Int(0)
	MinimumReadTimeout         = fxp.One
	DefaultReadTimeout         = fxp.Ten
	MinimumWriteTimeout        = fxp.One
	DefaultWriteTimeout        = fxp.Thirty
	MinimumIdleTimeout         = fxp.One
	DefaultIdleTimeout         = fxp.Sixty
)

// WebServerSettings holds the settings for the embedded web server.
type WebServerSettings struct {
	Enabled             bool              `json:"enabled"`
	Address             string            `json:"address,omitempty"`
	CertFile            string            `json:"cert_file,omitempty"`
	KeyFile             string            `json:"key_file,omitempty"`
	ShutdownGracePeriod fxp.Int           `json:"shutdown_grace_period,omitempty"`
	ReadTimeout         fxp.Int           `json:"read_timeout,omitempty"`
	WriteTimeout        fxp.Int           `json:"write_timeout,omitempty"`
	IdleTimeout         fxp.Int           `json:"idle_timeout,omitempty"`
	Lock                sync.RWMutex      `json:"-"`
	Users               map[string][]byte `json:"users,omitempty"`
}

// Validate the settings.
func (s *WebServerSettings) Validate() {
	s.Address = strings.TrimSpace(s.Address)
	if s.Address == "" {
		s.Address = "localhost:0"
	}
	if s.ShutdownGracePeriod < MinimumShutdownGracePeriod {
		s.ShutdownGracePeriod = DefaultShutdownGracePeriod
	}
	if s.ReadTimeout < MinimumReadTimeout {
		s.ReadTimeout = DefaultReadTimeout
	}
	if s.WriteTimeout < MinimumWriteTimeout {
		s.WriteTimeout = DefaultWriteTimeout
	}
	if s.IdleTimeout < MinimumIdleTimeout {
		s.IdleTimeout = DefaultIdleTimeout
	}
}

// HashedPasswordLookup looks up hashed passwords.
func (s *WebServerSettings) HashedPasswordLookup(user, _ string) ([]byte, bool) {
	s.Lock.RLock()
	defer s.Lock.RUnlock()
	pw, ok := s.Users[user]
	return pw, ok
}

// Hasher hashes passwords.
func (s *WebServerSettings) Hasher(in string) []byte {
	h := sha256.Sum256([]byte(in + "!gcs"))
	return h[:]
}