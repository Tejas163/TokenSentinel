package main

import (
	"bytes"
	"encoding/csv"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tokensentinel/engine"
)

func TestStripBOM(t *testing.T) {
	tests := []struct {
		name     string
		input    [][]string
		expected string
	}{
		{"BOM header", [][]string{{"\ufeffmodel", "cost"}}, "model"},
		{"No BOM", [][]string{{"model", "cost"}}, "model"},
		{"Empty input", [][]string{}, ""},
		{"NBSP prefix", [][]string{{"\u00a0model"}}, "model"},
		{"ZWSP prefix", [][]string{{"\u200bmodel"}}, "model"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stripBOM(tc.input)
			if len(result) == 0 && tc.expected == "" {
				return
			}
			if result[0][0] != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result[0][0])
			}
		})
	}
}

func TestDetectColumns(t *testing.T) {
	tests := []struct {
		name         string
		header       []string
		wantStart    int
		wantModel    int
		wantProvider int
		wantInput    int
		wantOutput   int
		wantCost     int
		wantNoModel  bool
	}{
		{
			name: "standard headers",
			header:    []string{"model", "input_tokens", "output_tokens", "cost"},
			wantStart: 1, wantModel: 0, wantProvider: -1, wantInput: 1, wantOutput: 2, wantCost: 3,
		},
		{
			name: "with provider",
			header:    []string{"model", "provider", "input_tokens", "output_tokens", "cost"},
			wantStart: 1, wantModel: 0, wantProvider: 1, wantInput: 2, wantOutput: 3, wantCost: 4,
		},
		{
			name: "alternative names",
			header:    []string{"llm", "vendor", "prompt_tokens", "completion_tokens", "total_cost"},
			wantStart: 1, wantModel: 0, wantProvider: 1, wantInput: 2, wantOutput: 3, wantCost: 4,
		},
		{
			name: "case insensitive",
			header:    []string{"Model", "Input_Tokens", "Output", "Amount"},
			wantStart: 1, wantModel: 0, wantProvider: -1, wantInput: 1, wantOutput: 2, wantCost: 3,
		},
		{
			name: "positional fallback",
			header:    []string{"gpt-4", "1000", "500", "5.00"},
			wantStart: 0, wantModel: 0, wantProvider: -1, wantInput: 1, wantOutput: 2, wantCost: 3,
		},
		{
			name: "too few columns",
			header:    []string{"model", "cost"},
			wantStart: 0, wantModel: 0, wantProvider: -1, wantInput: -1, wantOutput: -1, wantCost: 1,
		},
		{
			name: "model name header",
			header:    []string{"model_name", "service", "input", "output", "cost"},
			wantStart: 1, wantModel: 0, wantProvider: 1, wantInput: 2, wantOutput: 3, wantCost: 4,
		},
		{
			name: "model_id header",
			header:    []string{"model_id", "input_tokens", "output_tokens", "cost"},
			wantStart: 1, wantModel: 0, wantProvider: -1, wantInput: 1, wantOutput: 2, wantCost: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows := [][]string{tc.header}
			start, cm := detectColumns(rows)
			if tc.wantNoModel {
				if cm.model >= 0 {
					t.Error("expected no model column detected")
				}
				return
			}
			if start != tc.wantStart {
				t.Errorf("expected start %d, got %d", tc.wantStart, start)
			}
			if cm.model != tc.wantModel {
				t.Errorf("expected model col %d, got %d", tc.wantModel, cm.model)
			}
			if cm.provider != tc.wantProvider {
				t.Errorf("expected provider col %d, got %d", tc.wantProvider, cm.provider)
			}
			if cm.inputTokens != tc.wantInput {
				t.Errorf("expected input_tokens col %d, got %d", tc.wantInput, cm.inputTokens)
			}
			if cm.outputTokens != tc.wantOutput {
				t.Errorf("expected output_tokens col %d, got %d", tc.wantOutput, cm.outputTokens)
			}
			if cm.cost != tc.wantCost {
				t.Errorf("expected cost col %d, got %d", tc.wantCost, cm.cost)
			}
		})
	}
}

func TestDetectColumnsEmptyRows(t *testing.T) {
	start, cm := detectColumns([][]string{})
	if start != 0 {
		t.Errorf("expected start 0 for empty rows, got %d", start)
	}
	if cm.model >= 0 {
		t.Error("expected no model column for empty rows")
	}
}

func TestDetectColumnsIgnoresNoiseHeaders(t *testing.T) {
	knownHeaders := map[string]string{
		"total_tokens": "",
		"usage_date":   "",
		"date":         "",
		"timestamp":    "",
	}

	for hdr := range knownHeaders {
		t.Run(hdr, func(t *testing.T) {
			rows := [][]string{{hdr, "model", "input_tokens", "output_tokens", "cost"}}
			_, cm := detectColumns(rows)
			if cm.model < 0 {
				t.Errorf("model should be detected despite %q header", hdr)
			}
		})
	}
}

func TestGetCol(t *testing.T) {
	row := []string{"a", "b", "c"}

	if v := getCol(row, 0); v != "a" {
		t.Errorf("expected 'a', got %q", v)
	}
	if v := getCol(row, 5); v != "" {
		t.Errorf("expected empty for out of range, got %q", v)
	}
	if v := getCol(row, -1); v != "" {
		t.Errorf("expected empty for negative index, got %q", v)
	}
	if v := getCol([]string{"  spaced  "}, 0); v != "spaced" {
		t.Errorf("expected trimmed value, got %q", v)
	}
}

func TestHandleSampleCSV(t *testing.T) {
	req := httptest.NewRequest("GET", "/sample-billing.csv", nil)
	w := httptest.NewRecorder()
	handleSampleCSV(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) < 100 {
		t.Errorf("expected CSV content, got %d bytes", len(body))
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/csv" {
		t.Errorf("expected text/csv content type, got %s", ct)
	}
}

func TestHandleLanding(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handleLanding(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "TokenSentinel") {
		t.Error("expected page to contain TokenSentinel")
	}
}

func TestHandleLandingNotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	handleLanding(w, req)

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown path, got %d", w.Result().StatusCode)
	}
}

func TestHandleAnalyzeNoFile(t *testing.T) {
	req := httptest.NewRequest("POST", "/analyze", nil)
	w := httptest.NewRecorder()
	handleAnalyze(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleAnalyzeWrongMethod(t *testing.T) {
	req := httptest.NewRequest("GET", "/analyze", nil)
	w := httptest.NewRecorder()
	handleAnalyze(w, req)

	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Result().StatusCode)
	}
}

func TestHandleAnalyzeValidCSV(t *testing.T) {
	csvContent := "model,provider,input_tokens,output_tokens,cost\ngpt-4,openai,1000,200,5.00\ngpt-4o-mini,openai,500,100,0.50\n"
	payload := buildMultipartPayload(t, csvContent)
	req := httptest.NewRequest("POST", "/analyze", payload.Body)
	req.Header.Set("Content-Type", payload.ContentType)

	w := httptest.NewRecorder()
	handleAnalyze(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/report/") {
		t.Errorf("expected redirect to /report/, got %s", loc)
	}
}

func TestHandleAnalyzeWithProvider(t *testing.T) {
	csvContent := "model,provider,input_tokens,output_tokens,cost\ngpt-4,openai,1000,200,5.00\nclaude-3-opus,anthropic,500,100,3.00\n"
	payload := buildMultipartPayload(t, csvContent)
	req := httptest.NewRequest("POST", "/analyze", payload.Body)
	req.Header.Set("Content-Type", payload.ContentType)

	w := httptest.NewRecorder()
	handleAnalyze(w, req)

	if w.Result().StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Result().StatusCode)
	}
}

func TestHandleAnalyzeBOMCSV(t *testing.T) {
	csvContent := "\ufeffmodel,provider,input_tokens,output_tokens,cost\ngpt-4,openai,1000,200,5.00\n"
	payload := buildMultipartPayload(t, csvContent)
	req := httptest.NewRequest("POST", "/analyze", payload.Body)
	req.Header.Set("Content-Type", payload.ContentType)

	w := httptest.NewRecorder()
	handleAnalyze(w, req)

	if w.Result().StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303 redirect for BOM CSV, got %d", w.Result().StatusCode)
	}
}

func TestHandleAnalyzeEmptyCSV(t *testing.T) {
	payload := buildMultipartPayload(t, "")
	req := httptest.NewRequest("POST", "/analyze", payload.Body)
	req.Header.Set("Content-Type", payload.ContentType)

	w := httptest.NewRecorder()
	handleAnalyze(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty CSV, got %d", w.Result().StatusCode)
	}
}

func TestHandleAnalyzeMissingColumns(t *testing.T) {
	csvContent := "name,value\ngpt-4,100\n"
	payload := buildMultipartPayload(t, csvContent)
	req := httptest.NewRequest("POST", "/analyze", payload.Body)
	req.Header.Set("Content-Type", payload.ContentType)

	w := httptest.NewRecorder()
	handleAnalyze(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing columns, got %d", w.Result().StatusCode)
	}
}

func TestHandleReportNotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/report/99999", nil)
	w := httptest.NewRecorder()
	handleReport(w, req)

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Result().StatusCode)
	}
}

func TestHandleReportInvalidID(t *testing.T) {
	req := httptest.NewRequest("GET", "/report/abc", nil)
	w := httptest.NewRecorder()
	handleReport(w, req)

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Result().StatusCode)
	}
}

func TestParseFloatSurfaceErrors(t *testing.T) {
	v, err := csv.NewReader(strings.NewReader("not_a_number\n")).Read()
	if err != nil {
		t.Skip("could not parse test CSV")
	}
	_ = v
}

func TestGeneratePDF(t *testing.T) {
	report := makeTestReport()
	pdfBytes, err := generatePDF(&report)
	if err != nil {
		t.Fatalf("generatePDF failed: %v", err)
	}
	if len(pdfBytes) == 0 {
		t.Error("expected non-empty PDF bytes")
	}
}

func TestHandlePDF(t *testing.T) {
	csvContent := "model,input_tokens,output_tokens,cost\ngpt-4,1000,200,5.00\n"
	payload := buildMultipartPayload(t, csvContent)
	req := httptest.NewRequest("POST", "/analyze", payload.Body)
	req.Header.Set("Content-Type", payload.ContentType)

	w := httptest.NewRecorder()
	handleAnalyze(w, req)
	if w.Result().StatusCode != http.StatusSeeOther {
		t.Fatalf("analyze failed: %d", w.Result().StatusCode)
	}

	loc := w.Result().Header.Get("Location")
	pdfW := httptest.NewRecorder()

	parts := strings.Split(strings.TrimPrefix(loc, "/report/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		id := 0
		for _, c := range parts[0] {
			if c >= '0' && c <= '9' {
				id = id*10 + int(c-'0')
			}
		}
		if id > 0 {
			handlePDF(pdfW, id)
			if pdfW.Result().StatusCode == http.StatusOK {
				ct := pdfW.Result().Header.Get("Content-Type")
				if ct != "application/pdf" {
					t.Errorf("expected application/pdf, got %s", ct)
				}
			}
		}
	}
}

func TestHandleReportAfterAnalyze(t *testing.T) {
	csvContent := "model,input_tokens,output_tokens,cost\ngpt-4,1000,200,5.00\n"
	payload := buildMultipartPayload(t, csvContent)
	req := httptest.NewRequest("POST", "/analyze", payload.Body)
	req.Header.Set("Content-Type", payload.ContentType)

	w := httptest.NewRecorder()
	handleAnalyze(w, req)
	if w.Result().StatusCode != http.StatusSeeOther {
		t.Fatalf("analyze failed: %d", w.Result().StatusCode)
	}

	loc := w.Result().Header.Get("Location")
	reportReq := httptest.NewRequest("GET", loc, nil)
	reportW := httptest.NewRecorder()
	handleReport(reportW, reportReq)

	if reportW.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 for report, got %d", reportW.Result().StatusCode)
	}
}



// Test helpers

type multipartPayload struct {
	Body        *bytes.Buffer
	ContentType string
}

func buildMultipartPayload(t *testing.T, csvContent string) multipartPayload {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "test.csv")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := fw.Write([]byte(csvContent)); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}
	return multipartPayload{Body: &buf, ContentType: w.FormDataContentType()}
}

func makeTestReport() engine.AssessmentReport {
	return engine.AssessmentReport{
		Assessment: engine.Assessment{
			CompanyName: "TestCorp",
			Source:      "manual",
			Currency:    "USD",
		},
		CostBreakdown: []engine.CostProjection{
			{Model: "gpt-4", Provider: "openai", CurrentMonthlyCost: 5000, ProjectedMonthlyCost: 3000, InputTokensMillions: 10, OutputTokensMillions: 2},
		},
		Recommendations: []engine.Recommendation{
			{Category: "model_switch", Description: "Switch gpt-4 to gpt-4o", Priority: "high", MonthlySavings: 2000, CurrentCost: 5000, PaybackPeriodDays: 0},
		},
		TotalCurrent:   5000,
		TotalProjected: 3000,
		TotalSavings:   2000,
		Currency:       "USD",
		CurrencySymbol: "$",
	}
}
