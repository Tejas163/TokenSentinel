package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	report, err := GetReport(appStore, assessmentID)
	if err != nil {
		log.Printf("csv export: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=TokenSentinel_Assessment_%d_Report.csv", assessmentID))

	writer := csv.NewWriter(w)
	writer.Write([]string{fmt.Sprintf("TokenSentinel Prescriptive Assessment Report — %s", report.Assessment.CompanyName)})
	writer.Write([]string{fmt.Sprintf("Generated: %s", time.Now().UTC().Format("January 02, 2006 15:04 UTC"))})
	writer.Write([]string{fmt.Sprintf("Version: %d", report.Assessment.Version)})
	writer.Write([]string{})

	writer.Write([]string{"EXECUTIVE SUMMARY"})
	writer.Write([]string{"Metric", "Value"})
	writer.Write([]string{"Current Monthly Spend", fmt.Sprintf("$%.2f", report.TotalCurrent)})
	writer.Write([]string{"Projected Monthly Spend", fmt.Sprintf("$%.2f", report.TotalProjected)})
	writer.Write([]string{"Projected Monthly Savings", fmt.Sprintf("$%.2f", report.TotalSavings)})
	if report.TotalCurrent > 0 {
		writer.Write([]string{"Savings Rate", fmt.Sprintf("%.1f%%", (report.TotalSavings/report.TotalCurrent)*100)})
	}
	writer.Write([]string{})

	writer.Write([]string{"COST BREAKDOWN BY MODEL"})
	writer.Write([]string{"Model", "Provider", "Input Tokens (M)", "Output Tokens (M)", "Current Monthly Cost", "Projected Monthly Cost"})
	for _, cp := range report.CostBreakdown {
		writer.Write([]string{
			cp.Model,
			cp.Provider,
			fmt.Sprintf("%.2f", cp.InputTokensMillions),
			fmt.Sprintf("%.2f", cp.OutputTokensMillions),
			fmt.Sprintf("%.2f", cp.CurrentMonthlyCost),
			fmt.Sprintf("%.2f", cp.ProjectedMonthlyCost),
		})
	}
	writer.Write([]string{})

	writer.Write([]string{"RECOMMENDATIONS"})
	writer.Write([]string{"Priority", "Category", "Description", "Current Cost", "Projected Cost", "Monthly Savings", "Payback Period (Days)"})
	for _, r := range report.Recommendations {
		writer.Write([]string{
			r.Priority,
			r.Category,
			r.Description,
			fmt.Sprintf("%.2f", r.CurrentCost),
			fmt.Sprintf("%.2f", r.ProjectedCost),
			fmt.Sprintf("%.2f", r.MonthlySavings),
			fmt.Sprintf("%d", r.PaybackPeriodDays),
		})
	}
	writer.Write([]string{})

	if len(report.Recommendations) > 0 {
		totalSavings := 0.0
		for _, r := range report.Recommendations {
			totalSavings += r.MonthlySavings
		}
		writer.Write([]string{"Total Potential Monthly Savings from Recommendations", fmt.Sprintf("$%.2f", totalSavings)})
	}

	writer.Flush()
}

func exportPDF(w http.ResponseWriter, assessmentID int) {
	report, err := GetReport(appStore, assessmentID)
	if err != nil {
		log.Printf("pdf export: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=TokenSentinel_Assessment_%d_Report.pdf", assessmentID))
	generatePDF(w, report)
}

func generatePDF(w http.ResponseWriter, report *AssessmentReport) {
	m := maroto.New()

	m.AddRow(20).Add(text.NewCol(12, "TokenSentinel Prescriptive Assessment Report", props.Text{
		Style: fontstyle.Bold,
		Size:  18,
		Align: align.Center,
	}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Prepared for: %s", report.Assessment.CompanyName), props.Text{
		Size: 12, Align: align.Center,
	}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Report Date: %s", time.Now().UTC().Format("January 02, 2006")), props.Text{
		Size: 10, Align: align.Center,
	}))
	m.AddRow(12).Add(text.NewCol(12, "", props.Text{}))

	if report.TotalCurrent > 0 {
		savingsRate := (report.TotalSavings / report.TotalCurrent) * 100
		m.AddRow(20).Add(text.NewCol(12, fmt.Sprintf("Total Potential Savings: $%.0f/mo (%.0f%%)",
			report.TotalSavings, savingsRate), props.Text{
			Style: fontstyle.Bold,
			Size:  14,
			Align: align.Center,
		}))
		m.AddRow(12).Add(text.NewCol(12, "", props.Text{}))
	}

	m.AddRow(12).Add(text.NewCol(12, "1. Executive Summary", props.Text{Style: fontstyle.Bold, Size: 14}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Your current monthly AI infrastructure spend is $%.2f.", report.TotalCurrent), props.Text{Size: 10}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("After applying the recommended optimizations in this report, your projected monthly spend is $%.2f.", report.TotalProjected), props.Text{Size: 10}))
	m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("This represents a potential savings of $%.2f per month.", report.TotalSavings), props.Text{Size: 10}))
	if report.TotalCurrent > 0 {
		m.AddRow(8).Add(text.NewCol(12, fmt.Sprintf("Your estimated savings rate is %.1f%%, meaning you could reduce your AI costs by nearly %.0f%% with the changes outlined below.",
			(report.TotalSavings/report.TotalCurrent)*100, (report.TotalSavings/report.TotalCurrent)*100), props.Text{Size: 10}))
	}
	m.AddRow(10).Add(text.NewCol(12, "", props.Text{}))

	if len(report.Recommendations) > 0 {
		m.AddRow(12).Add(text.NewCol(12, "2. Recommendations", props.Text{Style: fontstyle.Bold, Size: 14}))
		m.AddRow(8).Add(text.NewCol(12, "The following recommendations are ranked by potential impact. Each includes an estimated monthly savings and a payback period indicating how quickly the change pays for itself.", props.Text{Size: 10}))

		for i, r := range report.Recommendations {
			if i > 0 {
				m.AddRow(4).Add(text.NewCol(12, "", props.Text{}))
			}
			m.AddRows(row.New(10).Add(
				col.New(1).Add(text.New(fmt.Sprintf("%d.", i+1), props.Text{Style: fontstyle.Bold, Size: 10})),
				col.New(2).Add(text.New(strings.ToUpper(r.Priority), props.Text{
					Style: fontstyle.Bold, Size: 9,
				})),
				col.New(9).Add(text.New(fmt.Sprintf("$%.0f/mo savings", r.MonthlySavings), props.Text{
					Size: 10,
				})),
			))
			m.AddRows(row.New(14).Add(
				col.New(1).Add(text.New("", props.Text{Size: 8})),
				col.New(11).Add(text.New(r.Description, props.Text{Size: 10})),
			))
			m.AddRows(row.New(8).Add(
				col.New(1).Add(text.New("", props.Text{Size: 8})),
				col.New(11).Add(text.New(fmt.Sprintf("Category: %s | Current spend: $%.0f/mo | Payback: %d days",
					strings.ReplaceAll(r.Category, "_", " "), r.CurrentCost, r.PaybackPeriodDays), props.Text{Size: 8})),
			))
		}
		m.AddRow(10).Add(text.NewCol(12, "", props.Text{}))
	}

	if len(report.CostBreakdown) > 0 {
		m.AddRow(12).Add(text.NewCol(12, "3. Cost Breakdown by Model", props.Text{Style: fontstyle.Bold, Size: 14}))
		m.AddRow(8).Add(text.NewCol(12, "The table below shows your current spending by model and what each line item would cost after implementing all recommendations.", props.Text{Size: 10}))

		m.AddRows(row.New(10).Add(
			col.New(3).Add(text.New("Model", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Left})),
			col.New(2).Add(text.New("Provider", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Left})),
			col.New(2).Add(text.New("Input (M tokens)", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right})),
			col.New(2).Add(text.New("Output (M tokens)", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right})),
			col.New(2).Add(text.New("Current/Mo", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right})),
			col.New(1).Add(text.New("Projected", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right})),
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
		m.AddRow(10).Add(text.NewCol(12, "", props.Text{}))
	}

	m.AddRow(12).Add(text.NewCol(12, "4. Next Steps", props.Text{Style: fontstyle.Bold, Size: 14}))
	m.AddRow(8).Add(text.NewCol(12, "1. Review each recommendation with your engineering team to assess feasibility.", props.Text{Size: 10}))
	m.AddRow(8).Add(text.NewCol(12, "2. Start with high-priority items that offer the fastest payback period.", props.Text{Size: 10}))
	m.AddRow(8).Add(text.NewCol(12, "3. Use the What-If Simulator in your dashboard to model additional scenarios.", props.Text{Size: 10}))
	m.AddRow(8).Add(text.NewCol(12, "4. Re-run this assessment after implementing changes to track your savings.", props.Text{Size: 10}))
	m.AddRow(8).Add(text.NewCol(12, "5. Contact TokenSentinel support for help with implementation.", props.Text{Size: 10}))

	m.AddRow(20).Add(text.NewCol(12, fmt.Sprintf("TokenSentinel — %s", time.Now().UTC().Format("2006")), props.Text{
		Size: 8, Align: align.Center,
	}))

	document, err := m.Generate()
	if err != nil {
		log.Printf("pdf generation error: %v", err)
		http.Error(w, "pdf generation failed", http.StatusInternalServerError)
		return
	}

	bytes := document.GetBytes()
	w.Write(bytes)
}
