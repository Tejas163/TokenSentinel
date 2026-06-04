package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHandleAdminSeed_methodNotAllowed(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	w := httptest.NewRecorder()
	handleAdminSeed(w, request("GET", "/api/admin/seed-demo"))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleAdminSeed_existingAssessment(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	seedLimiterMu.Lock()
	seedLimits = make(map[string]time.Time)
	seedLimiterMu.Unlock()

	appStore = newMemStore()
	aid := appStore.(*memStore).addAssessment(&Assessment{
		CompanyName:          demoAssessmentName,
		CloudVendor:          "aws",
		MonthlyRequestVolume: 1000000,
		GPUConfigs: []GPUConfig{
			{Type: "A100", Count: 4, Region: "us-east-1", HourlyPrice: 3.50, Reserved: true},
			{Type: "H100", Count: 2, Region: "us-west-2", HourlyPrice: 4.50, Reserved: false},
		},
		TokenDistribution:   TokenDistribution{InputPct: 0.75, OutputPct: 0.25},
		CurrentMonthlySpend: 12000,
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"}, MonthlySpend: 7000},
			{Name: "anthropic", Models: []string{"claude-3-opus", "claude-3-sonnet"}, MonthlySpend: 3000},
			{Name: "self-hosted", Models: []string{"llama-3-70b", "mixtral-8x7b"}, MonthlySpend: 2000},
		},
		TeamComposition: TeamComposition{Developers: 20, PlatformEngineers: 3, DevOps: 2, Management: 2},
		Source:          "manual",
	})

	mock.ExpectQuery("SELECT id FROM assessments WHERE company_name").
		WithArgs(demoAssessmentName).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(aid))

	origRNG := seedRNG
	seedRNG = rand.New(rand.NewSource(42))
	defer func() { seedRNG = origRNG }()

	n := len(generateSeedRows())

	seedRNG = rand.New(rand.NewSource(42))
	defer func() { seedRNG = origRNG }()

	mock.ExpectExec("INSERT INTO cost_entries").
		WillReturnResult(sqlmock.NewResult(0, int64(n)))

	mock.ExpectQuery("SELECT id FROM monitoring_rules WHERE model").
		WithArgs("*").
		WillReturnError(fmt.Errorf("not found"))

	mock.ExpectExec("INSERT INTO monitoring_rules").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery("SELECT id FROM budget_rules WHERE model").
		WithArgs("*").
		WillReturnError(fmt.Errorf("not found"))

	mock.ExpectExec("INSERT INTO budget_rules").
		WillReturnResult(sqlmock.NewResult(1, 1))

	events = newSSEBroker()

	w := httptest.NewRecorder()
	handleAdminSeed(w, withKey(request("POST", "/api/admin/seed-demo"), "test-key-123"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp seedDemoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.AssessmentID != aid {
		t.Fatalf("expected assessment_id %d, got %d", aid, resp.AssessmentID)
	}
	if resp.EntriesAdded != n {
		t.Fatalf("expected %d entries, got %d", n, resp.EntriesAdded)
	}
	if !resp.AlreadyExists {
		t.Fatal("expected already_exists=true for existing assessment")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestHandleAdminSeed_newAssessment(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	seedLimiterMu.Lock()
	seedLimits = make(map[string]time.Time)
	seedLimiterMu.Unlock()

	mock.ExpectQuery("SELECT id FROM assessments WHERE company_name").
		WithArgs(demoAssessmentName).
		WillReturnError(fmt.Errorf("not found"))

	mock.ExpectQuery("INSERT INTO assessments").
		WithArgs("DemoCorp", "aws", sqlmock.AnyArg(), 1000000, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "manual", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(99))

	appStore = newMemStore()

	origRNG := seedRNG
	seedRNG = rand.New(rand.NewSource(99))
	defer func() { seedRNG = origRNG }()

	n := len(generateSeedRows())

	seedRNG = rand.New(rand.NewSource(99))
	defer func() { seedRNG = origRNG }()

	mock.ExpectExec("INSERT INTO cost_entries").
		WillReturnResult(sqlmock.NewResult(0, int64(n)))

	mock.ExpectQuery("SELECT id FROM monitoring_rules WHERE model").
		WithArgs("*").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))

	mock.ExpectQuery("SELECT id FROM budget_rules WHERE model").
		WithArgs("*").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))

	events = newSSEBroker()

	w := httptest.NewRecorder()
	handleAdminSeed(w, withKey(request("POST", "/api/admin/seed-demo"), "test-key-123"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp seedDemoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.AssessmentID != 99 {
		t.Fatalf("expected assessment_id 99, got %d", resp.AssessmentID)
	}
	if resp.EntriesAdded != n {
		t.Fatalf("expected %d entries, got %d", n, resp.EntriesAdded)
	}
	if resp.AlreadyExists {
		t.Fatal("expected already_exists=false for new assessment")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}
