package usageuctest

import (
	"sync"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/usageuc"
)

var _ usageuc.Recorder = (*MockRecorder)(nil)

// MockRecorder captures the usages passed to Record.
type MockRecorder struct {
	mu     sync.Mutex
	usages []domain.Usage
}

// Record implements usageuc.Recorder.
func (m *MockRecorder) Record(usage domain.Usage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usages = append(m.usages, usage)
}

// Recorded returns a copy of the captured usages.
func (m *MockRecorder) Recorded() []domain.Usage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.Usage(nil), m.usages...)
}
