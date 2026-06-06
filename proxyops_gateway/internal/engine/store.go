package engine

import "time"

type Store interface {
	GetAssessment(id int) (*Assessment, error)
	QueryLiveCostData(since time.Time) (*AssessmentLiveData, error)
	ReplaceCostProjections(assessmentID int, projections []CostProjection) error
	ReplaceRecommendations(assessmentID int, recs []Recommendation) error
	GetCostProjections(assessmentID int, scenario string) ([]CostProjection, error)
	GetRecommendations(assessmentID int) ([]Recommendation, error)
	InsertCostProjections(assessmentID int, projections []CostProjection, scenario string) error
}
