package engine

import (
	"fmt"
	"sync"
	"time"
)

type MemStore struct {
	mu               sync.Mutex
	assessments      map[int]*Assessment
	costProjections  map[int][]CostProjection
	recommendations  map[int][]Recommendation
	liveData         *AssessmentLiveData
	nextAssessmentID int
}

func NewMemStore() *MemStore {
	return &MemStore{
		assessments:      make(map[int]*Assessment),
		costProjections:  make(map[int][]CostProjection),
		recommendations:  make(map[int][]Recommendation),
		nextAssessmentID: 1,
	}
}

func (s *MemStore) AddAssessment(a *Assessment) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextAssessmentID
	s.nextAssessmentID++
	a.ID = id
	s.assessments[id] = a
	return id
}

func (s *MemStore) SetLiveData(ld *AssessmentLiveData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.liveData = ld
}

func (s *MemStore) GetAssessment(id int) (*Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assessments[id]
	if !ok {
		return nil, fmt.Errorf("assessment %d not found", id)
	}
	cp := *a
	return &cp, nil
}

func (s *MemStore) QueryLiveCostData(since time.Time) (*AssessmentLiveData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.liveData == nil {
		return &AssessmentLiveData{Models: make(map[string]*ModelUsage)}, nil
	}
	cp := *s.liveData
	cp.Models = make(map[string]*ModelUsage)
	for k, v := range s.liveData.Models {
		uv := *v
		cp.Models[k] = &uv
	}
	return &cp, nil
}

func (s *MemStore) ReplaceCostProjections(assessmentID int, projections []CostProjection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]CostProjection, len(projections))
	copy(cp, projections)
	s.costProjections[assessmentID] = cp
	return nil
}

func (s *MemStore) ReplaceRecommendations(assessmentID int, recs []Recommendation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]Recommendation, len(recs))
	copy(cp, recs)
	s.recommendations[assessmentID] = cp
	return nil
}

func (s *MemStore) GetCostProjections(assessmentID int, scenario string) ([]CostProjection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	projs, ok := s.costProjections[assessmentID]
	if !ok {
		return nil, nil
	}
	cp := make([]CostProjection, len(projs))
	copy(cp, projs)
	return cp, nil
}

func (s *MemStore) GetRecommendations(assessmentID int) ([]Recommendation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs, ok := s.recommendations[assessmentID]
	if !ok {
		return nil, nil
	}
	cp := make([]Recommendation, len(recs))
	copy(cp, recs)
	return cp, nil
}

func (s *MemStore) InsertCostProjections(assessmentID int, projections []CostProjection, scenario string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.costProjections[assessmentID]
	for _, p := range projections {
		cp := p
		cp.AssessmentID = assessmentID
		cp.Scenario = scenario
		existing = append(existing, cp)
	}
	s.costProjections[assessmentID] = existing
	return nil
}
