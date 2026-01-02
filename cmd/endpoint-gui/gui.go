package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/google/uuid"
	"github.com/spf13/viper"
)

type GUI struct {
	app    fyne.App
	window fyne.Window
	worker *Worker

	// UI Elements
	statusLabel *widget.Label
	startButton *widget.Button
	stopButton  *widget.Button
	logList     *widget.List

	// Stats Labels
	tasksLabel      *widget.Label
	latencyLabel    *widget.Label
	bandwidthLabel  *widget.Label
	rmqStatusLabel  *widget.Label
	queueDepthLabel *widget.Label

	// Logging
	logChan chan string
	logMu   sync.Mutex
	logData []string

	// Config
	configPath string

	// Context for goroutine management
	ctx    context.Context
	cancel context.CancelFunc
}

func NewGUI(app fyne.App, window fyne.Window, worker *Worker, configPath string) *GUI {
	ctx, cancel := context.WithCancel(context.Background())
	gui := &GUI{
		app:        app,
		window:     window,
		worker:     worker,
		configPath: configPath,
		logChan:    make(chan string, 1000), // Buffer up to 1000 log lines
		ctx:        ctx,
		cancel:     cancel,
	}
	gui.setupUI()
	go gui.logLoop()
	return gui
}

func (g *GUI) setupUI() {
	// Status Section
	g.statusLabel = widget.NewLabel("Stopped")
	g.statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Control Section
	g.startButton = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), g.onStart)
	g.stopButton = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), g.onStop)
	g.stopButton.Disable()
	clearButton := widget.NewButtonWithIcon("Clear Logs", theme.DeleteIcon(), func() {
		g.logMu.Lock()
		g.logData = nil
		g.logMu.Unlock()
		g.logList.Refresh()
	})
	settingsButton := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), g.onSettings)

	controls := container.NewHBox(g.startButton, g.stopButton, clearButton, layout.NewSpacer(), settingsButton)

	// Stats Section
	g.tasksLabel = widget.NewLabel("Tasks: 0 (0 Failed)")
	g.latencyLabel = widget.NewLabel("Latency: 0ms")
	g.bandwidthLabel = widget.NewLabel("Bandwidth: 0 B sent / 0 B recv")
	g.rmqStatusLabel = widget.NewLabel("RMQ: Disconnected")
	g.queueDepthLabel = widget.NewLabel("Queue: 0")

	stats := container.NewVBox(
		container.NewHBox(widget.NewLabel("Status: "), g.statusLabel),
		container.NewHBox(g.tasksLabel, layout.NewSpacer(), g.latencyLabel),
		container.NewHBox(g.bandwidthLabel),
		container.NewHBox(g.rmqStatusLabel, layout.NewSpacer(), g.queueDepthLabel),
	)

	// Log Section
	g.logList = widget.NewList(
		func() int {
			g.logMu.Lock()
			defer g.logMu.Unlock()
			return len(g.logData)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("template content")
			label.TextStyle = fyne.TextStyle{Monospace: true}
			return label
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			g.logMu.Lock()
			if id < 0 || id >= len(g.logData) {
				g.logMu.Unlock()
				return
			}
			text := g.logData[id]
			g.logMu.Unlock()
			item.(*widget.Label).SetText(text)
		},
	)
	g.logData = []string{"Application ready."}

	// Layout
	content := container.NewBorder(
		container.NewVBox(
			controls,
			widget.NewSeparator(),
			stats,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		container.NewPadded(g.logList), // Main content is the log list
	)

	g.window.SetContent(content)
	g.window.Resize(fyne.NewSize(600, 500))

	// Set up window close handler to cancel context
	g.window.SetCloseIntercept(func() {
		g.cancel() // Cancel context to stop goroutines
		g.window.Close()
	})

	// Start stats update loop
	go g.statsLoop()
}

func (g *GUI) statsLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			// Context cancelled, exit the loop
			return
		case <-ticker.C:
			stats := g.worker.GetStats()
			g.tasksLabel.SetText(fmt.Sprintf("Tasks: %d (%d Failed)", stats.TasksProcessed, stats.TasksFailed))

			avgLat := time.Duration(0)
			if stats.TasksProcessed > 0 {
				avgLat = time.Duration(stats.LatencySum / int64(stats.TasksProcessed))
			}
			g.latencyLabel.SetText(fmt.Sprintf("Avg Latency: %s", avgLat.Round(time.Millisecond)))
			g.bandwidthLabel.SetText(fmt.Sprintf("Bandwidth: %s sent / %s recv", formatBytes(stats.BytesSent), formatBytes(stats.BytesReceived)))

			status := "Disconnected"
			if stats.Connected == 1 {
				status = "Connected"
			}
			g.rmqStatusLabel.SetText(fmt.Sprintf("RMQ: %s", status))
			g.queueDepthLabel.SetText(fmt.Sprintf("Queue Depth: %d", stats.QueueDepth))
		}
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (g *GUI) onStart() {
	g.startButton.Disable()
	g.log("Starting endpoint...")

	go func() {
		// Use a local copy of path to avoid races, though g.configPath shouldn't change
		err := g.worker.Start(g.configPath)
		if err != nil {
			g.log("Error starting: " + err.Error())
			g.updateStatus("Error", false)
			// Re-enable start button on error
			g.window.Canvas().Refresh(g.startButton) // trigger refresh just in case
			g.startButton.Enable()
			return
		}

		g.updateStatus("Running", true)
		g.stopButton.Enable()
	}()
}

func (g *GUI) onStop() {
	g.stopButton.Disable()
	g.log("Stopping endpoint...")

	go func() {
		g.worker.Stop()
		g.updateStatus("Stopped", false)
		g.startButton.Enable()
	}()
}

func (g *GUI) onSettings() {
	if g.worker.IsRunning() {
		dialog.ShowConfirm("Worker Running", "The endpoint is currently running. Changes will only take effect after restarting the worker. Do you want to continue?", func(ok bool) {
			if ok {
				ShowConfigDialog(g.app, g.window, g.configPath, nil)
			}
		}, g.window)
	} else {
		ShowConfigDialog(g.app, g.window, g.configPath, nil)
	}
}

func (g *GUI) log(msg string) {
	// Send to channel instead of updating UI directly
	select {
	case g.logChan <- msg:
	default:
		// Channel full, drop log or handle overflow
	}
}

func (g *GUI) logLoop() {
	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	// background routine to pull from channel and append to logData
	go func() {
		for {
			select {
			case <-g.ctx.Done():
				// Context cancelled, exit the goroutine
				return
			default:
				// Pull one, then drain the rest
				msg, ok := <-g.logChan
				if !ok {
					return
				}

				var batch []string
				timeStr := time.Now().Format("15:04:05")
				batch = append(batch, fmt.Sprintf("[%s] %s", timeStr, msg))

				// Drain channel
			drain:
				for {
					select {
					case m := <-g.logChan:
						batch = append(batch, fmt.Sprintf("[%s] %s", timeStr, m))
						if len(batch) > 100 { // Don't batch too many at once to avoid huge locks
							break drain
						}
					default:
						break drain
					}
				}

				g.logMu.Lock()
				g.logData = append(g.logData, batch...)
				// Cap the internal buffer to prevent memory issues
				if len(g.logData) > 2000 {
					g.logData = g.logData[len(g.logData)-1000:]
				}
				g.logMu.Unlock()
			}
		}
	}()

	for {
		select {
		case <-g.ctx.Done():
			// Context cancelled, exit the loop
			return
		case <-ticker.C:
			g.logMu.Lock()
			if len(g.logData) == 0 {
				g.logMu.Unlock()
				continue
			}
			g.logMu.Unlock()

			g.logList.Refresh()
			g.logList.ScrollToBottom()
		}
	}
}

func (g *GUI) updateStatus(status string, running bool) {
	g.statusLabel.SetText(status)
	if running {
		g.statusLabel.TextStyle = fyne.TextStyle{Bold: true} // Maybe color if possible
	} else {
		g.statusLabel.TextStyle = fyne.TextStyle{Bold: false}
	}
}

// LogWriter implements io.Writer to bridge slog to GUI
type LogWriter struct {
	gui *GUI
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// Trim newline as log process adds it
	msg = strings.TrimSuffix(msg, "\n")
	w.gui.log(msg)
	return len(p), nil
}

// ShowConfigDialog shows the configuration dialog
func ShowConfigDialog(app fyne.App, parent fyne.Window, savePath string, done func()) {
	title := "Configuration"
	welcomeMsg := "Configure your endpoint settings."

	// If done is NOT nil, it's the initial setup
	if done != nil {
		title = "Initial Setup"
		welcomeMsg = "Welcome to Straw Endpoint! Please configure your settings."
	}

	win := app.NewWindow(title)

	idEntry := widget.NewEntry()
	idEntry.SetText(viper.GetString("endpoint_id"))
	if idEntry.Text == "" {
		idEntry.SetText(uuid.New().String())
	}

	rabbitEntry := widget.NewEntry()
	rabbitEntry.SetText(viper.GetString("rabbitmq_url"))
	rabbitEntry.SetPlaceHolder("amqp://guest:guest@localhost:5672/")

	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetText(viper.GetString("hmac_secret"))

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Endpoint ID", Widget: idEntry},
			{Text: "RabbitMQ URL", Widget: rabbitEntry},
			{Text: "HMAC Secret", Widget: secretEntry},
		},
		OnSubmit: func() {
			if idEntry.Text == "" || rabbitEntry.Text == "" || secretEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("endpoint ID, RabbitMQ URL, and HMAC Secret are all required"), win)
				return
			}

			// Save Config
			viper.Set("endpoint_id", idEntry.Text)
			viper.Set("rabbitmq_url", rabbitEntry.Text)
			viper.Set("hmac_secret", secretEntry.Text)

			// Defaults (only if not set)
			if !viper.IsSet("log_level") {
				viper.Set("log_level", "info")
			}
			if !viper.IsSet("log_format") {
				viper.Set("log_format", "text")
			}
			if !viper.IsSet("concurrency_limit") {
				viper.Set("concurrency_limit", 100)
			}

			if err := viper.WriteConfigAs(savePath); err != nil {
				dialog.ShowError(fmt.Errorf("failed to save config: %w", err), win)
				return
			}

			win.Close()
			if done != nil {
				done()
			}
		},
	}

	win.SetContent(container.NewPadded(
		container.NewVBox(
			widget.NewLabel(welcomeMsg),
			form,
		),
	))
	win.Resize(fyne.NewSize(400, 300))
	win.Show()
}
