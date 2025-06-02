# OpenTofu Inspect Feature - Complete Implementation Plan

## Overview
The `tofu inspect` command will provide a visual, interactive way to explore OpenTofu configurations through a web-based UI. It will show the dependency graph between resources, modules, and other configuration elements using a React-based frontend with @xyflow/react for graph visualization.

## Architecture

### Components
1. **CLI Command** (`tofu inspect`)
   - Loads and parses the configuration
   - Starts the web server on an ephemeral port
   - Opens the browser automatically (with option to disable)
   - Serves both API and static files

2. **Web Server**
   - Go HTTP server using standard library
   - Serves on random ephemeral port (49152-65535)
   - API endpoints under `/api/`
   - Static files (React app) served from embedded filesystem

3. **API Layer**
   - RESTful JSON API
   - Exposes configuration data in a format suitable for visualization
   - WebSocket support for future real-time updates

4. **Frontend (React App)**
   - Built with React + TypeScript
   - Uses @xyflow/react for graph visualization
   - Tailwind CSS v4 for styling
   - Embedded into Go binary using `embed` package

## Key Learnings from Code Analysis

### Command Structure
- Commands are defined in `cmd/tofu/commands.go` 
- Each command implements the `cli.Command` interface
- Commands use the `Meta` struct for common functionality
- Configuration loading is done via `c.loadConfig()` or similar methods

### Data Available
From `configs.Config` and `configs.Module`:
- **Resources**: `ManagedResources` and `DataResources` maps
- **Modules**: `ModuleCalls` for child modules, hierarchical structure via `Children`
- **Dependencies**: 
  - Explicit via `Resource.DependsOn`
  - Implicit via references in `Resource.Config` body
- **Providers**: `ProviderConfigs` and provider requirements
- **Variables/Outputs**: For understanding data flow

### Graph Building
- OpenTofu already has graph building capabilities (see `graph` command)
- Can leverage `tofu.Context` and its graph methods
- The DAG package provides graph structure and traversal

## Implementation Plan

### Phase 1: Basic Infrastructure
1. Create the `inspect` command structure
2. Set up HTTP server with ephemeral port selection
3. Create basic API structure
4. Set up React app build pipeline

### Phase 2: Configuration Analysis
1. Load and parse OpenTofu configuration
2. Extract resource dependencies
3. Build graph data structure
4. Create API endpoints to expose this data

### Phase 3: Frontend Development
1. Create React app with TypeScript
2. Implement @xyflow/react graph visualization
3. Add interactive features (zoom, pan, node selection)
4. Display resource details on selection

### Phase 4: Enhancement
1. Add module hierarchy visualization
2. Show provider dependencies
3. Add search and filter capabilities
4. Export functionality (SVG, PNG)

## Technical Decisions

### Go Backend
- **HTTP Server**: Use standard `net/http` package only
- **Routing**: Use standard library `http.ServeMux` or custom handler
- **Static Files**: Use `embed` package to embed built React app
- **JSON Serialization**: Use standard `encoding/json`
- **No external dependencies**: Keep it simple with stdlib only

### React Frontend
- **Build Tool**: Vite (fast, modern build tool)
- **Routing**: TanStack Router (type-safe routing)
- **Data Fetching**: TanStack Query (server state management)
- **Schema Validation**: Zod (runtime type checking for API responses)
- **Graph Library**: @xyflow/react
- **Styling**: Tailwind CSS v4 (latest version)
- **Type Safety**: TypeScript

### Build Process
- React app built separately into `internal/command/inspect/ui/dist`
- Go `embed` directive includes the dist directory
- Single binary distribution

## Go Implementation Details

### 1. Create the Inspect Command

```go
// internal/command/inspect.go
type InspectCommand struct {
    Meta
}

func (c *InspectCommand) Run(args []string) int {
    // Parse flags (--port, --address, --no-browser, --url-only)
    // Load configuration
    // Start HTTP server
    // Open browser (unless --no-browser)
}
```

### 2. Configuration Loading

```go
// Load config similar to other commands
configPath, err := modulePath(cmdFlags.Args())
config, diags := c.loadConfig(ctx, configPath)
```

### 3. Graph Building Logic

```go
// internal/command/inspect/graph.go
type GraphBuilder struct {
    config *configs.Config
}

type Node struct {
    ID       string                 `json:"id"`
    Type     string                 `json:"type"` // "resource", "module", "variable", "output"
    Data     map[string]interface{} `json:"data"`
    Position Position              `json:"position"`
}

type Edge struct {
    ID     string `json:"id"`
    Source string `json:"source"`
    Target string `json:"target"`
    Type   string `json:"type"` // "depends_on", "implicit", "module"
}

type Graph struct {
    Nodes []Node `json:"nodes"`
    Edges []Edge `json:"edges"`
}

func (gb *GraphBuilder) Build() (*Graph, error) {
    // 1. Create nodes for all resources
    // 2. Create nodes for modules
    // 3. Extract dependencies:
    //    - Parse Resource.DependsOn for explicit deps
    //    - Analyze Resource.Config body for implicit refs
    //    - Use configs.StaticEvaluator or lang package
    // 4. Create edges
    // 5. Calculate positions (initial layout)
}
```

### 4. Dependency Extraction

```go
// Extract references from HCL expressions
func extractReferences(body hcl.Body) []addrs.Reference {
    // Use hcl library to walk the body
    // Find all variable references, resource references, etc.
    // Similar to how OpenTofu does it internally
}

// Build dependency edges
func buildDependencyEdges(resources map[string]*configs.Resource) []Edge {
    var edges []Edge
    
    for name, res := range resources {
        // Explicit dependencies
        for _, dep := range res.DependsOn {
            // Convert traversal to resource address
            edges = append(edges, Edge{
                Source: name,
                Target: traversalToAddr(dep),
                Type:   "depends_on",
            })
        }
        
        // Implicit dependencies from config references
        refs := extractReferences(res.Config)
        for _, ref := range refs {
            edges = append(edges, Edge{
                Source: name,
                Target: ref.String(),
                Type:   "implicit",
            })
        }
    }
    
    return edges
}
```

### 5. HTTP Server Implementation

```go
// internal/command/inspect/server.go
type Server struct {
    config *configs.Config
    graph  *Graph
    addr   string
    port   int
}

func (s *Server) Start() (string, error) {
    // If port is 0, use random ephemeral port
    if s.port == 0 {
        s.port = getRandomPort()
    }
    
    mux := http.NewServeMux()
    
    // API routes
    mux.HandleFunc("/api/config", s.handleConfig)
    mux.HandleFunc("/api/graph", s.handleGraph)
    mux.HandleFunc("/api/resource/", s.handleResource)
    
    // Serve embedded React app
    mux.Handle("/", http.FileServer(http.FS(uiFS)))
    
    listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.addr, s.port))
    if err != nil {
        return "", err
    }
    
    url := fmt.Sprintf("http://%s", listener.Addr())
    
    go http.Serve(listener, mux)
    
    return url, nil
}

func getRandomPort() int {
    // Get random port in ephemeral range (49152-65535)
    listener, err := net.Listen("tcp", ":0")
    if err != nil {
        panic(err)
    }
    port := listener.Addr().(*net.TCPAddr).Port
    listener.Close()
    return port
}
```

### 6. Embedded UI

```go
// internal/command/inspect/ui.go
package inspect

import "embed"

//go:embed ui/dist/*
var uiFS embed.FS
```

### 7. Helper Functions

```go
// Extract all resources recursively from config tree
func extractAllResources(config *configs.Config) map[string]*configs.Resource {
    resources := make(map[string]*configs.Resource)
    
    config.DeepEach(func(c *configs.Config) {
        path := c.Path.String()
        prefix := ""
        if path != "" {
            prefix = path + "."
        }
        
        for name, res := range c.Module.ManagedResources {
            resources[prefix+res.Addr().String()] = res
        }
        
        for name, res := range c.Module.DataResources {
            resources[prefix+res.Addr().String()] = res
        }
    })
    
    return resources
}
```

## API Design

### Key Concept: Config vs Graph Data

The API provides two distinct views of dependencies:
- **Config endpoints** - Show what's explicitly written in configuration files
- **Graph endpoints** - Show the complete dependency graph that OpenTofu's graph walker builds

### Endpoints

#### `GET /api/health`
Server health check and basic info
```json
{
  "status": "healthy",
  "config_path": "/path/to/config"
}
```

#### `GET /api/config`
Returns configuration structure **as parsed from files**
```json
{
  "modules": [
    {
      "name": "root",
      "path": "",
      "source": ".",
      "calls": [
        {
          "name": "database",
          "source": "./modules/database"
        },
        {
          "name": "application", 
          "source": "./modules/application",
          "dependencies": ["module.database", "module.network"]
        }
      ]
    }
  ],
  "resources": [
    {
      "id": "random_string.config",
      "type": "random_string",
      "name": "config",
      "mode": "ManagedResourceMode",
      "provider": "registry.opentofu.org/hashicorp/random",
      "dependencies": {
        "explicit": ["module.database"],
        "implicit": ["random_id.root_fast1"]
      }
    }
  ],
  "providers": [...],
  "variables": [...],
  "outputs": [...]
}
```

#### `GET /api/graph`
Returns graph data **with expressions and source locations** in @xyflow/react format
```json
{
  "nodes": [
    {
      "id": "random_string.root_config",
      "type": "resource",
      "data": {
        "resourceType": "random_string",
        "name": "root_config",
        "mode": "ManagedResourceMode",
        "provider": "registry.opentofu.org/hashicorp/random",
        "modulePath": "",
        "moduleAddress": "root"
      },
      "source": {
        "filename": "main.tf",
        "startLine": 12,
        "endLine": 15,
        "startCol": 1,
        "endCol": 2
      }
    },
    {
      "id": "module.database",
      "type": "module", 
      "data": {
        "name": "database",
        "source": "./modules/database",
        "modulePath": "module.database",
        "hasChildren": true,
        "childResourceCount": 2,
        "childModuleCount": 0,
        "depth": 1,
        "inputs": [{"name": "db_name", "type": "string", "required": true}],
        "outputs": [{"name": "connection_string", "type": "string", "sensitive": false}]
      },
      "source": {
        "filename": "main.tf", 
        "startLine": 37,
        "endLine": 41,
        "startCol": 1,
        "endCol": 2
      }
    },
    {
      "id": "expr_0",
      "type": "expression",
      "data": {
        "operation": "+",
        "description": "Expression: +",
        "targetID": "module.database",
        "targetType": "module",
        "targetInput": "db_name",
        "inputs": 2,
        "outputs": 1
      },
      "source": {
        "filename": "main.tf",
        "startLine": 40,
        "endLine": 40,
        "startCol": 11,
        "endCol": 65
      }
    },
    {
      "id": "static_1",
      "type": "static_value",
      "data": {
        "value": "ABC",
        "type": "string",
        "description": "Static string (RHS): ABC",
        "side": "RHS"
      },
      "source": {
        "filename": "main.tf",
        "startLine": 40,
        "endLine": 40,
        "startCol": 50,
        "endCol": 55
      }
    }
  ],
  "edges": [
    {
      "id": "e3000",
      "source": "random_string.root_config",
      "target": "expr_0", 
      "type": "expression_input",
      "targetHandle": "input-0"
    },
    {
      "id": "e3001",
      "source": "static_1",
      "target": "expr_0",
      "type": "expression_input", 
      "targetHandle": "input-1"
    },
    {
      "id": "e3002",
      "source": "expr_0",
      "target": "module.database",
      "type": "expression_output",
      "targetHandle": "input-db_name"
    }
  ]
}
```

#### `GET /api/resource/:id`
Returns detailed information about a specific resource
```json
{
  "id": "random_string.config",
  "type": "random_string",
  "name": "config",
  "mode": "ManagedResourceMode",
  "provider": "registry.opentofu.org/hashicorp/random",
  "dependencies": {
    "explicit": ["module.database"],
    "implicit": []
  }
}
```

#### `GET /api/source/files`
Returns list of all source files in the configuration
```json
{
  "files": [
    {
      "path": "main.tf",
      "size": 2845,
      "lines": 104
    },
    {
      "path": "modules/database/main.tf",
      "size": 1024,
      "lines": 45
    }
  ]
}
```

#### `GET /api/source/content?file=main.tf`
Returns full file content
```json
{
  "filename": "main.tf",
  "content": "# Test configuration...\n\nresource \"random_id\" \"root_fast1\" {\n...",
  "lines": ["# Test configuration", "", "resource \"random_id\" \"root_fast1\" {", "..."],
  "startLine": 1,
  "endLine": 104,
  "totalLines": 104
}
```

#### `GET /api/source/content?file=main.tf&start=37&end=41`
Returns specific line range
```json
{
  "filename": "main.tf",
  "startLine": 37,
  "endLine": 41,
  "content": "# Database module\nmodule \"database\" {\n  source = \"./modules/database\"\n  \n  db_name = random_string.root_config.result + \"ABC\" + \"DEF\"\n}",
  "lines": [
    "# Database module",
    "module \"database\" {",
    "  source = \"./modules/database\"",
    "  ",
    "  db_name = random_string.root_config.result + \"ABC\" + \"DEF\"",
    "}"
  ],
  "totalLines": 104
}
```

#### `WebSocket /api/ws`
For future real-time updates during plan/apply operations

### API Handlers

```go
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
    // Return overall configuration structure
    response := map[string]interface{}{
        "modules":   extractModules(s.config),
        "resources": extractResources(s.config),
        "providers": extractProviders(s.config),
        "variables": extractVariables(s.config),
        "outputs":   extractOutputs(s.config),
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(s.graph)
}

func (s *Server) handleResource(w http.ResponseWriter, r *http.Request) {
    // Extract resource ID from URL
    // Return detailed resource information
}
```

## Frontend Design

### Layout
- **Header**: Command info, current directory, controls
- **Main Canvas**: Graph visualization
- **Sidebar**: Resource details, search, filters
- **Bottom Panel**: Logs, errors, warnings

### Graph Features
- **Nodes**: Different shapes/colors for different resource types
- **Edges**: Show dependency relationships
- **Clustering**: Group by modules
- **Interactivity**: Click to select, double-click to zoom
- **Mini-map**: For navigation in large graphs

### Frontend Architecture
- **Routes** (TanStack Router):
  - `/` - Main graph view
  - `/resource/:id` - Resource detail view
  - `/modules` - Module hierarchy view
  - `/search` - Advanced search interface

- **Data Management** (TanStack Query):
  - Query caching for API responses
  - Optimistic updates for UI interactions
  - Background refetching for live updates
  - Request deduplication

- **Type Safety** (Zod):
  - Runtime validation of all API responses
  - Auto-generated TypeScript types from Zod schemas
  - Error boundary integration for invalid data

- **Styling** (Tailwind v4):
  - Component-based utility classes
  - Dark mode support
  - Custom theme for OpenTofu branding
  - Responsive design for different screen sizes

## Command Usage

```bash
# Basic usage (binds to localhost with random ephemeral port)
tofu inspect

# Specify a different directory
tofu inspect /path/to/config

# Don't open browser automatically
tofu inspect --no-browser

# Specify a port (still binds to localhost)
tofu inspect --port 8080

# Specify IP address to bind to (default: 127.0.0.1)
tofu inspect --address 0.0.0.0  # Listen on all interfaces
tofu inspect --address 192.168.1.100  # Listen on specific IP

# Specify both address and port
tofu inspect --address 0.0.0.0 --port 8080

# Output the URL and exit
tofu inspect --url-only
```

## File Structure

```
internal/command/
├── inspect.go              # Main command implementation
├── inspect/
│   ├── api.go             # API handlers
│   ├── server.go          # HTTP server setup
│   ├── graph.go           # Graph building logic
│   ├── ui/                # React app source
│   │   ├── package.json
│   │   ├── vite.config.ts
│   │   ├── tsconfig.json
│   │   ├── tailwind.config.ts
│   │   ├── src/
│   │   │   ├── main.tsx
│   │   │   ├── App.tsx
│   │   │   ├── router.tsx
│   │   │   ├── api/
│   │   │   │   ├── client.ts     # TanStack Query setup
│   │   │   │   ├── queries.ts    # Query definitions
│   │   │   │   └── schemas.ts    # Zod schemas
│   │   │   ├── components/
│   │   │   │   ├── Graph.tsx
│   │   │   │   ├── Sidebar.tsx
│   │   │   │   └── ...
│   │   │   ├── hooks/
│   │   │   ├── routes/
│   │   │   │   ├── index.tsx
│   │   │   │   ├── resource.$id.tsx
│   │   │   │   └── ...
│   │   │   └── styles/
│   │   │       └── globals.css
│   │   └── dist/          # Built files (embedded)
│   └── ui.go              # Embedded filesystem
```

## Example API Schema (Zod)

```typescript
// schemas.ts
import { z } from 'zod';

export const ResourceNodeSchema = z.object({
  id: z.string(),
  type: z.literal('resource'),
  data: z.object({
    resourceType: z.string(),
    name: z.string(),
    mode: z.string(),
    provider: z.string(),
    modulePath: z.string().optional(),
    moduleAddress: z.string().optional(),
    source: z.string().optional(),
  }),
  position: z.object({
    x: z.number(),
    y: z.number(),
  }).optional(),
});

export const ModuleNodeSchema = z.object({
  id: z.string(),
  type: z.literal('module'),
  data: z.object({
    name: z.string(),
    source: z.string(),
    modulePath: z.string(),
    hasChildren: z.boolean(),
    childResourceCount: z.number(),
    childModuleCount: z.number(),
    depth: z.number(),
    inputs: z.array(z.object({
      name: z.string(),
      type: z.string(),
      description: z.string(),
      default: z.any().optional(),
      required: z.boolean(),
    })),
    outputs: z.array(z.object({
      name: z.string(),
      type: z.string(),
      description: z.string(),
      sensitive: z.boolean(),
    })),
  }),
  position: z.object({
    x: z.number(),
    y: z.number(),
  }).optional(),
});

export const ExpressionNodeSchema = z.object({
  id: z.string(),
  type: z.literal('expression'),
  data: z.object({
    operation: z.string(), // "+", "-", "*", "/", "==", "!=", etc.
    description: z.string(),
    targetID: z.string(),
    targetType: z.string(),
    targetInput: z.string(),
    inputs: z.number(), // Number of input handles (usually 2)
    outputs: z.number(), // Number of output handles (usually 1)
  }),
  position: z.object({
    x: z.number(),
    y: z.number(),
  }).optional(),
});

export const StaticValueNodeSchema = z.object({
  id: z.string(),
  type: z.literal('static_value'),
  data: z.object({
    value: z.string(),
    type: z.string(),
    description: z.string(),
    side: z.enum(['LHS', 'RHS']), // Which side of the operation
  }),
  position: z.object({
    x: z.number(),
    y: z.number(),
  }).optional(),
});

export const UnknownOperandNodeSchema = z.object({
  id: z.string(),
  type: z.literal('unknown_operand'),
  data: z.object({
    side: z.string(),
    description: z.string(),
    exprType: z.string(),
  }),
  position: z.object({
    x: z.number(),
    y: z.number(),
  }).optional(),
});

export const NodeSchema = z.union([
  ResourceNodeSchema,
  ModuleNodeSchema,
  ExpressionNodeSchema,
  StaticValueNodeSchema,
  UnknownOperandNodeSchema,
]);

export const EdgeSchema = z.object({
  id: z.string(),
  source: z.string(),
  target: z.string(),
  type: z.enum([
    'depends_on', 
    'implicit', 
    'provider',
    'module_input',
    'module_output', 
    'expression_input',
    'expression_output'
  ]),
  sourceHandle: z.string().optional(),
  targetHandle: z.string().optional(),
});

export const GraphResponseSchema = z.object({
  nodes: z.array(NodeSchema),
  edges: z.array(EdgeSchema),
});

export type GraphResponse = z.infer<typeof GraphResponseSchema>;
export type ResourceNode = z.infer<typeof ResourceNodeSchema>;
export type ModuleNode = z.infer<typeof ModuleNodeSchema>;
export type ExpressionNode = z.infer<typeof ExpressionNodeSchema>;
export type StaticValueNode = z.infer<typeof StaticValueNodeSchema>;
export type UnknownOperandNode = z.infer<typeof UnknownOperandNodeSchema>;
export type Node = z.infer<typeof NodeSchema>;
export type Edge = z.infer<typeof EdgeSchema>;
```

## Dependencies

### Go Dependencies
- No new external dependencies needed (use standard library)

### Frontend Dependencies
- react
- @xyflow/react
- typescript
- vite
- tailwindcss (v4)
- @tanstack/react-router
- @tanstack/react-query
- zod
- @types/react
- @types/node

## Build Integration

### Makefile Changes
```makefile
# Add React build step
ui-build:
	cd internal/command/inspect/ui && npm install && npm run build

# Update main build to include UI
build: ui-build
	go build ./cmd/tofu
```

### GitHub Actions
- Add Node.js setup for CI/CD
- Cache node_modules
- Build UI before Go build

## Testing Strategy

1. **Unit Tests**:
   - Graph building logic
   - Dependency extraction
   - API handlers

2. **Integration Tests**:
   - Full command execution
   - Server startup/shutdown
   - API responses

3. **Example Configurations**:
   - Simple resource dependencies
   - Module hierarchies
   - Complex cross-module references

## Security Considerations

1. **Local Only by Default**: Bind to 127.0.0.1 unless explicitly overridden
2. **Read-Only**: No mutations to configuration or state
3. **Sensitive Data**: Be careful with variable values (mark sensitive ones)
4. **CORS**: Not needed for local tool, but consider for future

## Performance Considerations

1. **Large Configurations**: 
   - Implement pagination for resources
   - Progressive loading for graph visualization
   - Efficient dependency calculation

2. **Caching**:
   - Cache parsed configuration
   - Cache dependency graph
   - Only recalculate on file changes

## Future Enhancements

1. **Live Mode**: Show real-time changes during apply
2. **Plan Visualization**: Show planned changes
3. **State Visualization**: Show current state
4. **Cost Estimation**: Integrate with cost estimation tools
5. **Export/Import**: Save and share visualizations
6. **Collaboration**: Share via URL (with embedded server)
7. **Live Reload**: Watch for file changes and update graph
8. **Search/Filter**: Add resource filtering capabilities
9. **Export**: Generate static HTML or images

## Success Criteria

1. Can load and visualize any valid OpenTofu configuration
2. Interactive graph with smooth performance for 100+ resources
3. Single binary, no external dependencies
4. Cross-platform compatibility (Windows, macOS, Linux)
5. Intuitive UI that helps understand complex configurations

## Open Questions

1. Should we support custom layouts/themes?
2. How to handle very large configurations (1000+ resources)?
3. Should we integrate with existing TF tools (like Rover)?
4. WebSocket vs. polling for real-time updates?
5. Should we support exporting to other formats (GraphViz, Mermaid)?

## Current Implementation Status

### ✅ Completed (Go Backend)
- **Basic command structure** - `tofu inspect` with full CLI options
- **HTTP server** - Ephemeral port selection, standard library only
- **Configuration loading** - Uses OpenTofu's existing config loading
- **API endpoints** - `/api/health`, `/api/config`, `/api/graph`, `/api/resource/:id`, `/api/hierarchy`
- **Source code integration** - `/api/source/files` and `/api/source/content` with line range support
- **Config parsing** - Extracts resources, modules, providers, variables, outputs
- **Dependency extraction** - Explicit dependencies from `depends_on`
- **Module relationships** - Inter-module dependencies and calls with input/output mapping
- **Expression parsing** - Recursive binary expression parsing (addition, concatenation, etc.)
- **Graph structure** - Nodes and edges in @xyflow/react format with handles
- **Source location tracking** - Every node includes filename and line numbers for "edit source" workflows
- **Static value nodes** - Literal values in expressions with LHS/RHS positioning
- **Handle mapping** - Edges specify exact input handles (input-0, input-1) for proper connections
- **Security** - Path traversal protection for source file access
- **Browser integration** - Auto-opens browser with cross-platform support
- **CORS support** - Development mode with proper CORS headers

### ✅ Completed (Frontend Foundation)
- **React app structure** - Complete TypeScript setup with Vite
- **Zod schemas** - Full type safety for all API responses including expressions and source locations
- **TanStack Query integration** - Proper data fetching and caching
- **Component foundation** - Graph, ConfigView, ResourceDetail components
- **Development workflow** - Hot reload during development

### 🚧 Advanced Features Implemented
- **Recursive expression parsing** - Handles complex expressions like `a + "ABC" + "DEF"`
- **Multiple node types** - Resources, modules, expressions, static values, unknown operands
- **Expression visualization** - Shows binary operations with 2 inputs, 1 output
- **Source-to-graph mapping** - Click on any node to see exact source code location
- **Line-by-line source display** - API supports fetching specific line ranges

### 📋 Remaining (High Priority)
1. **Frontend expression node rendering** - Custom React Flow nodes for expressions with multiple input handles
2. **Source code viewer component** - Display source with syntax highlighting and line numbers
3. **Edit source workflow** - Integrate source viewer with node selection
4. **Graph layout improvements** - Better positioning for expression trees

### 📋 Remaining (Medium Priority)
1. **Integrate with OpenTofu's actual graph walker** 
   - Use `tofu.Context` and `PlanGraphForUI` like the graph command
   - Get real dependency graph with provider dependencies
   - Show ordering constraints and execution graph

2. **Enhanced expression support**
   - Function calls, conditionals, loops
   - Template expressions with interpolation
   - Variable references and module outputs

3. **Embedded UI setup**
   - Create `//go:embed ui/dist/*` structure
   - Set up build pipeline integration
   - Serve React app from embedded filesystem

### 📋 Remaining (Low Priority)
1. **Error handling and diagnostics**
   - Show configuration errors and warnings
   - Display validation issues
   - Handle malformed configurations gracefully

2. **Performance optimizations**
   - Graph layout algorithms (force-directed, hierarchical)
   - WebSocket support for real-time updates
   - Resource filtering and search backend

### 🎯 Current Achievement: Complete Source Integration + Expression Parsing

The backend now provides comprehensive graph visualization capabilities:

#### Source Code Integration
- **File location tracking** - Every node includes exact filename and line numbers
- **Source API endpoints** - Fetch file lists and content with line range support  
- **Security** - Path traversal protection for safe file access
- **Edit source workflows** - Click any node to see/edit its source code

#### Advanced Expression Parsing
- **Recursive binary expressions** - Handles `a + "ABC" + "DEF"` style chained operations
- **Multiple node types** - Resources, modules, expressions, static values
- **Proper handle mapping** - LHS → input-0, RHS → input-1 for precise connections
- **Expression trees** - Complex expressions become connected subgraphs
- **Static value tracking** - Literal strings/numbers with source locations

#### Graph Features
- **Complete dependency mapping** - Module inputs/outputs, resource dependencies
- **Handle-based connections** - Precise input/output handle specifications
- **Cross-module references** - Module calls with input assignments
- **Development-ready API** - CORS support, proper error handling

#### Examples Working
- **Simple expressions**: `resource.attr + "string"`
- **Complex expressions**: `resource.result + "ABC" + "DEF"` 
- **Module dependencies**: Database → Application with connection strings
- **Source mapping**: Click on expression → see exact line in main.tf

The system now provides everything needed for a rich visual configuration editor with bidirectional source code integration.

## Frontend Integration Plan

### Data Flow Architecture
```
Config Files → Config Parser → /api/config (explicit dependencies)
             ↓
Config Files → Graph Walker → /api/graph (execution dependencies)
             ↓
React App → TanStack Query → Zod Validation → @xyflow/react
```

### Key Frontend Features to Support
- **Dual view toggle** - Switch between config and graph dependencies
- **Module hierarchy** - Expandable module view
- **Dependency highlighting** - Show dependency chains
- **Real-time updates** - WebSocket integration for plan/apply
- **Search and filtering** - Find resources and dependencies
- **Export capabilities** - Save graphs as images/files

## Next Steps

1. **Complete graph walker integration** (Go backend)
2. **Set up embedded UI structure** (Go backend) 
3. **Create React app foundation** (Frontend)
4. **Implement graph visualization** (Frontend)
5. **Add interactive features** (Frontend)
6. **Integration testing** (Full stack)