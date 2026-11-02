package tui

import (
	"sort"

	tea "charm.land/bubbletea/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

type detailsResult struct {
	id, generation uint64
	page           inspection.Page
	err            error
}

// loadPage captures only immutable identifiers and the storage callback. The
// command never touches UI state; results merge on the Bubble Tea goroutine.
func (m *model) loadPage(s *session, before int) tea.Cmd {
	if s.loadDetails == nil || s.detailsLoading || m.detailBusy {
		return nil
	}
	m.detailBusy = true
	s.detailsLoading = true
	s.detailsError = ""
	s.loadGeneration++
	id, generation, load := s.id, s.loadGeneration, s.loadDetails
	return func() tea.Msg {
		page, err := load(before)
		return detailsResult{id: id, generation: generation, page: page, err: err}
	}
}

func (m *model) finishDetails(result detailsResult) tea.Cmd {
	m.detailBusy = false
	for _, s := range m.sessions {
		if s.id != result.id || s.loadGeneration != result.generation {
			continue
		}
		s.detailsLoading = false
		if result.err != nil {
			s.detailsError = "Unable to load history · r retry"
			break
		}
		byNumber := make(map[int]inspection.Turn)
		for _, t := range s.records {
			byNumber[t.Number] = t
		}
		for _, t := range result.page.Turns {
			byNumber[t.Number] = t.Clone()
		}
		for n, t := range s.liveRecords {
			byNumber[n] = t
		}
		s.records = nil
		for _, t := range byNumber {
			s.records = append(s.records, t)
		}
		sort.Slice(s.records, func(i, j int) bool { return s.records[i].Number < s.records[j].Number })
		s.records = inspection.Retain(s.records)
		if result.page.Logs != nil {
			var logs []string
			seen := make(map[string]bool)
			for _, entry := range result.page.Logs {
				text := entry.At.Format("15:04:05") + "  " + displayText(entry.Text)
				logs = append(logs, text)
				seen[text] = true
			}
			for _, text := range s.logs {
				if !seen[text] {
					logs = append(logs, text)
				}
			}
			s.logs = logs[max(0, len(logs)-maxLogs):]
		}
		s.before = result.page.Before
		s.detailsLoaded = true
		break
	}
	m.trimDetails()
	m.rebuildTree()
	if s := m.selected(); m.activeSession != 0 && s != nil && !s.detailsLoaded && !s.detailsLoading && s.detailsError == "" {
		return m.loadPage(s, 0)
	}
	return nil
}

// Keep at most three inspected sessions in memory. Metadata remains available
// in the overview; reopening an evicted page performs a fresh disk read.
func (m *model) trimDetails() {
	var cached []*session
	for _, s := range m.sessions {
		if s.loadDetails != nil && s.detailsLoaded {
			cached = append(cached, s)
		}
	}
	sort.Slice(cached, func(i, j int) bool { return cached[i].lastViewed < cached[j].lastViewed })
	count := len(cached)
	for _, s := range cached {
		if count <= 3 {
			break
		}
		if s.id == m.activeSession || s.detailsLoading {
			continue
		}
		s.records = nil
		s.logs = nil
		s.liveRecords = nil
		s.detailsLoaded = false
		s.before = 0
		s.loadGeneration++
		count--
	}
}
