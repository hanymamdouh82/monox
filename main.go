package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hanymamdouh82/monox/internal/config"
	"github.com/hanymamdouh82/monox/internal/fetcher"
	"github.com/jroimartin/gocui"
)

// Pane positions and tracking labels
const (
	TEMPS        = "temps"
	MEMORY       = "memory"
	DOCKER_PS    = "docker_ps"
	DOCKER_STATS = "docker_stats"
	SYNCTHING    = "syncthing"
	DISK_USAGE   = "disk_usage"
	SMART        = "smart"
	SYSTEM_LOAD  = "system_load"
)

// Ordered list of panes for keyboard cycling
var paneCyclingOrder = []string{
	TEMPS, MEMORY,
	DOCKER_PS, DOCKER_STATS,
	SYNCTHING, DISK_USAGE,
	SMART, SYSTEM_LOAD,
}

func main() {
	// 1. Define the CLI string flag with a fallback default value
	configPath := flag.String("config", "config.yaml", "Path to the YAML configuration file")

	// 2. Parse the CLI arguments
	flag.Parse()

	// 3. Pass the evaluated pointer string value to the configuration package
	if err := config.LoadConfig(*configPath); err != nil {
		log.Fatalf("Critical: failed to load configuration file [%s]: %v", *configPath, err)
	}

	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	// Configure layout function and global keys
	g.SetManagerFunc(layout)

	// Initialize keyboard and mouse interaction bindings
	if err := initKeybindings(g); err != nil {
		log.Panicln(err)
	}

	// Initialize background monitoring routines
	go startTickers(g)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		g.Close() // Safely restore terminal screen state
		os.Exit(0)
	}()

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}
}

// Layout creates a 2-column x 4-row grid with absolute coordinate tracking
func layout(g *gocui.Gui) error {
	// Clear the panel matrix canvas to solve layout text ghosting across window modifications
	// g.Clear()

	maxX, maxY := g.Size()

	// ─── 1. Core Branding Banner ─────────────────────────────────────────────
	if v, err := g.SetView("branding", -1, -1, maxX, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		v.FgColor = gocui.ColorCyan | gocui.AttrBold

		// Center the branding text dynamically based on terminal width
		title := fmt.Sprintf(" %s // MONOX - MONITORE ON UNIX ", config.AppConfig.Name)
		padding := (maxX - len(title)) / 2
		if padding > 0 {
			fmt.Fprint(v, strings.Repeat(" ", padding)+title)
		} else {
			fmt.Fprint(v, title)
		}
	}

	// ─── 2. Grid Geometry Calculations ───────────────────────────────────────
	startY := 2
	usableY := maxY - startY
	rowH := usableY / 4
	halfX := maxX / 2

	// Left column configuration (0 -> halfX)
	if err := createPane(g, TEMPS, " Temps [2s] ", 0, startY, halfX-1, startY+rowH-1); err != nil {
		return err
	}
	if err := createPane(g, DOCKER_PS, " Docker PS [5s] ", 0, startY+rowH, halfX-1, startY+(rowH*2)-1); err != nil {
		return err
	}
	if err := createPane(g, SYNCTHING, " Syncthing [60s] ", 0, startY+(rowH*2), halfX-1, startY+(rowH*3)-1); err != nil {
		return err
	}
	if err := createPane(g, SMART, " SMART [60s] ", 0, startY+(rowH*3), halfX-1, maxY-1); err != nil {
		return err
	}

	// Right column configuration (halfX -> maxX)
	if err := createPane(g, MEMORY, " Memory [2s] ", halfX, startY, maxX-1, startY+rowH-1); err != nil {
		return err
	}
	if err := createPane(g, DOCKER_STATS, " Docker Stats [10s] ", halfX, startY+rowH, maxX-1, startY+(rowH*2)-1); err != nil {
		return err
	}
	if err := createPane(g, DISK_USAGE, " Disk Usage [60s] ", halfX, startY+(rowH*2), maxX-1, startY+(rowH*3)-1); err != nil {
		return err
	}
	if err := createPane(g, SYSTEM_LOAD, " System Load [2s] ", halfX, startY+(rowH*3), maxX-1, maxY-1); err != nil {
		return err
	}

	// Set default focus to the first panel on initialization
	if g.CurrentView() == nil {
		g.Highlight = true
		g.SelFgColor = gocui.ColorCyan
		if _, err := g.SetCurrentView(TEMPS); err != nil {
			return err
		}
	}

	return nil
}

// Helper to create and initialize consistent pane layout templates
func createPane(g *gocui.Gui, id string, title string, x0, y0, x1, y1 int) error {
	if v, err := g.SetView(id, x0, y0, x1, y1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = title
		v.Wrap = true        // Enables soft line-wrapping on text boundaries
		v.Autoscroll = false // Keep false to allow manual engine view scroll control bounds
		v.FgColor = gocui.ColorBlue
	}
	return nil
}

// ─── Interaction Bindings & Navigation Logic ─────────────────────────────────

func initKeybindings(g *gocui.Gui) error {
	// Program Termination Bindings
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'q', gocui.ModNone, quit); err != nil {
		return err
	}

	// Focus Pane on Click
	g.Mouse = true
	if err := g.SetKeybinding("", gocui.MouseLeft, gocui.ModNone, clickToFocus); err != nil {
		return err
	}

	// Navigation: Up/Down Arrows (Global Target Interaction Engine)
	if err := g.SetKeybinding("", gocui.KeyArrowDown, gocui.ModNone, scrollDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyArrowUp, gocui.ModNone, scrollUp); err != nil {
		return err
	}

	// Navigation: Mouse Scroll Wheel
	if err := g.SetKeybinding("", gocui.MouseWheelDown, gocui.ModNone, scrollDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.MouseWheelUp, gocui.ModNone, scrollUp); err != nil {
		return err
	}

	// Keyboard Pane Navigation (Forward with Tab, Backward with Shift+Tab)
	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, nextPane); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModAlt, prevPane); err != nil {
		return err
	}

	return nil
}
func nextPane(g *gocui.Gui, v *gocui.View) error {
	return switchPane(g, 1)
}

func prevPane(g *gocui.Gui, v *gocui.View) error {
	return switchPane(g, -1)
}

func switchPane(g *gocui.Gui, direction int) error {
	currentView := g.CurrentView()
	nextIdx := 0

	// Find our position in the cycling order loop array
	if currentView != nil {
		for i, name := range paneCyclingOrder {
			if name == currentView.Name() {
				// Calculate index wrapping cleanly via modulo math
				nextIdx = (i + direction + len(paneCyclingOrder)) % len(paneCyclingOrder)
				break
			}
		}
	}

	targetViewName := paneCyclingOrder[nextIdx]
	targetView, err := g.SetCurrentView(targetViewName)
	if err != nil {
		return err
	}

	// Native gocui selection coloring configuration:
	g.Highlight = true
	g.SelFgColor = gocui.ColorCyan

	_, _ = g.SetViewOnTop(targetViewName)
	return targetView.SetCursor(0, 0)
}

func clickToFocus(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	_, err := g.SetCurrentView(v.Name())
	return err
}

func scrollDown(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		ox, oy := v.Origin()
		cx, cy := v.Cursor()
		// Shift viewport downward by incrementing structural origin index bounds
		if err := v.SetOrigin(ox, oy+1); err != nil {
			return nil
		}
		_ = v.SetCursor(cx, cy+1)
	}
	return nil
}

func scrollUp(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		ox, oy := v.Origin()
		if oy > 0 {
			// Pull structural layout upstream
			_ = v.SetOrigin(ox, oy-1)
		}
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

// ─── Asynchronous Worker Loop ────────────────────────────────────────────────

func startTickers(g *gocui.Gui) {
	updateView(g, TEMPS, fetcher.FetchTemps)
	updateView(g, MEMORY, fetcher.FetchMemory)
	updateView(g, DOCKER_PS, fetcher.FetchDockerPs)
	updateView(g, DOCKER_STATS, fetcher.FetchDockerStats)
	updateView(g, SYNCTHING, fetcher.FetchSyncthing)
	updateView(g, DISK_USAGE, fetcher.FetchDiskUsage)
	updateView(g, SMART, fetcher.FetchSmart)
	updateView(g, SYSTEM_LOAD, fetcher.FetchSystemLoad)

	t2 := time.NewTicker(2 * time.Second)
	t5 := time.NewTicker(5 * time.Second)
	t10 := time.NewTicker(10 * time.Second)
	t60 := time.NewTicker(60 * time.Second)

	for {
		select {
		case <-t2.C:
			go updateView(g, TEMPS, fetcher.FetchTemps)
			go updateView(g, MEMORY, fetcher.FetchMemory)
			go updateView(g, SYSTEM_LOAD, fetcher.FetchSystemLoad)
		case <-t5.C:
			go updateView(g, DOCKER_PS, fetcher.FetchDockerPs)
		case <-t10.C:
			go updateView(g, DOCKER_STATS, fetcher.FetchDockerStats)
		case <-t60.C:
			go updateView(g, SMART, fetcher.FetchSmart)
			go updateView(g, DISK_USAGE, fetcher.FetchDiskUsage)
			go updateView(g, SYNCTHING, fetcher.FetchSyncthing)
		}
	}
}

func updateView(g *gocui.Gui, id string, fetchFunc func() string) {
	content := fetchFunc()

	g.Update(func(g *gocui.Gui) error {
		v, err := g.View(id)
		if err != nil {
			return nil
		}

		// To prevent background refreshes from pulling the user out of their manually
		// scrolled tracking positions, we capture the existing layout coordinate context
		// and restore it immediately after writing the new block frame data.
		ox, oy := v.Origin()

		v.Clear()
		fmt.Fprint(v, content)

		_ = v.SetOrigin(ox, oy)
		return nil
	})
}
