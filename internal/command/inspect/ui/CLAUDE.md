# OpenTofu Inspect Tool - UI Development Notes

## Project Overview
This is a React-based UI for the OpenTofu inspect tool that provides graph visualization of Terraform/OpenTofu configurations using React Flow. The tool allows users to visualize resource dependencies, module relationships, and configuration structure.

## Architecture

### Tech Stack
- **React** with TypeScript
- **React Flow** for graph visualization 
- **ELK (Eclipse Layout Kernel)** for automatic graph layout
- **TanStack Query** for API data fetching
- **Zod** for type-safe API validation
- **Tailwind CSS** for styling
- **Vite** for build tooling

### Key Components

#### Graph Component (`/src/components/Graph.tsx`)
Main graph visualization component that:
- Handles layout using ELK algorithm
- Manages node editing state with inline editors
- Filters nodes based on scope (root vs module-specific views)
- Implements dynamic dimension calculation for nodes
- Sorts edges by input order for proper vertical alignment

#### Node Components (`/src/components/nodes/`)
Organized into separate files for maintainability:
- **ResourceNode.tsx**: Displays resources (managed/data sources)
- **ModuleNode.tsx**: Complex modules with inputs/outputs and handles
- **ExpressionNode.tsx**: Mathematical/logical operations
- **StaticValueNode.tsx**: Literal values (strings, numbers, booleans)
- **UnknownOperandNode.tsx**: Unresolved operands
- **EditButton.tsx**: Shared edit functionality
- **InlineEditor.tsx**: Overlay editor for source code editing

#### API Layer (`/src/api/`)
- **client.ts**: HTTP client setup
- **queries.ts**: TanStack Query hooks for data fetching
- **schemas.ts**: Zod schemas for type validation

## Key Features

### Dynamic Node Sizing
Nodes are sized dynamically based on their content:
- **Modules**: Based on name length, input/output counts, with proper spacing for headers
- **Expressions**: Based on operation text and handle counts
- **Static Values**: Based on value and type text with size limits
- **Resources**: Based on name, resource type, and module text

### Edge Ordering and Layout
- Edges are sorted by target handle (input-0, input-1, etc.) before layout
- ELK priorities ensure proper vertical alignment of input sources
- Layout options optimize for edge ordering while minimizing crossings

### Inline Editing
- Nodes with source locations can be edited inline
- Editor overlay appears on top of nodes
- Fetches actual source code content for editing
- Save/cancel functionality (save logic TODO)

### Scoped Views
- Root view: Shows top-level modules and root resources
- Module view: Shows contents of specific modules
- Proper edge filtering ensures consistency

## Development Commands

### Setup
```bash
npm install  # or pnpm install
```

### Development
```bash
npm run dev  # Start development server
```

### Type Checking & Linting
```bash
npm run typecheck  # TypeScript compilation check
npm run lint       # ESLint + Biome linting
```

### Build
```bash
npm run build     # Production build
npm run preview   # Preview built app
```

## Important Implementation Details

### Graph Layout Algorithm
Uses ELK with specific configuration:
- `elk.layered.considerModelOrder.strategy`: "NODES_AND_EDGES" - Respects input ordering
- `elk.layered.crossingMinimization.strategy`: "LAYER_SWEEP" - Better edge crossing minimization
- `elk.layered.nodePlacement.strategy`: "SIMPLE" - Predictable node placement
- Custom edge priorities based on input order

### State Management
- Graph editing state managed with `useState<Set<string>>` for editing nodes
- Handlers memoized with `useCallback` to prevent re-rendering
- Context passed through node data to avoid prop drilling

### Performance Optimizations
- Memoized layout calculations
- Proper useCallback usage for event handlers
- Efficient edge and node filtering
- Dynamic dimension calculation only when needed

### Node Handle System
- Modules: Individual handles for each input/output (`input-${name}`, `output-${name}`)
- Resources: Simple source/target handles
- Expressions: Multiple input handles based on operation
- Handle positioning with absolute positioning and proper styling

## Data Flow

1. **API Data**: Fetched via TanStack Query hooks
2. **Node Processing**: Raw graph data converted to ReactFlow format
3. **Filtering**: Nodes/edges filtered based on current scope
4. **Layout**: ELK processes nodes with dynamic dimensions and sorted edges
5. **Rendering**: ReactFlow renders with custom node components
6. **Interaction**: Edit states managed and passed through context

## Known Issues & TODOs

### Completed Fixes
- ✅ Fixed constant re-rendering by memoizing handlers
- ✅ Fixed missing handles during editing with overlay approach
- ✅ Fixed TypeScript compilation errors
- ✅ Fixed node positioning by proper edge filtering
- ✅ Fixed missing edge rendering
- ✅ Implemented dynamic dimension calculation
- ✅ Added vertical sorting by input order

### Remaining TODOs
- [ ] Implement actual save functionality in inline editor
- [ ] Add tests for components
- [ ] Add error boundaries for better error handling
- [ ] Optimize performance for large graphs
- [ ] Add graph export functionality
- [ ] Add search/filter capabilities

## File Structure
```
src/
├── components/
│   ├── Graph.tsx              # Main graph component
│   ├── ConfigView.tsx         # Configuration view
│   ├── ResourceDetail.tsx     # Resource detail panel
│   ├── ViewToggle.tsx         # View switching component
│   └── nodes/                 # Node components
│       ├── ResourceNode.tsx
│       ├── ModuleNode.tsx
│       ├── ExpressionNode.tsx
│       ├── StaticValueNode.tsx
│       ├── UnknownOperandNode.tsx
│       ├── EditButton.tsx
│       ├── InlineEditor.tsx
│       └── index.ts
├── api/
│   ├── client.ts              # HTTP client
│   ├── queries.ts             # TanStack Query hooks
│   └── schemas.ts             # Zod type schemas
├── hooks/                     # Custom React hooks
├── routes/                    # Routing components
└── styles/
    └── globals.css            # Global styles
```

## Graph Data Schema

### Nodes
- **id**: Unique identifier
- **type**: 'resource' | 'module' | 'expression' | 'static_value' | 'unknown_operand'
- **data**: Type-specific data (name, inputs, outputs, etc.)
- **source**: Optional source location for editing
- **parentId**: Module hierarchy relationship

### Edges
- **id**: Unique identifier
- **source/target**: Node IDs
- **type**: Edge relationship type
- **sourceHandle/targetHandle**: Specific input/output handles
- Used for sorting and layout priorities

## Styling Conventions
- Tailwind CSS for all styling
- Color coding by node type:
  - 🟢 Managed Resources (green)
  - 🔵 Data Sources (blue) 
  - 🟣 Modules (purple)
  - 🟠 Expressions (orange)
  - Static Values (blue/green/orange by type)
  - ❓ Unknown Operands (gray)
- Consistent spacing and sizing
- Responsive design principles