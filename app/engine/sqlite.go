package engine

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db       *sql.DB
	cache    *MemStore
	mu       sync.Mutex
	nextID   int
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	s := &SQLiteStore{
		db:     db,
		cache:  NewMemStore(),
		nextID: 1,
	}

	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := s.loadCache(); err != nil {
		return nil, fmt.Errorf("load cache: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS assessments (
			id INTEGER PRIMARY KEY,
			company_name TEXT NOT NULL DEFAULT '',
			cloud_vendor TEXT NOT NULL DEFAULT '',
			monthly_request_volume INTEGER NOT NULL DEFAULT 0,
			input_pct REAL NOT NULL DEFAULT 0.7,
			output_pct REAL NOT NULL DEFAULT 0.3,
			current_monthly_spend REAL NOT NULL DEFAULT 0,
			source TEXT NOT NULL DEFAULT '',
			currency TEXT NOT NULL DEFAULT 'USD',
			fx_rate REAL NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			team_developers INTEGER NOT NULL DEFAULT 0,
			team_platform_engineers INTEGER NOT NULL DEFAULT 0,
			team_devops INTEGER NOT NULL DEFAULT 0,
			team_management INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS cost_projections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL,
			model TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			current_monthly_cost REAL NOT NULL DEFAULT 0,
			projected_monthly_cost REAL NOT NULL DEFAULT 0,
			input_tokens_millions REAL NOT NULL DEFAULT 0,
			output_tokens_millions REAL NOT NULL DEFAULT 0,
			scenario TEXT NOT NULL DEFAULT 'base',
			created_at TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(assessment_id) REFERENCES assessments(id)
		);
		CREATE TABLE IF NOT EXISTS recommendations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL,
			category TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			current_cost REAL NOT NULL DEFAULT 0,
			projected_cost REAL NOT NULL DEFAULT 0,
			monthly_savings REAL NOT NULL DEFAULT 0,
			payback_period_days INTEGER NOT NULL DEFAULT 0,
			priority TEXT NOT NULL DEFAULT 'medium',
			created_at TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(assessment_id) REFERENCES assessments(id)
		);
		CREATE TABLE IF NOT EXISTS live_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			total_monthly_cost REAL NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS live_data_models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			live_data_id INTEGER NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			request_count INTEGER NOT NULL DEFAULT 1,
			actual_cost REAL NOT NULL DEFAULT 0,
			FOREIGN KEY(live_data_id) REFERENCES live_data(id)
		);
		CREATE TABLE IF NOT EXISTS assessment_providers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			monthly_spend REAL NOT NULL DEFAULT 0,
			FOREIGN KEY(assessment_id) REFERENCES assessments(id)
		);
		CREATE TABLE IF NOT EXISTS assessment_provider_models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL,
			model TEXT NOT NULL,
			FOREIGN KEY(provider_id) REFERENCES assessment_providers(id)
		);
		CREATE TABLE IF NOT EXISTS assessment_gpu_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL,
			gpu_type TEXT NOT NULL DEFAULT '',
			count INTEGER NOT NULL DEFAULT 0,
			region TEXT NOT NULL DEFAULT '',
			hourly_price REAL NOT NULL DEFAULT 0,
			reserved INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(assessment_id) REFERENCES assessments(id)
		);
	`)
	return err
}

func (s *SQLiteStore) loadCache() error {
	rows, err := s.db.Query(`SELECT id FROM assessments ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return err
		}
		a, err := s.loadAssessment(id)
		if err != nil {
			return err
		}
		s.cache.AddAssessment(a)
		if id >= s.nextID {
			s.nextID = id + 1
		}

		projections, err := s.loadProjections(id)
		if err != nil {
			return err
		}
		if len(projections) > 0 {
			s.cache.ReplaceCostProjections(id, projections)
		}

		recs, err := s.loadRecommendations(id)
		if err != nil {
			return err
		}
		if len(recs) > 0 {
			s.cache.ReplaceRecommendations(id, recs)
		}
	}

	return rows.Err()
}

func (s *SQLiteStore) loadAssessment(id int) (*Assessment, error) {
	row := s.db.QueryRow(`
		SELECT id, company_name, cloud_vendor, monthly_request_volume,
		       input_pct, output_pct, current_monthly_spend, source,
		       currency, fx_rate, version,
		       team_developers, team_platform_engineers, team_devops, team_management,
		       created_at, updated_at
		FROM assessments WHERE id = ?`, id)

	a := &Assessment{}
	var fxRate sql.NullFloat64
	var teamDev, teamPlat, teamDevops, teamMgmt int
	err := row.Scan(&a.ID, &a.CompanyName, &a.CloudVendor, &a.MonthlyRequestVolume,
		&a.TokenDistribution.InputPct, &a.TokenDistribution.OutputPct,
		&a.CurrentMonthlySpend, &a.Source,
		&a.Currency, &fxRate, &a.Version,
		&teamDev, &teamPlat, &teamDevops, &teamMgmt,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if fxRate.Valid {
		a.FXRate = fxRate.Float64
	}
	a.TeamComposition = TeamComposition{
		Developers:        teamDev,
		PlatformEngineers: teamPlat,
		DevOps:            teamDevops,
		Management:        teamMgmt,
	}

	provRows, err := s.db.Query(`SELECT id, name, monthly_spend FROM assessment_providers WHERE assessment_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer provRows.Close()

	for provRows.Next() {
		pu := ProviderUsage{}
		var provID int
		if err := provRows.Scan(&provID, &pu.Name, &pu.MonthlySpend); err != nil {
			return nil, err
		}
		modelRows, err := s.db.Query(`SELECT model FROM assessment_provider_models WHERE provider_id = ?`, provID)
		if err != nil {
			return nil, err
		}
		for modelRows.Next() {
			var m string
			if err := modelRows.Scan(&m); err != nil {
				modelRows.Close()
				return nil, err
			}
			pu.Models = append(pu.Models, m)
		}
		modelRows.Close()
		a.ProvidersUsed = append(a.ProvidersUsed, pu)
	}

	gpuRows, err := s.db.Query(`SELECT gpu_type, count, region, hourly_price, reserved FROM assessment_gpu_configs WHERE assessment_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer gpuRows.Close()

	for gpuRows.Next() {
		g := GPUConfig{}
		var reserved int
		if err := gpuRows.Scan(&g.Type, &g.Count, &g.Region, &g.HourlyPrice, &reserved); err != nil {
			return nil, err
		}
		g.Reserved = reserved != 0
		a.GPUConfigs = append(a.GPUConfigs, g)
	}

	return a, nil
}

func (s *SQLiteStore) loadProjections(assessmentID int) ([]CostProjection, error) {
	rows, err := s.db.Query(`
		SELECT model, provider, current_monthly_cost, projected_monthly_cost,
		       input_tokens_millions, output_tokens_millions, scenario
		FROM cost_projections WHERE assessment_id = ?`, assessmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projections []CostProjection
	for rows.Next() {
		cp := CostProjection{AssessmentID: assessmentID}
		if err := rows.Scan(&cp.Model, &cp.Provider, &cp.CurrentMonthlyCost,
			&cp.ProjectedMonthlyCost, &cp.InputTokensMillions, &cp.OutputTokensMillions, &cp.Scenario); err != nil {
			return nil, err
		}
		projections = append(projections, cp)
	}
	return projections, rows.Err()
}

func (s *SQLiteStore) loadRecommendations(assessmentID int) ([]Recommendation, error) {
	rows, err := s.db.Query(`
		SELECT category, description, current_cost, projected_cost,
		       monthly_savings, payback_period_days, priority
		FROM recommendations WHERE assessment_id = ?`, assessmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		r := Recommendation{AssessmentID: assessmentID}
		if err := rows.Scan(&r.Category, &r.Description, &r.CurrentCost,
			&r.ProjectedCost, &r.MonthlySavings, &r.PaybackPeriodDays, &r.Priority); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

func (s *SQLiteStore) saveAssessment(a *Assessment) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO assessments (id, company_name, cloud_vendor, monthly_request_volume,
			input_pct, output_pct, current_monthly_spend, source,
			currency, fx_rate, version,
			team_developers, team_platform_engineers, team_devops, team_management,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			company_name=excluded.company_name, cloud_vendor=excluded.cloud_vendor,
			monthly_request_volume=excluded.monthly_request_volume,
			input_pct=excluded.input_pct, output_pct=excluded.output_pct,
			current_monthly_spend=excluded.current_monthly_spend, source=excluded.source,
			currency=excluded.currency, fx_rate=excluded.fx_rate, version=excluded.version,
			team_developers=excluded.team_developers, team_platform_engineers=excluded.team_platform_engineers,
			team_devops=excluded.team_devops, team_management=excluded.team_management,
			updated_at=excluded.updated_at`,
		a.ID, a.CompanyName, a.CloudVendor, a.MonthlyRequestVolume,
		a.TokenDistribution.InputPct, a.TokenDistribution.OutputPct,
		a.CurrentMonthlySpend, a.Source,
		a.Currency, a.FXRate, a.Version,
		a.TeamComposition.Developers, a.TeamComposition.PlatformEngineers,
		a.TeamComposition.DevOps, a.TeamComposition.Management,
		a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return err
	}

	tx.Exec(`DELETE FROM assessment_gpu_configs WHERE assessment_id = ?`, a.ID)
	for _, gpu := range a.GPUConfigs {
		reserved := 0
		if gpu.Reserved {
			reserved = 1
		}
		_, err = tx.Exec(`INSERT INTO assessment_gpu_configs (assessment_id, gpu_type, count, region, hourly_price, reserved) VALUES (?, ?, ?, ?, ?, ?)`,
			a.ID, gpu.Type, gpu.Count, gpu.Region, gpu.HourlyPrice, reserved)
		if err != nil {
			return err
		}
	}

	tx.Exec(`DELETE FROM assessment_providers WHERE assessment_id = ?`, a.ID)
	for _, pu := range a.ProvidersUsed {
		res, err := tx.Exec(`INSERT INTO assessment_providers (assessment_id, name, monthly_spend) VALUES (?, ?, ?)`,
			a.ID, pu.Name, pu.MonthlySpend)
		if err != nil {
			return err
		}
		provID, _ := res.LastInsertId()
		for _, m := range pu.Models {
			_, err = tx.Exec(`INSERT INTO assessment_provider_models (provider_id, model) VALUES (?, ?)`, provID, m)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) saveProjections(assessmentID int, projections []CostProjection) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec(`DELETE FROM cost_projections WHERE assessment_id = ?`, assessmentID)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, cp := range projections {
		_, err := tx.Exec(`
			INSERT INTO cost_projections (assessment_id, model, provider, current_monthly_cost,
				projected_monthly_cost, input_tokens_millions, output_tokens_millions, scenario, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			assessmentID, cp.Model, cp.Provider, cp.CurrentMonthlyCost,
			cp.ProjectedMonthlyCost, cp.InputTokensMillions, cp.OutputTokensMillions, cp.Scenario, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) saveRecommendations(assessmentID int, recs []Recommendation) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec(`DELETE FROM recommendations WHERE assessment_id = ?`, assessmentID)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range recs {
		_, err := tx.Exec(`
			INSERT INTO recommendations (assessment_id, category, description, current_cost,
				projected_cost, monthly_savings, payback_period_days, priority, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			assessmentID, r.Category, r.Description, r.CurrentCost,
			r.ProjectedCost, r.MonthlySavings, r.PaybackPeriodDays, r.Priority, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) AddAssessment(a *Assessment) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++
	a.ID = id
	now := time.Now().UTC().Format(time.RFC3339)
	a.CreatedAt = now
	a.UpdatedAt = now

	if err := s.saveAssessment(a); err != nil {
		panic(fmt.Sprintf("sqlite save assessment: %v", err))
	}

	s.cache.AddAssessment(a)
	return id
}

func (s *SQLiteStore) GetAssessment(id int) (*Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	a, err := s.cache.GetAssessment(id)
	if err == nil {
		return a, nil
	}

	a, err = s.loadAssessment(id)
	if err != nil {
		return nil, fmt.Errorf("assessment %d not found", id)
	}
	return a, nil
}

func (s *SQLiteStore) QueryLiveCostData(since time.Time) (*AssessmentLiveData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cache.QueryLiveCostData(since)
}

func (s *SQLiteStore) SetLiveData(ld *AssessmentLiveData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache.SetLiveData(ld)
}

func (s *SQLiteStore) ReplaceCostProjections(assessmentID int, projections []CostProjection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.saveProjections(assessmentID, projections); err != nil {
		return err
	}

	return s.cache.ReplaceCostProjections(assessmentID, projections)
}

func (s *SQLiteStore) ReplaceRecommendations(assessmentID int, recs []Recommendation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.saveRecommendations(assessmentID, recs); err != nil {
		return err
	}

	return s.cache.ReplaceRecommendations(assessmentID, recs)
}

func (s *SQLiteStore) GetCostProjections(assessmentID int, scenario string) ([]CostProjection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	projections, err := s.cache.GetCostProjections(assessmentID, scenario)
	if err == nil && projections != nil {
		return projections, nil
	}

	return s.loadProjections(assessmentID)
}

func (s *SQLiteStore) GetRecommendations(assessmentID int) ([]Recommendation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	recs, err := s.cache.GetRecommendations(assessmentID)
	if err == nil && recs != nil {
		return recs, nil
	}

	return s.loadRecommendations(assessmentID)
}

func (s *SQLiteStore) InsertCostProjections(assessmentID int, projections []CostProjection, scenario string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.saveProjections(assessmentID, projections); err != nil {
		return err
	}

	return s.cache.InsertCostProjections(assessmentID, projections, scenario)
}
