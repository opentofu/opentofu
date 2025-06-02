import { useState } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./api/client";
import { useHealth, useConfig, useGraph } from "./api/queries";
import Graph from "./components/Graph";
import ConfigView from "./components/ConfigView";
import ResourceDetail from "./components/ResourceDetail";
import SourceViewer, { SourceInfo } from "./components/SourceViewer";
import ViewToggle, { ViewDescription } from "./components/ViewToggle";
import "./styles/globals.css";
import { ReactFlowProvider } from "@xyflow/react";
import type { GraphNode } from "./api/schemas";

function Dashboard() {
	const [currentView, setCurrentView] = useState<"config" | "graph">("graph");
	const [selectedResourceId, setSelectedResourceId] = useState<string | null>(null);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

	const { data: health, isLoading: healthLoading, error: healthError } = useHealth();
	const { data: config, isLoading: configLoading, error: configError } = useConfig();
	const { data: graph, isLoading: graphLoading, error: graphError } = useGraph();

	// Show connection error if any of the critical APIs fail
	if (healthError || configError) {
		return (
			<div className="h-full flex items-center justify-center">
				<div className="error-message max-w-md">
					<h2 className="text-lg font-semibold mb-2">Connection Error</h2>
					<p>Failed to connect to the OpenTofu inspect server.</p>
					<p className="text-sm mt-2">Error: {(healthError || configError)?.message}</p>
				</div>
			</div>
		);
	}

	if (healthLoading || configLoading) {
		return (
			<div className="h-full flex items-center justify-center">
				<div className="text-center">
					<div className="loading-spinner mx-auto mb-4" />
					<p className="text-gray-600">Loading configuration...</p>
				</div>
			</div>
		);
	}

	const resourceCount = config?.resources?.length || 0;
	const moduleCount = config?.modules?.length || 0;
	const nodeCount = graph?.nodes?.length || 0;
	const edgeCount = graph?.edges?.length || 0;

	const handleResourceSelect = (resourceId: string) => {
		setSelectedResourceId(resourceId);
	};

	const handleResourceClose = () => {
		setSelectedResourceId(null);
	};

	const handleNodeSelect = (node: GraphNode) => {
		setSelectedNode(node);
		// Clear resource detail when selecting a different type of node
		if (node.type !== 'resource') {
			setSelectedResourceId(null);
		}
	};

	const handleSidebarClose = () => {
		setSelectedNode(null);
		setSelectedResourceId(null);
	};

	return (
		<div className="h-full flex flex-col bg-gray-50">
			{/* Header */}
			<div className="header flex items-center justify-between">
				<div>
					<h1 className="text-xl font-bold text-gray-900">OpenTofu Inspect</h1>
					<p className="text-sm text-gray-600 mt-1">📁 {health?.config_path}</p>
				</div>

				<div className="flex items-center gap-4">
					<ViewToggle view={currentView} onViewChange={setCurrentView} configCount={resourceCount} graphCount={nodeCount} />
				</div>
			</div>

			{/* Stats bar */}
			<div className="bg-white border-b border-gray-200 px-4 py-3">
				<div className="flex items-center justify-between">
					<div className="flex gap-6 text-sm">
						<div className="flex items-center gap-1">
							<span className="font-medium text-gray-900">{resourceCount}</span>
							<span className="text-gray-600">resources</span>
						</div>
						<div className="flex items-center gap-1">
							<span className="font-medium text-gray-900">{moduleCount}</span>
							<span className="text-gray-600">modules</span>
						</div>
						<div className="flex items-center gap-1">
							<span className="font-medium text-gray-900">{nodeCount}</span>
							<span className="text-gray-600">graph nodes</span>
						</div>
						<div className="flex items-center gap-1">
							<span className="font-medium text-gray-900">{edgeCount || 0}</span>
							<span className="text-gray-600">dependencies</span>
						</div>
					</div>

					<ViewDescription view={currentView} />
				</div>
			</div>

			{/* Main content */}
			<div className="flex-1 flex overflow-hidden">
				{/* Main view area */}
				<div className="flex-1 flex flex-col">
					<div className="flex-1 p-4" style={{ height: "calc(100vh - 200px)" }}>
						<div className="graph-container" style={{ height: "100%", width: "100%" }}>
							{currentView === "graph" ? (
								<ReactFlowProvider>
									<Graph data={graph || null} isLoading={graphLoading} error={graphError} scope={undefined} onNodeSelect={handleNodeSelect} />
								</ReactFlowProvider>
							) : (
								<ConfigView data={config || null} isLoading={configLoading} error={configError} onResourceSelect={handleResourceSelect} />
							)}
						</div>
					</div>
				</div>

				{/* Sidebar */}
				<div className="w-80 sidebar">
					{selectedNode ? (
						<div className="p-4 h-full overflow-y-auto">
							<div className="flex items-center justify-between mb-4">
								<h2 className="text-lg font-semibold">Node Details</h2>
								<button
									type="button"
									onClick={handleSidebarClose}
									className="text-gray-400 hover:text-gray-600 text-xl"
								>
									✕
								</button>
							</div>
							
							{/* Node info */}
							<div className="bg-white border border-gray-200 rounded-lg p-4 mb-4">
								<div className="flex items-center gap-2 mb-2">
									<span className="text-xs font-medium text-gray-500 uppercase">{selectedNode.type}</span>
								</div>
								<h3 className="font-semibold text-gray-900 mb-1">{(selectedNode.data as any)?.name || selectedNode.id}</h3>
								{(selectedNode.data as any)?.description && (
									<p className="text-sm text-gray-600 mb-2">{(selectedNode.data as any).description}</p>
								)}
								
								{/* Source location info */}
								{selectedNode.source && (
									<div className="mt-3">
										<SourceInfo source={selectedNode.source} />
									</div>
								)}
							</div>
							
							{/* Source code viewer */}
							{selectedNode.source && (
								<div className="mb-4">
									<h4 className="text-sm font-medium text-gray-700 mb-2">Source Code</h4>
									<SourceViewer
										filename={selectedNode.source.filename}
										highlightStart={selectedNode.source.startLine}
										highlightEnd={selectedNode.source.endLine}
									/>
								</div>
							)}
						</div>
					) : selectedResourceId ? (
						<ResourceDetail resourceId={selectedResourceId} onClose={handleResourceClose} />
					) : (
						<div className="h-full flex items-center justify-center text-gray-500">
							<div className="text-center">
								<div className="text-4xl mb-2">👆</div>
								<p className="text-sm">Click on a node to see details</p>
							</div>
						</div>
					)}
				</div>
			</div>
		</div>
	);
}

function App() {
	return (
		<QueryClientProvider client={queryClient}>
			<Dashboard />
		</QueryClientProvider>
	);
}

export default App;
