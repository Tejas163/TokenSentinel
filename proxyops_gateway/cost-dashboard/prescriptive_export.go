package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

func handleReportDownload(w http.ResponseWriter, r *http.Request, id int, format string) {
	switch format {
	case "csv":
		exportCSV(w, id)
	case "pdf":
		exportPDF(w, id)
	default:
		http.Error(w, "unsupported format", http.StatusBadRequest)
	}
}

func exportCSV(w http.ResponseWriter, assessmentID int) {
	rows, err := db.Query(`SELECT model, provider, current_monthly_cost, projected_monthly_cost,
		input_tokens_millions, output_tokens_millions, scenario
		FROM cost_projections WHERE assessment_id = $1 ORDER BY scenario, current_monthly_cost DESC`, assessmentID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "no data", http.StatusNotFound)
			return
		}
		log.Printf("csv export query error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=assessment-%d-report.csv", assessmentID))

	writer := csv.NewWriter(w)
	writer.Write([]string{"Model", "Provider", "Current Monthly Cost", "Projected Monthly Cost", "Input Tokens (M)", "Output Tokens (M)", "Scenario"})

	for rows.Next() {
		var model, provider, scenario string
		var currentCost, projectedCost, inputM, outputM float64
		if err := rows.Scan(&model, &provider, &currentCost, &projectedCost, &inputM, &outputM, &scenario); err != nil {
			continue
		}
		writer.Write([]string{
			model,
			provider,
			fmt.Sprintf("%.2f", currentCost),
			fmt.Sprintf("%.2f", projectedCost),
			fmt.Sprintf("%.4f", inputM),
			fmt.Sprintf("%.4f", outputM),
			scenario,
		})
	}

	recRows, err := db.Query(`SELECT category, description, current_cost, projected_cost, monthly_savings, payback_period_days, priority
		FROM recommendations WHERE assessment_id = $1 ORDER BY monthly_savings DESC`, assessmentID)
	if err == nil {
		defer recRows.Close()
		writer.Write([]string{})
		writer.Write([]string{"Recommendations"})
		writer.Write([]string{"Category", "Description", "Current Cost", "Projected Cost", "Monthly Savings", "Payback (Days)", "Priority"})
		for recRows.Next() {
			var cat, desc, priority string
			var currentCost, projectedCost, savings float64
			var payback int
			if err := recRows.Scan(&cat, &desc, &currentCost, &projectedCost, &savings, &payback, &priority); err != nil {
				continue
			}
			writer.Write([]string{cat, desc, fmt.Sprintf("%.2f", currentCost), fmt.Sprintf("%.2f", projectedCost), fmt.Sprintf("%.2f", savings), fmt.Sprintf("%d", payback), priority})
		}
	}

	writer.Flush()
}

func exportPDF(w http.ResponseWriter, assessmentID int) {
	report, err := GetReport(assessmentID)
	if err != nil {
		log.Printf("pdf export: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=assessment-%d-report.pdf", assessmentID))
	generatePDF(w, report)
}

func generatePDF(w http.ResponseWriter, report *AssessmentReport) {
	m := maroto.New()

	m.AddRow(20).Add(text.NewCol(12, "TokenSentinel Prescriptive Report", props.Text{
		Style: fontstyle.Bold,
		Size:  16,
		Align: align.Center,
	}))

	m.AddRow(10).Add(text.NewCol(12, fmt.Sprintf("Company: %s", report.Assessment.CompanyName), props.Text{Size: 11}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Version: %d", report.Assessment.Version), props.Text{Size: 10}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Generated: %s", time.Now().UTC().Format("2006-01-02 15:04 UTC")), props.Text{Size: 10}))

	m.AddRow(10).Add(text.NewCol(12, "Executive Summary", props.Text{Style: fontstyle.Bold, Size: 13}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Current Monthly Spend:  $%.2f", report.TotalCurrent), props.Text{Size: 10}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Projected Monthly Spend: $%.2f", report.TotalProjected), props.Text{Size: 10}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Projected Monthly Savings: $%.2f", report.TotalSavings), props.Text{Size: 10}))
	if report.TotalCurrent > 0 {
		m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Savings Rate: %.1f%%", (report.TotalSavings/report.TotalCurrent)*100), props.Text{Size: 10}))
	}

	if len(report.Recommendations) > 0 {
		m.AddRow(10).Add(text.NewCol(12, "Top Recommendation", props.Text{Style: fontstyle.Bold, Size: 13}))
		r := report.Recommendations[0]
		m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("[%s] %s", r.Priority, r.Description), props.Text{Size: 10}))
		m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Monthly Savings: $%.2f", r.MonthlySavings), props.Text{Size: 10}))
	}

	if len(report.CostBreakdown) > 0 {
		m.AddRow(10).Add(text.NewCol(12, "Cost Breakdown by Model", props.Text{Style: fontstyle.Bold, Size: 13}))

		m.AddRows(row.New(10).Add(
			col.New(3).Add(text.New("Model", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Left})),
			col.New(2).Add(text.New("Provider", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Left})),
			col.New(2).Add(text.New("Input (M)", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right})),
			col.New(2).Add(text.New("Output (M)", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right})),
			col.New(2).Add(text.New("Current/Mo", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right})),
			col.New(1).Add(text.New("Proj/Mo", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right})),
		))

		for _, cp := range report.CostBreakdown {
			m.AddRows(row.New(8).Add(
				col.New(3).Add(text.New(cp.Model, props.Text{Size: 8})),
				col.New(2).Add(text.New(cp.Provider, props.Text{Size: 8})),
				col.New(2).Add(text.New(fmt.Sprintf("%.2f", cp.InputTokensMillions), props.Text{Size: 8, Align: align.Right})),
				col.New(2).Add(text.New(fmt.Sprintf("%.2f", cp.OutputTokensMillions), props.Text{Size: 8, Align: align.Right})),
				col.New(2).Add(text.New(fmt.Sprintf("$%.0f", cp.CurrentMonthlyCost), props.Text{Size: 8, Align: align.Right})),
				col.New(1).Add(text.New(fmt.Sprintf("$%.0f", cp.ProjectedMonthlyCost), props.Text{Size: 8, Align: align.Right})),
			))
		}
	}

	document, err := m.Generate()
	if err != nil {
		log.Printf("pdf generation error: %v", err)
		http.Error(w, "pdf generation failed", http.StatusInternalServerError)
		return
	}

	bytes := document.GetBytes()
	w.Write(bytes)
}
