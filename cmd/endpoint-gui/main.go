package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"github.com/spf13/viper"
)

func main() {
	a := app.New()
	w := a.NewWindow("Straw Endpoint")

	// Determine Config Path
	configDir, err := os.UserConfigDir()
	if err != nil {
		dialog.ShowError(err, w)
		return
	}
	appConfigDir := filepath.Join(configDir, "straw-endpoint")
	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		dialog.ShowError(err, w)
		return
	}
	configPath := filepath.Join(appConfigDir, "config.yaml")

	// Initialize GUI (initially without worker)
	gui := NewGUI(a, w, nil, configPath)

	// Setup Logger
	// Write to both stdout and the GUI log window
	logWriter := &LogWriter{gui: gui}
	multiWriter := io.MultiWriter(os.Stdout, logWriter)
	logger := slog.New(slog.NewTextHandler(multiWriter, nil))

	// Initialize Worker
	worker := NewWorker(logger)
	gui.worker = worker // Inject worker into GUI

	// Check if config exists
	_, err = os.Stat(configPath)
	configExists := !os.IsNotExist(err)

	if !configExists {
		// Show Setup Dialog
		// We pass a callback to show the main window after setup is done
		ShowConfigDialog(a, w, configPath, func() {
			w.Show()
		})
	} else {
		// Config exists, load it into viper so it can be edited/used
		viper.SetConfigFile(configPath)
		_ = viper.ReadInConfig()
		w.Show()
	}

	a.Run()
}
