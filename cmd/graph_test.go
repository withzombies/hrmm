package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mcpherrinm/hrmm/internal/fetcher"
)

func TestDashboardModel_TickMsgTriggersFetch(t *testing.T) {
	model := dashboardModel{
		selectedMetrics: []string{"test_metric"},
		interval:        time.Second,
	}

	msg := tickMsg(time.Now())
	_, cmd := model.Update(msg)

	if cmd == nil {
		t.Error("expected tickMsg to return a command for fetching")
	}
}

func TestDashboardModel_MetricsMsgUpdatesState(t *testing.T) {
	model := dashboardModel{
		selectedMetrics: []string{"test_metric"},
		interval:        time.Second,
	}

	testData := []fetcher.MetricData{
		{Name: "test_metric", Value: fetcher.NullableFloat64(42.0)},
	}

	msg := metricsMsg{data: testData, err: nil}
	result, cmd := model.Update(msg)

	dm := result.(dashboardModel)

	if len(dm.metricsData) != 1 {
		t.Errorf("expected 1 metric, got %d", len(dm.metricsData))
	}

	if dm.lastFetch.IsZero() {
		t.Error("expected lastFetch to be set")
	}

	if dm.lastError != nil {
		t.Errorf("expected nil error, got %v", dm.lastError)
	}

	// Should return pollTick command to continue polling
	if cmd == nil {
		t.Error("expected metricsMsg to return a command for next poll")
	}
}

func TestDashboardModel_MetricsMsgError(t *testing.T) {
	model := dashboardModel{
		selectedMetrics: []string{"test_metric"},
		interval:        time.Second,
	}

	testErr := errors.New("connection refused")
	msg := metricsMsg{data: nil, err: testErr}

	result, cmd := model.Update(msg)
	dm := result.(dashboardModel)

	if dm.lastError == nil {
		t.Error("expected lastError to be set")
	}

	if dm.lastError.Error() != "connection refused" {
		t.Errorf("expected 'connection refused', got '%v'", dm.lastError)
	}

	// Should continue polling despite error
	if cmd == nil {
		t.Error("expected polling to continue with a command")
	}
}

func TestDashboardModel_MetricsMsgClearsError(t *testing.T) {
	// Start with an existing error
	model := dashboardModel{
		selectedMetrics: []string{"test_metric"},
		interval:        time.Second,
		lastError:       errors.New("previous error"),
	}

	// Send successful metrics
	testData := []fetcher.MetricData{
		{Name: "test_metric", Value: fetcher.NullableFloat64(42.0)},
	}
	msg := metricsMsg{data: testData, err: nil}

	result, _ := model.Update(msg)
	dm := result.(dashboardModel)

	if dm.lastError != nil {
		t.Errorf("expected lastError to be cleared, got %v", dm.lastError)
	}
}

func TestDashboardModel_InitReturnsBatch(t *testing.T) {
	model := dashboardModel{
		selectedMetrics: []string{"test_metric"},
		interval:        time.Second,
	}

	cmd := model.Init()

	if cmd == nil {
		t.Error("expected Init to return a batch command")
	}

	// tea.Batch returns a function, we can't easily inspect its contents
	// but we verify it's not nil
}

func TestDashboardModel_QuitOnCtrlC(t *testing.T) {
	model := dashboardModel{
		selectedMetrics: []string{"test_metric"},
		interval:        time.Second,
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := model.Update(msg)

	// tea.Quit is a special command, check it's not nil
	if cmd == nil {
		t.Error("expected ctrl+c to return quit command")
	}
}

func TestDashboardModel_QuitOnQ(t *testing.T) {
	model := dashboardModel{
		selectedMetrics: []string{"test_metric"},
		interval:        time.Second,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := model.Update(msg)

	if cmd == nil {
		t.Error("expected 'q' to return quit command")
	}
}

func TestDashboardModel_WindowSizeUpdates(t *testing.T) {
	model := dashboardModel{
		selectedMetrics: []string{"test_metric"},
		interval:        time.Second,
	}

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	result, _ := model.Update(msg)

	dm := result.(dashboardModel)

	if dm.width != 120 {
		t.Errorf("expected width 120, got %d", dm.width)
	}
	if dm.height != 40 {
		t.Errorf("expected height 40, got %d", dm.height)
	}
}

func TestFetchMetrics_WithHttpTestServer(t *testing.T) {
	// Create test server with changing metrics
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, `# HELP test_counter A test counter
# TYPE test_counter counter
test_counter %d
`, callCount*10)
	}))
	defer server.Close()

	// Create fetcher pointing to test server
	f := fetcher.New(server.URL, nil, nil)

	// Verify multiple fetches show increasing values
	for i := 1; i <= 3; i++ {
		metrics, err := f.Fetch()
		if err != nil {
			t.Fatalf("fetch %d failed: %v", i, err)
		}

		if len(metrics) != 1 {
			t.Fatalf("fetch %d: expected 1 metric, got %d", i, len(metrics))
		}

		expected := float64(i * 10)
		if float64(metrics[0].Value) != expected {
			t.Errorf("fetch %d: expected %f, got %f", i, expected, float64(metrics[0].Value))
		}
	}
}

func TestDashboardModel_ViewShowsError(t *testing.T) {
	model := dashboardModel{
		selectedMetrics: []string{"test_metric"},
		interval:        time.Second,
		width:           80,
		height:          24,
		lastError:       errors.New("connection timeout"),
	}

	view := model.View()

	if view == "" {
		t.Error("expected non-empty view")
	}

	// Check that error is displayed
	if !containsString(view, "Error") {
		t.Error("expected view to contain error message")
	}
}

func TestDashboardModel_ViewShowsMetrics(t *testing.T) {
	model := dashboardModel{
		selectedMetrics: []string{"cpu_usage", "memory_bytes"},
		interval:        time.Second,
		width:           80,
		height:          24,
		metricsData: []fetcher.MetricData{
			{Name: "cpu_usage", Value: fetcher.NullableFloat64(42.0)},
			{Name: "memory_bytes", Value: fetcher.NullableFloat64(1024)},
		},
	}

	view := model.View()

	if !containsString(view, "cpu_usage") {
		t.Error("expected view to contain cpu_usage")
	}
	if !containsString(view, "memory_bytes") {
		t.Error("expected view to contain memory_bytes")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
