package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mcpherrinm/hrmm/internal/fetcher"
	"github.com/spf13/cobra"
)

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
	list list.Model
	err  error
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
				return newDashboardModel(selectedMetrics), nil
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
}

func newDashboardModel(metrics []string) dashboardModel {
	return dashboardModel{
		selectedMetrics: metrics,
	}
}

func (m dashboardModel) Init() tea.Cmd {
	return nil // Polling will be added in a future task
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
	}
	return m, nil
}

func (m dashboardModel) View() string {
	s := "Dashboard\n\n"
	s += fmt.Sprintf("Terminal: %dx%d\n\n", m.width, m.height)

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
		// Fetch metrics from all URLs
		var allMetrics []fetcher.MetricData
		for _, url := range urls {
			metricsFetcher := fetcher.New(url, metrics, labels)
			metricsData, err := metricsFetcher.Fetch()
			if err != nil {
				fmt.Printf("Error fetching metrics from %s: %v\n", url, err)
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
			list: l,
		}, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
			os.Exit(1)
		}
	},
}
