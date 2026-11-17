// SPDX-License-Identifier: GPL-3.0-only
package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"time"
)

type Source struct {
	ID        string `json:"id"`
	URL       string `json:"url,omitempty"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Revision  int64  `json:"revision,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

func ProfileMode(p Profile) string {
	if p.Kind == "vk" {
		return "turn"
	}
	return "vpn"
}
func sourceID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}
func profileID(p Profile) string {
	if p.ExternalID != "" {
		return sourceID(p.SourceID + "\x00" + p.Kind + "\x00" + p.ExternalID)
	}
	return sourceID(p.SourceID + "\x00" + p.Kind + "\x00" + string(p.Raw))
}
func (s State) Source(id string) (Source, bool) {
	for _, source := range s.Sources {
		if source.ID == id {
			return source, true
		}
	}
	return Source{}, false
}
func (s State) MinimumRevision(link string) int64 {
	for _, source := range s.Sources {
		if source.URL == link && link != "" {
			return source.Revision
		}
	}
	return 0
}

// Normalize migrates the original single-source DPAPI document without dropping data.
func (s *State) Normalize() {
	if len(s.Sources) == 0 && len(s.Profiles) > 0 {
		id := sourceID("legacy-manual")
		name := "Добавленные вручную"
		if s.SubscriptionURL != "" {
			id = sourceID(s.SubscriptionURL)
			if u, e := url.Parse(s.SubscriptionURL); e == nil {
				name = u.Hostname()
			}
		}
		kind := "vpn"
		for _, p := range s.Profiles {
			if ProfileMode(p) == "turn" {
				kind = "turn"
				break
			}
		}
		s.Sources = []Source{{ID: id, URL: s.SubscriptionURL, Name: name, Kind: kind, Revision: s.Revision, ExpiresAt: s.ExpiresAt}}
		for i := range s.Profiles {
			s.Profiles[i].SourceID = id
		}
	}
	for i := range s.Profiles {
		if s.Profiles[i].ID == "" {
			s.Profiles[i].ID = profileID(s.Profiles[i])
		}
	}
	if s.Selections == nil {
		s.Selections = map[string]string{}
	}
	if s.Selected < 0 || s.Selected >= len(s.Profiles) {
		s.Selected = 0
	}
	if len(s.Profiles) > 0 {
		s.Selections[ProfileMode(s.Profiles[s.Selected])] = s.Profiles[s.Selected].ID
	}
}

func (s State) Clone() State {
	s.Sources = append([]Source(nil), s.Sources...)
	s.Profiles = append([]Profile(nil), s.Profiles...)
	selections := map[string]string{}
	for k, v := range s.Selections {
		selections[k] = v
	}
	s.Selections = selections
	return s
}

// MergeImport replaces one source, preserving every independently added subscription.
// A failed parse/fetch never calls this function; the controller commits atomically.
func (s State) MergeImport(result ImportResult, link string, now time.Time) (State, error) {
	next := s.Clone()
	next.Normalize()
	if len(result.Profiles) == 0 {
		return s, errors.New("В источнике нет поддерживаемых профилей")
	}
	kind := "vpn"
	for _, p := range result.Profiles {
		if ProfileMode(p) == "turn" {
			kind = "turn"
			break
		}
	}
	id := sourceID(link)
	name := "Профили из файла или текста"
	if link == "" {
		raw, _ := json.Marshal(result.Profiles)
		id = sourceID("manual:" + string(raw))
	} else if u, e := url.Parse(link); e == nil {
		name = u.Hostname()
	}
	selectedID := ""
	if len(next.Profiles) > 0 {
		selectedID = next.Profiles[next.Selected].ID
	}
	source := Source{ID: id, URL: link, Name: name, Kind: kind, Revision: result.Revision, ExpiresAt: result.ExpiresAt, UpdatedAt: now.Unix()}
	found := false
	for i, old := range next.Sources {
		if old.ID == id {
			next.Sources[i] = source
			found = true
			break
		}
	}
	if !found {
		next.Sources = append(next.Sources, source)
	}
	kept := make([]Profile, 0, len(next.Profiles)+len(result.Profiles))
	for _, p := range next.Profiles {
		if p.SourceID != id {
			kept = append(kept, p)
		}
	}
	seen := map[string]bool{}
	first := len(kept)
	for _, p := range result.Profiles {
		p.SourceID = id
		p.ID = profileID(p)
		if !seen[p.ID] {
			kept = append(kept, p)
			seen[p.ID] = true
		}
	}
	if len(kept) > 1024 || len(next.Sources) > 64 {
		return s, errors.New("Лимит: 1024 профиля и 64 источника")
	}
	next.Profiles = kept
	next.Selected = first
	// Keep a selected server on refresh. Adding a different source selects its first server.
	if found {
		for i, p := range kept {
			if p.ID == selectedID {
				next.Selected = i
				break
			}
		}
	}
	next.SelectProfile(next.Selected)
	return next, nil
}

func (s *State) SelectProfile(index int) bool {
	if index < 0 || index >= len(s.Profiles) {
		return false
	}
	s.Selected = index
	p := s.Profiles[index]
	if s.Selections == nil {
		s.Selections = map[string]string{}
	}
	s.Selections[ProfileMode(p)] = p.ID
	if source, ok := s.Source(p.SourceID); ok {
		s.SubscriptionURL = source.URL
		s.Revision = source.Revision
		s.ExpiresAt = source.ExpiresAt
	}
	return true
}
func (s State) PreferredIndex(mode string) int {
	for i, p := range s.Profiles {
		if ProfileMode(p) == mode && p.ID == s.Selections[mode] {
			return i
		}
	}
	for i, p := range s.Profiles {
		if ProfileMode(p) == mode {
			return i
		}
	}
	return -1
}
func (s State) RemoveProfile(index int) State {
	next := s.Clone()
	selected := ""
	if s.Selected >= 0 && s.Selected < len(s.Profiles) {
		selected = s.Profiles[s.Selected].ID
	}
	next.Profiles = append(next.Profiles[:index], next.Profiles[index+1:]...)
	next.Selected = 0
	for i, p := range next.Profiles {
		if p.ID == selected {
			next.Selected = i
			break
		}
	}
	if len(next.Profiles) > 0 {
		next.SelectProfile(next.Selected)
	}
	return next
}
