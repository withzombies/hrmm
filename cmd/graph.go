package cmd

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mcpherrinm/hrmm/internal/buffer"
	"github.com/mcpherrinm/hrmm/internal/fetcher"
	"github.com/spf13/cobra"
)

// Message types for dashboard polling
type tickMsg time.Time
type metricsMsg struct {
	data []fetcher.MetricData
	err  error
}

// metricGraph holds the data and chart for a single metric
type metricGraph struct {
	name   string
	buffer *buffer.RingBuffer
	chart  timeserieslinechart.Model
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
	graphs          map[string]*metricGraph
	width           int
	height          int
	fetchers        []*fetcher.MetricsFetcher
	interval        time.Duration
	lastFetch       time.Time
	lastError       error
}

// calculateGrid returns the number of columns and rows for the grid layout
// based on terminal width and number of metrics
func (m dashboardModel) calculateGrid() (cols, rows int) {
	if m.width < 80 {
		cols = 1
	} else if m.width < 160 {
		cols = 2
	} else {
		cols = 3
	}
	rows = (len(m.selectedMetrics) + cols - 1) / cols // ceiling division
	return cols, rows
}

func newDashboardModel(metrics []string, fetchers []*fetcher.MetricsFetcher, interval time.Duration) dashboardModel {
	graphs := make(map[string]*metricGraph)
	for _, name := range metrics {
		graphs[name] = &metricGraph{
			name:   name,
			buffer: buffer.New(30),
			chart:  timeserieslinechart.New(40, 10), // default size, will be resized
		}
	}
	return dashboardModel{
		selectedMetrics: metrics,
		graphs:          graphs,
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
		// Recalculate chart sizes based on grid layout
		if len(m.selectedMetrics) > 0 {
			cols, rows := m.calculateGrid()
			// Header takes ~4 lines, footer ~2 lines, each row needs label line
			availableHeight := m.height - 6 - rows
			chartHeight := availableHeight / rows
			if chartHeight < 5 {
				chartHeight = 5 // minimum height
			}
			// -2 for padding between columns, -2 for overall margin
			chartWidth := (m.width / cols) - 4
			if chartWidth < 20 {
				chartWidth = 20 // minimum width
			}
			for _, graph := range m.graphs {
				graph.chart.Resize(chartWidth, chartHeight)
				graph.chart.DrawBraille()
			}
		}
	case tickMsg:
		return m, m.fetchMetrics()
	case metricsMsg:
		m.lastFetch = time.Now()
		if msg.err != nil {
			m.lastError = msg.err
		} else {
			m.lastError = nil
			for _, metric := range msg.data {
				if graph, ok := m.graphs[metric.Name]; ok {
					value := float64(metric.Value)
					// Skip NaN/Inf values
					if math.IsNaN(value) || math.IsInf(value, 0) {
						continue
					}
					graph.buffer.Push(value)
					graph.chart.Push(timeserieslinechart.TimePoint{
						Time:  m.lastFetch,
						Value: value,
					})
					graph.chart.DrawBraille()
				}
			}
		}
		return m, m.pollTick()
	}
	return m, nil
}

// renderMetricCell renders a single metric's label and chart as a cell
func (m dashboardModel) renderMetricCell(name string) string {
	graph, ok := m.graphs[name]
	if !ok {
		return ""
	}

	var label string
	if val, ok := graph.buffer.Latest(); ok {
		label = fmt.Sprintf("%s: %.2f (points: %d)", name, val, graph.buffer.Len())
	} else {
		label = fmt.Sprintf("%s: (no data)", name)
	}

	return label + "\n" + graph.chart.View()
}

func (m dashboardModel) View() string {
	s := "Dashboard\n"
	s += fmt.Sprintf("Terminal: %dx%d | ", m.width, m.height)
	if !m.lastFetch.IsZero() {
		s += fmt.Sprintf("Last fetch: %s ago | ", time.Since(m.lastFetch).Round(time.Second))
	}
	cols, _ := m.calculateGrid()
	s += fmt.Sprintf("Metrics: %d | Grid: %d cols\n\n", len(m.graphs), cols)

	if m.lastError != nil {
		s += fmt.Sprintf("⚠ Error: %v\n\n", m.lastError)
	}

	// Handle case where we haven't received WindowSizeMsg yet
	if m.width == 0 || m.height == 0 {
		s += "Waiting for terminal size...\n"
		s += "\nPress q to quit.\n"
		return s
	}

	if len(m.selectedMetrics) == 0 {
		s += "No metrics selected.\n"
	} else {
		// Render metrics in grid layout
		var rows []string
		for i := 0; i < len(m.selectedMetrics); i += cols {
			// Collect cells for this row
			var rowCells []string
			for j := 0; j < cols && i+j < len(m.selectedMetrics); j++ {
				name := m.selectedMetrics[i+j]
				cell := m.renderMetricCell(name)
				rowCells = append(rowCells, cell)
			}
			// Join cells horizontally with some padding
			row := lipgloss.JoinHorizontal(lipgloss.Top, addPadding(rowCells)...)
			rows = append(rows, row)
		}
		// Join rows vertically
		s += strings.Join(rows, "\n\n")
	}

	s += "\n\nPress q to quit.\n"
	return s
}

// addPadding adds spacing between grid cells
func addPadding(cells []string) []string {
	if len(cells) <= 1 {
		return cells
	}
	padded := make([]string, len(cells))
	for i, cell := range cells {
		if i < len(cells)-1 {
			padded[i] = cell + "  " // 2 spaces between columns
		} else {
			padded[i] = cell
		}
	}
	return padded
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
