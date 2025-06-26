# Graph Visualization Command for OpenTofu

Issue: (I thought there was an open issue for that but I couldn't find)

## Summary

This RFC proposes a new command for the `tofu` CLI that generates a graph-like visualization of resource relationships and dependencies in an OpenTofu project. The output is a self-contained HTML file, viewable locally or as a pipeline artifact, providing a read-only, interactive Web UI built with React Flow.

## Motivation

- Users often need to understand complex resource relationships and dependencies in their infrastructure code.
- A portable, cacheable HTML artifact or a zip file can be used in CI pipelines for auditing, reviews, and historical snapshots.
- Graph UI visualization can help developers to debug and understand the infrastructure state.

## Background

OpenTofu currently lacks a built-in web-based visualization for resource graphs. The `tofu graph` command is not useful for an easy visualization, while it can be useful for automation or more detailed debugging. While doing the OpenTofu hackathon, @yantrio proposed and created a new way to show the graph in a web UI, so we're expanding further in this RFC in order to gather community's feedback.

## Proposed Solution

Introduce a new command:

```sh
tofu graph webui -o graph.html
```

This command will:
- Analyze the current state or plan to extract resource nodes and their dependencies.
- Generate a static HTML file embedding a React Flow-based UI.
- Provide a read-only, interactive graph (pan, zoom, inspect nodes/edges).

### User Documentation

#### Usage

```sh
tofu graph webui -o graph.html
```

- `-o <file>`: Output HTML file (default: `graph.html`, or a zip file)
- The command can be run without depending on `tofu plan` or `tofu apply` since it will use existing configuration.
- The generated HTML can be opened in any browser or uploaded as a CI artifact.

#### Example Workflow

1. Run `tofu graph webui -o graph.html`.
2. Open `graph.html` locally, or grab it from a pipeline artifact in order to understand how the graph looked at some particular point in time.

#### Features

- Interactive, read-only graph (no editing or mutation)
- Node and edge details on click/hover
- Search/filter nodes

### Technical Approach

- Parse the dependency graph from the current state or plan, by using the `*configs.Config` structure.
- Serialize nodes and edges as JSON, embedded in the HTML.
- Use React Flow (https://reactflow.dev/) for rendering the graph UI.
- Output a single zip file with all the assets or an HTML file (open question).
- Ensure the HTML is portable and can be cached as a pipeline artifact.
- The command will not expose any sensitive data.
- The output is strictly read-only; no state mutation or API calls.

### Open Questions

- Should the graph be generated from the running plan, from a plan file, or both?
- How to handle very large graphs (performance, usability)?
- What metadata should be shown for nodes/edges?
- Should the UI support dark mode or theming?

## Potential Alternatives

- There is an existing project called Terramaid for generating Mermaid graphs.
