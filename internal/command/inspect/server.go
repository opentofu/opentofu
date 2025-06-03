// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package inspect

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/configs"
)

// Server represents the HTTP server for the inspect command
type Server struct {
	Config     *configs.Config
	Address    string
	Port       int
	DevMode    bool
	graph      *Graph
	configRoot string // Root directory of the configuration
}

// Start initializes and starts the HTTP server
func (s *Server) Start() (string, error) {
	// Set config root if not already set
	if s.configRoot == "" {
		s.configRoot = s.Config.Module.SourceDir
	}

	// Build the graph from configuration
	builder := &GraphBuilder{
		config:     s.Config,
		configRoot: s.configRoot,
	}
	graph, err := builder.Build()
	if err != nil {
		return "", fmt.Errorf("failed to build graph: %w", err)
	}
	s.graph = graph

	// If port is 0, use random ephemeral port
	if s.Port == 0 {
		s.Port = getRandomPort()
	}

	mux := http.NewServeMux()

	// API routes with CORS wrapping in dev mode
	if s.DevMode {
		mux.HandleFunc("/api/config", s.corsWrapper(s.handleConfig))
		mux.HandleFunc("/api/graph", s.corsWrapper(s.handleGraph))
		mux.HandleFunc("/api/hierarchy", s.corsWrapper(s.handleHierarchy))
		mux.HandleFunc("/api/resource/", s.corsWrapper(s.handleResource))
		mux.HandleFunc("/api/health", s.corsWrapper(s.handleHealth))
		mux.HandleFunc("/api/source/files", s.corsWrapper(s.handleSourceFiles))
		mux.HandleFunc("/api/source/content", s.corsWrapper(s.handleSourceContent))
	} else {
		mux.HandleFunc("/api/config", s.handleConfig)
		mux.HandleFunc("/api/graph", s.handleGraph)
		mux.HandleFunc("/api/hierarchy", s.handleHierarchy)
		mux.HandleFunc("/api/resource/", s.handleResource)
		mux.HandleFunc("/api/health", s.handleHealth)
		mux.HandleFunc("/api/source/files", s.handleSourceFiles)
		mux.HandleFunc("/api/source/content", s.handleSourceContent)
	}

	// Serve UI based on mode
	if s.DevMode {
		// Development mode: serve a simple dev page with CORS headers
		mux.HandleFunc("/", s.handleDevIndex)
	} else {
		// Production mode: serve embedded React app
		uiHandler := http.FileServer(GetUIFileSystem())
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If requesting the root path and no index.html exists, show dev page
			if r.URL.Path == "/" {
				// Try to serve index.html from embedded filesystem
				if file, err := GetUIFileSystem().Open("index.html"); err == nil {
					file.Close()
					uiHandler.ServeHTTP(w, r)
					return
				}
				// Fallback to development page
				s.handleIndex(w, r)
				return
			}
			// For all other paths, try embedded filesystem first
			uiHandler.ServeHTTP(w, r)
		}))
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.Address, s.Port))
	if err != nil {
		return "", fmt.Errorf("failed to listen on %s:%d: %w", s.Address, s.Port, err)
	}

	url := fmt.Sprintf("http://%s", listener.Addr())

	go func() {
		if err := http.Serve(listener, mux); err != nil {
			// Log error but don't crash - the server might be shutting down
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	return url, nil
}

// getRandomPort returns a random ephemeral port
func getRandomPort() int {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		// Fallback to a port in the ephemeral range
		return 49152
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":      "healthy",
		"config_path": s.Config.Module.SourceDir,
	}
	json.NewEncoder(w).Encode(response)
}

// handleConfig returns the overall configuration structure
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"modules":   extractModules(s.Config),
		"resources": extractResources(s.Config),
		"providers": extractProviders(s.Config),
		"variables": extractVariables(s.Config),
		"outputs":   extractOutputs(s.Config),
	}

	json.NewEncoder(w).Encode(response)
}

// handleGraph returns the dependency graph
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.graph)
}

// handleResource returns detailed information about a specific resource
func (s *Server) handleResource(w http.ResponseWriter, r *http.Request) {
	// Extract resource ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/resource/")
	if path == "" {
		http.Error(w, "Resource ID required", http.StatusBadRequest)
		return
	}

	// Find the resource in our configuration
	resource, moduleConfig := findResourceByIDWithModule(s.Config, path)
	if resource == nil {
		http.Error(w, "Resource not found", http.StatusNotFound)
		return
	}

	// Build module context
	moduleContext := buildModuleContext(moduleConfig, s.Config)

	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"id":           path,
		"type":         resource.Type,
		"name":         resource.Name,
		"mode":         resource.Mode.String(),
		"provider":     resource.Provider.String(),
		"module":       moduleContext,
		"dependencies": extractResourceDependenciesEnhanced(resource, path, s.Config),
		"attributes":   extractResourceAttributes(resource),
	}

	json.NewEncoder(w).Encode(response)
}

// handleHierarchy returns the module hierarchy structure
func (s *Server) handleHierarchy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	hierarchy := buildModuleHierarchy(s.Config)
	response := map[string]interface{}{
		"modules": hierarchy,
	}

	json.NewEncoder(w).Encode(response)
}

// handleIndex serves a simple HTML page for now
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	html := `<!DOCTYPE html>
<html>
<head>
    <title>OpenTofu Inspect</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        .endpoint { background: #f5f5f5; padding: 10px; margin: 10px 0; border-radius: 5px; }
        a { color: #0066cc; text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <h1>OpenTofu Inspect Server</h1>
    <p>The OpenTofu inspect server is running! This is a temporary page while the React UI is being developed.</p>
    
    <h2>Available API Endpoints:</h2>
    <div class="endpoint">
        <strong>GET <a href="/api/health">/api/health</a></strong><br>
        Server health check
    </div>
    <div class="endpoint">
        <strong>GET <a href="/api/config">/api/config</a></strong><br>
        Overall configuration structure
    </div>
    <div class="endpoint">
        <strong>GET <a href="/api/graph">/api/graph</a></strong><br>
        Dependency graph data
    </div>
    <div class="endpoint">
        <strong>GET <a href="/api/hierarchy">/api/hierarchy</a></strong><br>
        Module hierarchy with parent-child relationships
    </div>
    <div class="endpoint">
        <strong>GET /api/resource/{id}</strong><br>
        Detailed resource information with module context
    </div>
    
    <h2>Configuration Summary:</h2>
    <p>Configuration loaded from: <code>` + s.Config.Module.SourceDir + `</code></p>
    <p>Resources found: <strong>` + strconv.Itoa(len(s.Config.Module.ManagedResources)+len(s.Config.Module.DataResources)) + `</strong></p>
    <p>Modules found: <strong>` + strconv.Itoa(len(s.Config.Module.ModuleCalls)) + `</strong></p>
</body>
</html>`
	w.Write([]byte(html))
}

// Helper functions for extracting configuration data
func extractModules(config *configs.Config) []map[string]interface{} {
	var modules []map[string]interface{}

	// Add root module information
	modules = append(modules, map[string]interface{}{
		"id":     "module.root",
		"name":   "root",
		"path":   "",
		"source": ".",
		"calls":  extractModuleCalls(config.Module),
	})

	config.DeepEach(func(c *configs.Config) {
		if c != config { // Skip root module
			moduleID := "module." + c.Path.String()
			modules = append(modules, map[string]interface{}{
				"id":     moduleID,
				"name":   c.Path[len(c.Path)-1],
				"path":   moduleID,
				"source": c.SourceAddrRaw,
				"calls":  extractModuleCalls(c.Module),
			})
		}
	})

	return modules
}

func extractModuleCalls(module *configs.Module) []map[string]interface{} {
	var calls []map[string]interface{}

	for name, call := range module.ModuleCalls {
		callInfo := map[string]interface{}{
			"name":   name,
			"source": call.SourceAddrRaw,
		}

		// Extract dependencies from module call
		dependencies := []string{}
		for _, dep := range call.DependsOn {
			dependencies = append(dependencies, dep.RootName())
		}
		if len(dependencies) > 0 {
			callInfo["dependencies"] = dependencies
		}

		calls = append(calls, callInfo)
	}

	return calls
}

func extractResources(config *configs.Config) []map[string]interface{} {
	var resources []map[string]interface{}

	config.DeepEach(func(c *configs.Config) {
		pathPrefix := ""
		modulePath := ""
		var parentID *string

		if len(c.Path) > 0 {
			pathPrefix = c.Path.String() + "."
			modulePath = "module." + c.Path.String()
			parentID = &modulePath
		} else {
			rootModuleID := "module.root"
			parentID = &rootModuleID
		}

		// Build module context for this resource's module
		moduleContext := buildModuleContext(c, config)

		for _, res := range c.Module.ManagedResources {
			dependencies := extractResourceDependencies(res)
			resources = append(resources, map[string]interface{}{
				"id":           pathPrefix + res.Addr().String(),
				"type":         res.Type,
				"name":         res.Name,
				"mode":         res.Mode.String(),
				"provider":     res.Provider.String(),
				"parentId":     parentID,
				"module":       moduleContext,
				"dependencies": dependencies,
			})
		}

		for _, res := range c.Module.DataResources {
			dependencies := extractResourceDependencies(res)
			resources = append(resources, map[string]interface{}{
				"id":           pathPrefix + res.Addr().String(),
				"type":         res.Type,
				"name":         res.Name,
				"mode":         res.Mode.String(),
				"provider":     res.Provider.String(),
				"parentId":     parentID,
				"module":       moduleContext,
				"dependencies": dependencies,
			})
		}
	})

	return resources
}

func extractProviders(config *configs.Config) []map[string]interface{} {
	var providers []map[string]interface{}

	for _, provider := range config.Module.ProviderConfigs {
		providers = append(providers, map[string]interface{}{
			"name":  provider.Name,
			"alias": provider.Alias,
		})
	}

	return providers
}

func extractVariables(config *configs.Config) []map[string]interface{} {
	var variables []map[string]interface{}

	config.DeepEach(func(c *configs.Config) {
		pathPrefix := ""
		var parentID *string

		if len(c.Path) > 0 {
			pathPrefix = c.Path.String() + "."
			modulePath := "module." + c.Path.String()
			parentID = &modulePath
		} else {
			rootModuleID := "module.root"
			parentID = &rootModuleID
		}

		// Build module context for this variable's module
		moduleContext := buildModuleContext(c, config)

		for name, variable := range c.Module.Variables {
			variables = append(variables, map[string]interface{}{
				"id":          pathPrefix + name,
				"name":        name,
				"description": variable.Description,
				"sensitive":   variable.Sensitive,
				"parentId":    parentID,
				"module":      moduleContext,
			})
		}
	})

	return variables
}

func extractOutputs(config *configs.Config) []map[string]interface{} {
	var outputs []map[string]interface{}

	config.DeepEach(func(c *configs.Config) {
		pathPrefix := ""
		var parentID *string

		if len(c.Path) > 0 {
			pathPrefix = c.Path.String() + "."
			modulePath := "module." + c.Path.String()
			parentID = &modulePath
		} else {
			rootModuleID := "module.root"
			parentID = &rootModuleID
		}

		// Build module context for this output's module
		moduleContext := buildModuleContext(c, config)

		for name, output := range c.Module.Outputs {
			outputs = append(outputs, map[string]interface{}{
				"id":          pathPrefix + name,
				"name":        name,
				"description": output.Description,
				"sensitive":   output.Sensitive,
				"parentId":    parentID,
				"module":      moduleContext,
			})
		}
	})

	return outputs
}

func findResourceByID(config *configs.Config, id string) *configs.Resource {
	var result *configs.Resource

	config.DeepEach(func(c *configs.Config) {
		pathPrefix := ""
		if len(c.Path) > 0 {
			pathPrefix = c.Path.String() + "."
		}

		for _, res := range c.Module.ManagedResources {
			if pathPrefix+res.Addr().String() == id {
				result = res
				return
			}
		}

		for _, res := range c.Module.DataResources {
			if pathPrefix+res.Addr().String() == id {
				result = res
				return
			}
		}
	})

	return result
}

func extractResourceDependencies(resource *configs.Resource) map[string]interface{} {
	dependencies := map[string]interface{}{
		"explicit": []string{},
		"implicit": []string{},
	}

	// Add explicit dependencies from depends_on
	explicitDeps := []string{}
	for _, dep := range resource.DependsOn {
		explicitDeps = append(explicitDeps, dep.RootName())
	}
	dependencies["explicit"] = explicitDeps

	// Extract implicit dependencies from configuration body
	// This is a simplified approach - ideally we'd use HCL's reference extraction
	implicitDeps := []string{}
	if resource.Config != nil {
		// Get the variables referenced in the config
		// This is a basic implementation - in a full version we'd use
		// the lang package to properly extract references

		// For now, we'll return empty implicit deps and implement this properly later
		// when we integrate with OpenTofu's actual dependency resolution
	}
	dependencies["implicit"] = implicitDeps

	return dependencies
}

// Enhanced resource dependency extraction with cross-module dependencies
func extractResourceDependenciesEnhanced(resource *configs.Resource, resourceID string, config *configs.Config) map[string]interface{} {
	dependencies := map[string]interface{}{
		"explicit":    []string{},
		"implicit":    []string{},
		"crossModule": []string{},
	}

	// Add explicit dependencies from depends_on
	explicitDeps := []string{}
	crossModuleDeps := []string{}
	currentModulePath := getModulePathFromResourceID(resourceID)

	for _, dep := range resource.DependsOn {
		depName := dep.RootName()
		explicitDeps = append(explicitDeps, depName)

		// Check if this dependency is cross-module
		if isCrossModuleDependency(depName, currentModulePath) {
			crossModuleDeps = append(crossModuleDeps, depName)
		}
	}

	dependencies["explicit"] = explicitDeps
	dependencies["crossModule"] = crossModuleDeps

	// For implicit dependencies, we'd need HCL parsing
	dependencies["implicit"] = []string{}

	return dependencies
}

// Extract resource attributes (simplified)
func extractResourceAttributes(resource *configs.Resource) map[string]interface{} {
	attributes := make(map[string]interface{})

	// This would require HCL parsing to extract actual attribute values
	// For now, we'll just return the resource type and name
	attributes["resource_type"] = resource.Type
	attributes["name"] = resource.Name

	return attributes
}

// Find resource by ID and return both resource and its module config
func findResourceByIDWithModule(config *configs.Config, id string) (*configs.Resource, *configs.Config) {
	var result *configs.Resource
	var moduleConfig *configs.Config

	config.DeepEach(func(c *configs.Config) {
		pathPrefix := ""
		if len(c.Path) > 0 {
			pathPrefix = c.Path.String() + "."
		}

		for _, res := range c.Module.ManagedResources {
			if pathPrefix+res.Addr().String() == id {
				result = res
				moduleConfig = c
				return
			}
		}

		for _, res := range c.Module.DataResources {
			if pathPrefix+res.Addr().String() == id {
				result = res
				moduleConfig = c
				return
			}
		}
	})

	return result, moduleConfig
}

// Build module context for a resource
func buildModuleContext(moduleConfig *configs.Config, rootConfig *configs.Config) map[string]interface{} {
	if moduleConfig == nil {
		return map[string]interface{}{
			"path":         "",
			"name":         "root",
			"source":       ".",
			"depth":        0,
			"parent":       nil,
			"ancestorPath": []string{"root"},
		}
	}

	path := moduleConfig.Path.String()
	name := "root"
	if len(moduleConfig.Path) > 0 {
		name = moduleConfig.Path[len(moduleConfig.Path)-1]
	}

	// Calculate depth and parent
	depth := len(moduleConfig.Path)
	var parent *string
	if depth > 1 {
		parentPath := strings.Join(moduleConfig.Path[:depth-1], ".")
		parent = &parentPath
	}

	// Build ancestor path
	ancestorPath := []string{"root"}
	for _, pathPart := range moduleConfig.Path {
		ancestorPath = append(ancestorPath, pathPart)
	}

	return map[string]interface{}{
		"path":         path,
		"name":         name,
		"source":       moduleConfig.SourceAddrRaw,
		"depth":        depth,
		"parent":       parent,
		"ancestorPath": ancestorPath,
	}
}

// Build complete module hierarchy
func buildModuleHierarchy(config *configs.Config) map[string]interface{} {
	hierarchy := make(map[string]interface{})

	// Add root module
	rootChildren := buildModuleChildren(config, "")
	hierarchy["root"] = map[string]interface{}{
		"id":        "module.root",
		"name":      "root",
		"path":      "",
		"source":    ".",
		"depth":     0,
		"parent":    nil,
		"children":  rootChildren,
		"variables": extractVariableNames(config.Module),
		"outputs":   extractOutputNames(config.Module),
		"calls":     extractModuleCallsEnhanced(config.Module),
	}

	// Add all other modules
	config.DeepEach(func(c *configs.Config) {
		if c != config { // Skip root module
			moduleID := c.Path.String()
			name := c.Path[len(c.Path)-1]
			depth := len(c.Path)

			var parent *string
			if depth > 1 {
				parentPath := "module." + strings.Join(c.Path[:depth-1], ".")
				parent = &parentPath
			} else {
				parentStr := "module.root"
				parent = &parentStr
			}

			children := buildModuleChildren(config, moduleID)

			hierarchy[moduleID] = map[string]interface{}{
				"id":        "module." + moduleID,
				"name":      name,
				"path":      "module." + moduleID,
				"source":    c.SourceAddrRaw,
				"depth":     depth,
				"parent":    parent,
				"children":  children,
				"variables": extractVariableNames(c.Module),
				"outputs":   extractOutputNames(c.Module),
				"calls":     extractModuleCallsEnhanced(c.Module),
			}
		}
	})

	return hierarchy
}

// Build children lists for a module
func buildModuleChildren(config *configs.Config, modulePath string) map[string]interface{} {
	moduleChildren := []string{}
	resourceChildren := []string{}

	// Find child modules
	config.DeepEach(func(c *configs.Config) {
		if len(c.Path) > 0 {
			currentPath := c.Path.String()

			// Check if this is a direct child of the target module
			if modulePath == "" {
				// Root module children
				if len(c.Path) == 1 {
					moduleChildren = append(moduleChildren, "module."+currentPath)
				}
			} else {
				// Non-root module children
				if strings.HasPrefix(currentPath, modulePath+".") {
					remainingPath := strings.TrimPrefix(currentPath, modulePath+".")
					if !strings.Contains(remainingPath, ".") {
						moduleChildren = append(moduleChildren, "module."+currentPath)
					}
				}
			}
		}
	})

	// Find child resources
	config.DeepEach(func(c *configs.Config) {
		currentModulePath := c.Path.String()

		if currentModulePath == modulePath {
			// Add resources from this exact module
			for _, res := range c.Module.ManagedResources {
				if modulePath == "" {
					resourceChildren = append(resourceChildren, res.Addr().String())
				} else {
					resourceChildren = append(resourceChildren, modulePath+"."+res.Addr().String())
				}
			}

			for _, res := range c.Module.DataResources {
				if modulePath == "" {
					resourceChildren = append(resourceChildren, res.Addr().String())
				} else {
					resourceChildren = append(resourceChildren, modulePath+"."+res.Addr().String())
				}
			}
		}
	})

	return map[string]interface{}{
		"modules":   moduleChildren,
		"resources": resourceChildren,
	}
}

// Extract variable names from a module
func extractVariableNames(module *configs.Module) []string {
	names := []string{}
	for name := range module.Variables {
		names = append(names, name)
	}
	return names
}

// Extract output names from a module
func extractOutputNames(module *configs.Module) []string {
	names := []string{}
	for name := range module.Outputs {
		names = append(names, name)
	}
	return names
}

// Enhanced module call extraction with inputs
func extractModuleCallsEnhanced(module *configs.Module) []map[string]interface{} {
	var calls []map[string]interface{}

	for name, call := range module.ModuleCalls {
		callInfo := map[string]interface{}{
			"name":    name,
			"source":  call.SourceAddrRaw,
			"version": nil,                          // Would need to extract from version constraints
			"inputs":  make(map[string]interface{}), // Would need HCL parsing
		}

		// Extract dependencies from module call
		dependencies := []string{}
		for _, dep := range call.DependsOn {
			dependencies = append(dependencies, dep.RootName())
		}
		if len(dependencies) > 0 {
			callInfo["dependencies"] = dependencies
		}

		calls = append(calls, callInfo)
	}

	return calls
}

// Helper functions
func getModulePathFromResourceID(resourceID string) string {
	parts := strings.Split(resourceID, ".")
	if len(parts) >= 3 && parts[0] == "module" {
		// Find the module path by removing the resource type and name
		for i := len(parts) - 2; i >= 1; i-- {
			candidate := strings.Join(parts[:i+1], ".")
			if strings.HasPrefix(candidate, "module.") {
				return candidate
			}
		}
	}
	return ""
}

func isCrossModuleDependency(dependency, currentModulePath string) bool {
	if currentModulePath == "" {
		// Root module - check if dependency is in a module
		return strings.HasPrefix(dependency, "module.")
	}

	// Non-root module - check if dependency is outside current module
	return !strings.HasPrefix(dependency, currentModulePath+".")
}

// corsWrapper wraps a handler function with CORS headers for development mode
func (s *Server) corsWrapper(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// handleDevIndex serves a development page explaining how to run the React app
func (s *Server) handleDevIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	html := `<!DOCTYPE html>
<html>
<head>
    <title>OpenTofu Inspect - Development Mode</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; max-width: 900px; margin: 50px auto; padding: 20px; line-height: 1.6; }
        .hero { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; border-radius: 10px; margin-bottom: 30px; }
        .endpoint { background: #f8f9fa; padding: 15px; margin: 15px 0; border-radius: 8px; border-left: 4px solid #007acc; }
        .code { background: #2d3748; color: #e2e8f0; padding: 15px; border-radius: 8px; font-family: 'Monaco', 'Consolas', monospace; margin: 10px 0; }
        .step { background: #e3f2fd; padding: 20px; margin: 20px 0; border-radius: 8px; border-left: 4px solid #2196f3; }
        a { color: #007acc; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .status { display: inline-block; background: #4caf50; color: white; padding: 5px 10px; border-radius: 15px; font-size: 0.9em; }
    </style>
</head>
<body>
    <div class="hero">
        <h1>🚀 OpenTofu Inspect - Development Mode</h1>
        <p>API server is running! <span class="status">Ready for React development</span></p>
    </div>
    
    <div class="step">
        <h3>📦 Step 1: Start the React Development Server</h3>
        <p>In a new terminal, navigate to the UI directory and start the development server:</p>
        <div class="code">cd internal/command/inspect/ui<br>npm run dev</div>
        <p>This will start the React app with Hot Module Replacement (HMR) on <a href="http://localhost:5173" target="_blank">http://localhost:5173</a></p>
    </div>

    <div class="step">
        <h3>🔄 Step 2: Configure API Base URL</h3>
        <p>The React app should automatically connect to this API server. The API base URL is:</p>
        <div class="code">http://` + s.Address + `:` + fmt.Sprintf("%d", getPortFromListener()) + `</div>
    </div>

    <h2>📡 Available API Endpoints:</h2>
    <div class="endpoint">
        <strong>GET <a href="/api/health">/api/health</a></strong><br>
        Server health check and configuration path
    </div>
    <div class="endpoint">
        <strong>GET <a href="/api/config">/api/config</a></strong><br>
        Configuration structure with modules, resources, providers
    </div>
    <div class="endpoint">
        <strong>GET <a href="/api/graph">/api/graph</a></strong><br>
        Dependency graph data for React Flow visualization
    </div>
    <div class="endpoint">
        <strong>GET <a href="/api/hierarchy">/api/hierarchy</a></strong><br>
        Module hierarchy with parent-child relationships for sub flows
    </div>
    <div class="endpoint">
        <strong>GET /api/resource/{id}</strong><br>
        Detailed information about a specific resource with module context
    </div>

    <h2>🛠️ Development Workflow</h2>
    <ul>
        <li>✅ API server running (this server)</li>
        <li>📱 React app with HMR: <code>npm run dev</code></li>
        <li>🔍 Real-time code changes without rebuilding Go binary</li>
        <li>🌐 CORS enabled for cross-origin requests</li>
    </ul>
</body>
</html>`
	w.Write([]byte(html))
}

// Helper to get port from listener (placeholder)
func getPortFromListener() int {
	// This would need to be properly implemented to get the actual port
	return 8080
}

// handleSourceFiles returns a list of all source files in the configuration
func (s *Server) handleSourceFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	files := []map[string]interface{}{}

	// Walk the configuration directory to find all .tf files
	err := filepath.WalkDir(s.configRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-.tf files
		if d.IsDir() || !strings.HasSuffix(path, ".tf") {
			return nil
		}

		// Get relative path from config root
		relPath, err := filepath.Rel(s.configRoot, path)
		if err != nil {
			relPath = path
		}

		// Get file info
		info, err := d.Info()
		if err != nil {
			return err
		}

		// Count lines in file
		lineCount := 0
		if content, err := os.ReadFile(path); err == nil {
			lineCount = strings.Count(string(content), "\n") + 1
		}

		files = append(files, map[string]interface{}{
			"path":  relPath,
			"size":  info.Size(),
			"lines": lineCount,
		})

		return nil
	})

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to scan source files: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"files": files,
	}

	json.NewEncoder(w).Encode(response)
}

// handleSourceContent returns the content of a specific source file
func (s *Server) handleSourceContent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get file parameter
	filename := r.URL.Query().Get("file")
	if filename == "" {
		http.Error(w, "Missing 'file' parameter", http.StatusBadRequest)
		return
	}

	// Security check: ensure file is within config root
	// Clean the paths to handle any ".." or other path traversal attempts
	cleanConfigRoot, err := filepath.Abs(s.configRoot)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	fullPath := filepath.Join(cleanConfigRoot, filename)
	cleanFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// Ensure the config root ends with a separator to prevent partial matches
	configRootWithSep := cleanConfigRoot
	if !strings.HasSuffix(configRootWithSep, string(filepath.Separator)) {
		configRootWithSep += string(filepath.Separator)
	}

	if !strings.HasPrefix(cleanFullPath, configRootWithSep) && cleanFullPath != cleanConfigRoot {
		http.Error(w, "File path not allowed", http.StatusForbidden)
		return
	}

	// Check if file exists and is a .tf file
	if !strings.HasSuffix(filename, ".tf") {
		http.Error(w, "Only .tf files are allowed", http.StatusBadRequest)
		return
	}

	// Read file content
	content, err := os.ReadFile(cleanFullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Failed to read file: %v", err), http.StatusInternalServerError)
		}
		return
	}

	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")

	// Handle line range parameters
	startLine := 1
	startCol := 0
	endLine := -1

	if startLineStr := r.URL.Query().Get("startLine"); startLineStr != "" {
		if start, err := strconv.Atoi(startLineStr); err == nil && start > 0 {
			startLine = start
		}
	}

	if startColStr := r.URL.Query().Get("startCol"); startColStr != "" {
		if start, err := strconv.Atoi(startColStr); err == nil && start > 0 {
			startCol = start
		}
	}

	file, _ := hclsyntax.ParseConfig(
		content,
		filename,
		hcl.Pos{Line: 0, Column: 0},
	)

	bodySyntax := file.Body.(*hclsyntax.Body)
	for _, block := range bodySyntax.Blocks {
		if block.Range().Start.Line == startLine-1 && block.Range().Start.Column == startCol {
			endLine = block.CloseBraceRange.Start.Line + 1
		}
	}

	if endLine == -1 {
		http.Error(w, "Resource/Module not found", http.StatusNotFound)
		return
	}

	// Validate line range
	if startLine > len(lines) {
		startLine = len(lines)
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine {
		startLine = endLine
	}

	// Extract requested lines (convert to 0-based indexing)
	var selectedLines []string
	var selectedContent string

	if startLine > 0 && endLine > 0 {
		start := startLine - 1
		end := endLine
		if end > len(lines) {
			end = len(lines)
		}
		selectedLines = lines[start:end]
		selectedContent = strings.Join(selectedLines, "\n")
	}

	response := map[string]interface{}{
		"filename":   filename,
		"content":    selectedContent,
		"lines":      selectedLines,
		"startLine":  startLine,
		"endLine":    endLine,
		"totalLines": len(lines),
	}

	// If no line range specified, include full content
	if r.URL.Query().Get("startLine") == "" && r.URL.Query().Get("startCol") == "" {
		response["content"] = contentStr
		response["lines"] = lines
	}

	json.NewEncoder(w).Encode(response)
}
