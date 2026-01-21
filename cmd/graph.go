package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mcpherrinm/hrmm/internal/fetcher"
	"github.com/spf13/cobra"
)

// Message types for dashboard polling
type tickMsg time.Time
type metricsMsg struct {
	data []fetcher.MetricData
	err  error
}

// metricItem implements list.Item for MetricData
type metricItem struct {
	metric   fetcher.MetricData
	selected bool
}

func (i metricItem) FilterValue() string { return i.metric.Identifier() }

func (i metricItem) Title() string { return i.metric.Identifier() }

func (i metricItem) Description() string {
	selected := " "
	if i.selected {
		selected = "x"
	}
	return fmt.Sprintf("[%s] %s", selected, i.metric.Help)
}

// metricSelectionModel represents the metric selection screen using bubbles/list
type metricSelectionModel struct {
	list     list.Model
	err      error
	fetchers []*fetcher.MetricsFetcher
	interval time.Duration
}

func (m *metricSelectionModel) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m *metricSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Update list dimensions to fit terminal size
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 2) // Leave space for title and padding
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case " ":
			// Toggle selection
			if selectedItem, ok := m.list.SelectedItem().(metricItem); ok {
				selectedItem.selected = !selectedItem.selected
				m.list.SetItem(m.list.Index(), selectedItem)
			}
		case "enter":
			// Proceed to graph view with selected metrics
			var selectedMetrics []string
			for _, item := range m.list.Items() {
				if metricItem, ok := item.(metricItem); ok && metricItem.selected {
					selectedMetrics = append(selectedMetrics, metricItem.metric.Name)
				}
			}
			if len(selectedMetrics) > 0 {
				return newDashboardModel(selectedMetrics, m.fetchers, m.interval), nil
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *metricSelectionModel) View() string {
	return "\n" + m.list.View()
}

// dashboardModel represents the dashboard view with live-updating charts
type dashboardModel struct {
	selectedMetrics []string
	width           int
	height          int
	fetchers        []*fetcher.MetricsFetcher
	interval        time.Duration
	lastFetch       time.Time
	lastError       error
	metricsData     []fetcher.MetricData
}

func newDashboardModel(metrics []string, fetchers []*fetcher.MetricsFetcher, interval time.Duration) dashboardModel {
	return dashboardModel{
		selectedMetrics: metrics,
		fetchers:        fetchers,
		interval:        interval,
	}
}

func (m dashboardModel) pollTick() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m dashboardModel) fetchMetrics() tea.Cmd {
	return func() tea.Msg {
		var allData []fetcher.MetricData
		for _, f := range m.fetchers {
			data, err := f.Fetch()
			if err != nil {
				return metricsMsg{data: nil, err: err}
			}
			allData = append(allData, data...)
		}
		return metricsMsg{data: allData, err: nil}
	}
}

func (m dashboardModel) Init() tea.Cmd {
	return tea.Batch(m.pollTick(), m.fetchMetrics())
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		return m, m.fetchMetrics()
	case metricsMsg:
		m.lastFetch = time.Now()
		if msg.err != nil {
			m.lastError = msg.err
		} else {
			m.lastError = nil
			m.metricsData = msg.data
		}
		return m, m.pollTick()
	}
	return m, nil
}

func (m dashboardModel) View() string {
	s := "Dashboard\n"
	s += fmt.Sprintf("Terminal: %dx%d | ", m.width, m.height)
	if !m.lastFetch.IsZero() {
		s += fmt.Sprintf("Last fetch: %s ago | ", time.Since(m.lastFetch).Round(time.Second))
	}
	s += fmt.Sprintf("Metrics: %d\n\n", len(m.metricsData))

	if m.lastError != nil {
		s += fmt.Sprintf("⚠ Error: %v\n\n", m.lastError)
	}

	if len(m.selectedMetrics) == 0 {
		s += "No metrics selected.\n"
	} else {
		s += "Selected metrics:\n"
		for _, name := range m.selectedMetrics {
			s += fmt.Sprintf("  • %s\n", name)
		}
	}

	s += "\nPress q to quit.\n"
	return s
}

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Display metrics in a graph/TUI format",
	Long:  "Poll prometheus metrics endpoints and display the results in a graph or TUI format.",
	Run: func(cmd *cobra.Command, args []string) {
		// Create fetchers for all URLs
		var fetchers []*fetcher.MetricsFetcher
		for _, url := range urls {
			fetchers = append(fetchers, fetcher.New(url, metrics, labels))
		}

		// Fetch metrics from all URLs for initial picker display
		var allMetrics []fetcher.MetricData
		for _, f := range fetchers {
			metricsData, err := f.Fetch()
			if err != nil {
				fmt.Printf("Error fetching metrics: %v\n", err)
				continue
			}
			allMetrics = append(allMetrics, metricsData...)
		}

		if len(allMetrics) == 0 {
			fmt.Println("No metrics found")
			return
		}

		// Convert metrics to list items
		items := make([]list.Item, len(allMetrics))
		for i, metric := range allMetrics {
			items[i] = metricItem{
				metric:   metric,
				selected: false,
			}
		}

		l := list.New(items, list.NewDefaultDelegate(), 80, 25)
		l.Title = "Select metrics to graph"
		l.SetShowStatusBar(false)
		l.SetFilteringEnabled(true)
		l.Styles.Title = l.Styles.Title.Foreground(list.DefaultStyles().Title.GetForeground())

		p := tea.NewProgram(&metricSelectionModel{
			list:     l,
			fetchers: fetchers,
			interval: pollInterval,
		}, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
			os.Exit(1)
		}
	},
}
