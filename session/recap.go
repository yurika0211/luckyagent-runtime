package session

import (
	"strings"
	"time"

	"github.com/yurika0211/luckyagent-runtime/appservice"
	"github.com/yurika0211/luckyagent-runtime/provider"
)

// maxSessionRecaps bounds the ordered preview list. Older entries drop first.
const maxSessionRecaps = 32

// SessionRecap is one stored preview. The alias keeps metadata in this package.
type SessionRecap = appservice.SessionRecap

// Recap returns the user-facing preview without loading the transcript.
func (s *Session) Recap() (string, *time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recap, cloneRecapTime(s.recapUpdatedAt)
}

func cloneRecapTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	copy := *t
	return &copy
}

func cloneRecaps(in []SessionRecap) []SessionRecap {
	if len(in) == 0 {
		return nil
	}
	out := make([]SessionRecap, len(in))
	copy(out, in)
	return out
}

// normalizeRecaps keeps recap / recap_updated_at as the latest entry.
// A file that only has those two fields becomes a one-item list.
func normalizeRecaps(latest string, at *time.Time, list []SessionRecap) (string, *time.Time, []SessionRecap) {
	cleaned := make([]SessionRecap, 0, len(list))
	for _, item := range list {
		text := strings.TrimSpace(item.Text)
		if text == "" || item.UpdatedAt.IsZero() {
			continue
		}
		cleaned = append(cleaned, SessionRecap{Text: text, UpdatedAt: item.UpdatedAt.UTC()})
	}
	if len(cleaned) == 0 && strings.TrimSpace(latest) != "" && at != nil && !at.IsZero() {
		cleaned = append(cleaned, SessionRecap{Text: strings.TrimSpace(latest), UpdatedAt: at.UTC()})
	}
	if len(cleaned) > maxSessionRecaps {
		cleaned = cleaned[len(cleaned)-maxSessionRecaps:]
	}
	if len(cleaned) == 0 {
		return "", nil, nil
	}
	last := cleaned[len(cleaned)-1]
	return last.Text, cloneRecapTime(&last.UpdatedAt), cleaned
}

// appendRecap adds one preview. The same updated_at is not stored twice.
func appendRecap(list []SessionRecap, text string, at time.Time) []SessionRecap {
	text = strings.TrimSpace(text)
	if text == "" || at.IsZero() {
		return cloneRecaps(list)
	}
	at = at.UTC()
	out := cloneRecaps(list)
	for _, item := range out {
		if item.UpdatedAt.Equal(at) {
			return out
		}
	}
	out = append(out, SessionRecap{Text: text, UpdatedAt: at})
	if len(out) > maxSessionRecaps {
		out = out[len(out)-maxSessionRecaps:]
	}
	return out
}

// Recaps returns the ordered preview list. The last item matches Recap.
func (s *Session) Recaps() []SessionRecap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRecaps(s.recaps)
}

// RecapSnapshot captures recent evidence and its version atomically. The caller
// must not mutate nested message data. Recaps are distinct from compact summaries.
func (s *Session) RecapSnapshot(limit int) (string, []provider.Message, time.Time, error) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if err := s.checkLiveLocked(); err != nil {
		return "", nil, time.Time{}, err
	}
	if err := s.loadMessages(); err != nil {
		return "", nil, time.Time{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.recapUpdatedAt != nil && !s.recapUpdatedAt.Before(s.UpdatedAt) {
		return s.recap, nil, s.UpdatedAt, nil
	}
	start := 0
	if limit > 0 && len(s.Messages) > limit {
		start = len(s.Messages) - limit
	}
	messages := append([]provider.Message(nil), s.Messages[start:]...)
	// A long tool loop can push the user's request outside the bounded tail.
	// Retain that request so generation still knows the task and its language.
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == "user" {
			if i < start {
				messages = append([]provider.Message{s.Messages[i]}, messages...)
			}
			break
		}
	}
	return s.recap, messages, s.UpdatedAt, nil
}

// UpdateRecap persists a generated preview only while its source is current.
// It never changes conversation ordering, title or transcript. Failed writes
// restore the previous preview; deletion and ID reuse cannot revive old handles.
func (s *Session) UpdateRecap(text string, sourceUpdatedAt time.Time) (bool, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return false, nil
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if err := s.checkLiveLocked(); err != nil {
		return false, err
	}
	s.mu.Lock()
	if !s.UpdatedAt.Equal(sourceUpdatedAt) || (s.recapUpdatedAt != nil && !s.recapUpdatedAt.Before(sourceUpdatedAt)) {
		s.mu.Unlock()
		return false, nil
	}
	previous, previousAt := s.recap, s.recapUpdatedAt
	previousList := cloneRecaps(s.recaps)
	// The list key is the conversation version, so a retried generation of the
	// same UpdatedAt does not add a second row.
	next := appendRecap(s.recaps, text, sourceUpdatedAt)
	if len(next) == len(previousList) {
		s.mu.Unlock()
		return false, nil
	}
	latest, latestAt, stored := normalizeRecaps(text, &sourceUpdatedAt, next)
	s.recap, s.recapUpdatedAt, s.recaps = latest, latestAt, stored
	s.mu.Unlock()
	if err := s.saveMetadataLocked(); err != nil {
		s.mu.Lock()
		s.recap, s.recapUpdatedAt, s.recaps = previous, previousAt, previousList
		s.mu.Unlock()
		return false, err
	}
	return true, nil
}
