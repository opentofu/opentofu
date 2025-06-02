// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/opentofu/opentofu/internal/command/inspect"
)

// InspectCommand is a Command implementation that starts a web server
// to visualize OpenTofu configuration dependencies and relationships.
type InspectCommand struct {
	Meta
}

func (c *InspectCommand) Run(args []string) int {
	var address string
	var port int
	var noBrowser bool
	var urlOnly bool
	var devMode bool

	ctx := c.CommandContext()

	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("inspect")
	cmdFlags.StringVar(&address, "address", "127.0.0.1", "address to bind to")
	cmdFlags.IntVar(&port, "port", 0, "port to bind to (0 for random ephemeral port)")
	cmdFlags.BoolVar(&noBrowser, "no-browser", false, "don't open browser automatically")
	cmdFlags.BoolVar(&urlOnly, "url-only", false, "output the URL and exit")
	cmdFlags.BoolVar(&devMode, "dev-mode", false, "development mode (API only, no embedded UI)")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }

	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	configPath, err := modulePath(cmdFlags.Args())
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	// Check for user-supplied plugin path
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		c.Ui.Error(fmt.Sprintf("Error loading plugin path: %s", err))
		return 1
	}

	// Load the configuration
	config, diags := c.loadConfig(ctx, configPath)
	if diags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Create and start the server
	server := &inspect.Server{
		Config:  config,
		Address: address,
		Port:    port,
		DevMode: devMode,
	}

	url, err := server.Start()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error starting server: %s", err))
		return 1
	}

	if urlOnly {
		c.Ui.Output(url)
		return 0
	}

	c.Ui.Output(fmt.Sprintf("OpenTofu inspect server started at: %s", url))
	c.Ui.Output("Press Ctrl+C to stop the server")

	// Open browser unless disabled
	if !noBrowser {
		if err := openBrowser(url); err != nil {
			c.Ui.Warn(fmt.Sprintf("Could not open browser automatically: %s", err))
			c.Ui.Warn("Please open the URL manually in your browser")
		}
	}

	// Wait for shutdown signal
	<-c.ShutdownCh
	c.Ui.Output("Shutting down server...")

	return 0
}

func (c *InspectCommand) Help() string {
	helpText := `
Usage: tofu [global options] inspect [options]

  Starts a web server to visualize OpenTofu configuration dependencies
  and relationships through an interactive graph interface.

  The server will start on a random ephemeral port by default and
  automatically open your default browser. The visualization shows
  resources, modules, and their dependencies in an interactive graph.

Options:

  -address=127.0.0.1   IP address to bind to. Use 0.0.0.0 to listen on
                       all interfaces (default: 127.0.0.1)

  -port=0              Port to bind to. Use 0 for a random ephemeral port
                       (default: 0)

  -no-browser          Don't open browser automatically

  -url-only            Output the server URL and exit without starting
                       the interactive server

Examples:

  # Start with default settings (localhost, random port, open browser)
  tofu inspect

  # Start without opening browser
  tofu inspect -no-browser

  # Start on a specific port
  tofu inspect -port 8080

  # Listen on all interfaces
  tofu inspect -address 0.0.0.0

  # Just output the URL for scripting
  tofu inspect -url-only
`
	return strings.TrimSpace(helpText)
}

func (c *InspectCommand) Synopsis() string {
	return "Start a web server to visualize configuration dependencies"
}

// openBrowser attempts to open the given URL in the default browser
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}