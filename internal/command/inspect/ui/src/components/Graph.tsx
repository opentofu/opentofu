import { useCallback, useEffect, useState } from "react";
import { ReactFlow, Background, Controls, useNodesState, useEdgesState, addEdge, type Connection, type Edge, type Node, type NodeTypes, Position, Panel, useReactFlow } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import ELK, { ElkNode, ElkExtendedEdge } from "elkjs/lib/elk.bundled.js";
import type { GraphResponse, GraphNode } from "../api/schemas";
import { ResourceNode, ModuleNode, ExpressionNode, StaticValueNode, UnknownOperandNode, OutputRootNode } from "./nodes";

// Node types mapping
const nodeTypes: NodeTypes = {
	resource: ResourceNode,
	module: ModuleNode,
	expression: ExpressionNode,
	static_value: StaticValueNode,
	output_root: OutputRootNode,
	unknown_operand: UnknownOperandNode,
};

interface GraphProps {
	data: GraphResponse | null;
	isLoading: boolean;
	error: Error | null;
	scope?: string; // undefined for root module, or module name to scope down
	onNodeSelect?: (node: GraphNode) => void;
}

const elk = new ELK();

const getLayoutedElements = (nodes: Node[], edges: Edge[]) => {
	// Function to calculate node dimensions based on content
	const getNodeDimensions = (node: Node) => {
		const data = node.data;
		const nodeType = node.type || "resource";
		
		// Base dimensions with padding
		const padding = 20;
		const minWidth = 120;
		const minHeight = 60;
		
		switch (nodeType) {
			case "module": {
				const inputs = data.inputs || [];
				const outputs = data.outputs || [];
				const maxItems = Math.max(inputs.length, outputs.length);
				
				// Calculate width based on longest input/output name, not sum
				const nameWidth = (data.name?.length || 10) * 8; // ~8px per char
				const maxInputWidth = inputs.reduce((max: number, input: any) => 
					Math.max(max, (input.name?.length || 0) * 6), 0);
				const maxOutputWidth = outputs.reduce((max: number, output: any) => 
					Math.max(max, (output.name?.length || 0) * 6), 0);
				
				// Width should fit the name and the widest input/output side by side
				const contentWidth = Math.max(nameWidth, maxInputWidth + maxOutputWidth + 40);
				const width = Math.max(300, contentWidth + 60); // padding
				
				// Height needs more space for each input/output item
				const headerHeight = 70; // Module name header + input/output section headers
				const footerHeight = data.source ? 30 : 0; // Source info footer
				const itemsHeight = maxItems * 30; // More space per item
				const height = Math.max(180, headerHeight + itemsHeight + footerHeight + 40);
				
				return { width, height };
			}
			
			case "expression": {
				const operation = data.operation || "";
				const description = data.description || "";
				const targetInput = data.targetInput || "";
				
				const contentWidth = Math.max(
					operation.length * 12, // Larger font for operation
					description.length * 6,
					targetInput.length * 6
				);
				
				const inputCount = data.inputs || 0;
				const outputCount = data.outputs || 0;
				const maxHandles = Math.max(inputCount, outputCount);
				
				return {
					width: Math.max(120, contentWidth + 40),
					height: Math.max(80, 60 + (maxHandles * 5)) // Account for handle spacing
				};
			}
			
			case "static_value": {
				const value = data.value || "";
				const type = data.type || "";
				const side = data.side || "";
				
				const contentWidth = Math.max(
					value.toString().length * 8,
					(type + side).length * 6
				);
				
				return {
					width: Math.max(100, Math.min(140, contentWidth + 30)),
					height: 60
				};
			}
			
			case "unknown_operand": {
				const side = data.side || "";
				const exprType = data.exprType || "";
				const description = data.description || "";
				
				const contentWidth = Math.max(
					side.length * 8,
					exprType.length * 6,
					description.length * 6
				);
				
				return {
					width: Math.max(100, Math.min(140, contentWidth + 30)),
					height: 80
				};
			}
			
			default: { // resource, output, etc.
				const name = data.name || "";
				const resourceType = data.resourceType || "";
				const module = data.module || "";
				
				const contentWidth = Math.max(
					name.length * 8,
					resourceType.length * 6,
					module.length * 6
				);
				
				return {
					width: Math.max(minWidth, contentWidth + padding),
					height: Math.max(minHeight, module ? 100 : 80)
				};
			}
		}
	};

	// Sort edges by target handle to maintain input ordering
	const sortedEdges = [...edges].sort((a, b) => {
		// Sort by target node first, then by target handle
		if (a.target !== b.target) {
			return a.target.localeCompare(b.target);
		}
		
		// Extract input number from target handle (e.g., "input-0", "input-1")
		const getInputNumber = (handle: string | undefined) => {
			if (!handle) return 999; // Put edges without handles at the end
			const match = handle.match(/input-(\d+)$/);
			return match ? parseInt(match[1], 10) : 999;
		};
		
		const aInputNum = getInputNumber(a.targetHandle);
		const bInputNum = getInputNumber(b.targetHandle);
		
		return aInputNum - bInputNum;
	});

	// Convert ReactFlow edges to ELK format with priorities based on input order
	const elkEdges: ElkExtendedEdge[] = sortedEdges.map((edge, index) => ({
		id: edge.id,
		sources: [edge.source],
		targets: [edge.target],
		// Add priority to maintain edge ordering in layout
		layoutOptions: {
			"elk.priority": (1000 - index).toString(), // Higher numbers = higher priority
		},
	}));

	console.log("Input nodes to ELK:", nodes.map(n => ({ id: n.id, type: n.type })));
	console.log("Input edges to ELK:", elkEdges);

	const graph: ElkNode = {
		id: "root",
		layoutOptions: {
			"elk.spacing.nodeNode": "50",
			"elk.layered.spacing.nodeNodeBetweenLayers": "80",
			"elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
			"elk.layered.nodePlacement.strategy": "SIMPLE",
			"elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
			"elk.layered.considerModelOrder.crossingCounterNodeInfluence": "0.5",
			"elk.layered.considerModelOrder.crossingCounterPortInfluence": "0.5",
		},
		children: nodes.map((node) => {
			const dimensions = getNodeDimensions(node);
			return {
				...node,
				...dimensions,
			};
		}),
		edges: elkEdges,
	};

	console.log("Full graph passed to ELK:", graph);

	return elk
		.layout(graph)
		.then((layoutedGraph) => {
			console.log("ELK layout result:", layoutedGraph);
			layoutedGraph.children?.forEach((node) => {
				console.log(`Node ${node.id}: x=${node.x}, y=${node.y}, type=${(node as any).type}`);
			});
			return {
				nodes:
					layoutedGraph.children?.map((node) => ({
						...node,
						position: { x: node.x || 0, y: node.y || 0 },
					})) || [],
				edges: layoutedGraph.edges || edges,
			};
		})
		.catch(() => {
			// Fallback to grid layout on error
			const layoutedNodes = nodes.map((node, index) => {
				const gridCols = 6;
				return {
					...node,
					position: {
						x: (index % gridCols) * 220 + 50,
						y: Math.floor(index / gridCols) * 140 + 50,
					},
				};
			});

			return { nodes: layoutedNodes, edges };
		});
};

export default function Graph({ data, isLoading, error, scope, onNodeSelect }: GraphProps) {
	// State for tracking which nodes are in edit mode
	const [editingNodes, setEditingNodes] = useState<Set<string>>(new Set());

	// Edit handlers - memoized to prevent constant re-renders
	const handleEditNode = useCallback((nodeId: string) => {
		setEditingNodes((prev) => new Set(prev).add(nodeId));
	}, []);

	const handleSaveEdit = useCallback((nodeId: string, content: string) => {
		// TODO: Implement saving logic
		console.log("Save edit for node:", nodeId, "Content:", content);
		setEditingNodes((prev) => {
			const newSet = new Set(prev);
			newSet.delete(nodeId);
			return newSet;
		});
	}, []);

	const handleCancelEdit = useCallback((nodeId: string) => {
		setEditingNodes((prev) => {
			const newSet = new Set(prev);
			newSet.delete(nodeId);
			return newSet;
		});
	}, []);

	if (error) {
		return (
			<div className="h-full flex items-center justify-center">
				<div className="error-message max-w-md text-center">
					<h3 className="text-lg font-semibold mb-2">Graph Error</h3>
					<p>Failed to load dependency graph.</p>
					<p className="text-sm mt-2">{error.message}</p>
				</div>
			</div>
		);
	}

	if (isLoading) {
		return (
			<div className="h-full flex items-center justify-center">
				<div className="text-center">
					<div className="loading-spinner mx-auto mb-4" />
					<p className="text-gray-600">Loading dependency graph...</p>
				</div>
			</div>
		);
	}

	if (!data?.nodes?.length) {
		return (
			<div className="h-full flex items-center justify-center">
				<div className="text-center text-gray-500">
					<div className="text-4xl mb-4">📊</div>
					<h3 className="text-lg font-semibold mb-2">No Graph Data</h3>
					<p>No resources or dependencies found in the configuration.</p>
				</div>
			</div>
		);
	}

	// Filter nodes based on scope
	const filteredNodes = data.nodes.filter((node) => {
		if (!scope) {
			// Root scope: show only top-level modules and root-level resources/outputs/expressions/static_values
			if (node.type === "module") {
				// Only show modules that are direct children of root (depth 1)
				const moduleData = node.data as any;
				return moduleData.depth === 1;
			}
			// Include root-level resources, outputs, expressions, static values, and unknown operands
			if (node.type === "resource" || node.type === "output_root" || node.type === "expression" || node.type === "static_value" || node.type === "unknown_operand") {
				return !node.parentId || node.parentId === "module.root";
			}
			return false;
		}
		// Scoped to a specific module: show its direct children
		if (node.type === "module") {
			// Show modules that are direct children of the scope module
			return node.parentId === scope;
		}
		// Show resources, outputs, expressions, static values, and unknown operands that belong to the scope module
		if (node.type === "resource" || node.type === "output_root" || node.type === "expression" || node.type === "static_value" || node.type === "unknown_operand") {
			return node.parentId === scope;
		}
		return false;
	});

	// Simple node processing - just add position and handle properties
	const processedNodes: Node[] = filteredNodes.map(
		(node, index) =>
			({
				...node,
				// Set a temporary grid position to avoid ReactFlow drag issues
				position: {
					x: (index % 6) * 220 + 50,
					y: Math.floor(index / 6) * 140 + 50,
				},
				type: node.type || "resource",
				sourcePosition: Position.Right,
				targetPosition: Position.Left,
				// Unset parent if it's "module.root" since that's not an actual node
				parentId: node.parentId === "module.root" ? null : node.parentId,
				data: {
					...node.data,
					source: node.source, // Add source location to data
					_graphContext: {
						editingNodes,
						onEditNode: handleEditNode,
						onSaveEdit: handleSaveEdit,
						onCancelEdit: handleCancelEdit,
					},
				},
			}) as Node,
	);

	// Filter edges to only include those between filtered nodes
	const nodeIds = new Set(filteredNodes.map(node => node.id));
	const filteredEdges = (data.edges || []).filter(edge => 
		nodeIds.has(edge.source) && nodeIds.has(edge.target)
	);

	const processedEdges: Edge[] = filteredEdges.map((edge) => ({
		...edge,
		type: "default",
		style: { stroke: "#8b5cf6", strokeWidth: 2 },
	}));

	const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
	const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);

	useEffect(() => {
		async function layoutNodes() {
			// Add the graph context to processed nodes
			const nodesWithContext = processedNodes.map((node) => ({
				...node,
				data: {
					...node.data,
					_graphContext: {
						editingNodes,
						onEditNode: handleEditNode,
						onSaveEdit: handleSaveEdit,
						onCancelEdit: handleCancelEdit,
					},
				},
			}));

			const { nodes: layoutedNodes, edges: layoutedEdges } = await getLayoutedElements(nodesWithContext, processedEdges);
			setNodes(layoutedNodes as Node[]);
			setEdges(processedEdges); // Use our filtered edges, not ELK's returned edges
		}

		if (processedNodes.length > 0) {
			layoutNodes();
		}
	}, [data?.nodes, data?.edges, editingNodes, handleEditNode, handleSaveEdit, handleCancelEdit]);

	const onConnect = useCallback((params: Edge | Connection) => setEdges((eds) => addEdge(params, eds)), [setEdges]);

	const onNodeClick = useCallback(
		(event: React.MouseEvent, node: Node) => {
			if (onNodeSelect) {
				// Find the original GraphNode from the data
				const graphNode = filteredNodes.find((n) => n.id === node.id);
				if (graphNode) {
					onNodeSelect(graphNode);
				}
			}
		},
		[onNodeSelect, filteredNodes],
	);

	return (
		<div className="h-full w-full">
			<ReactFlow
				nodes={nodes}
				edges={edges}
				onNodesChange={onNodesChange}
				onEdgesChange={onEdgesChange}
				onConnect={onConnect}
				onNodeClick={onNodeClick}
				nodeTypes={nodeTypes}
				minZoom={0.1}
				maxZoom={2}
				defaultViewport={{ x: 200, y: 200, zoom: 1.2 }}
			>
				<Background />
				<Controls />
				<Panel position="top-left">
					<div className="bg-white p-3 rounded-lg shadow-lg text-sm">
						<div className="font-semibold mb-2">Node Types</div>
						<div className="text-xs space-y-1">
							<div>🟢 Managed Resources</div>
							<div>🔵 Data Sources</div>
							<div>🟣 Modules</div>
							<div>🟠 Expressions</div>
							<div>🔵 Static Values (String)</div>
							<div>🟢 Static Values (Number)</div>
							<div>🟠 Static Values (Boolean)</div>
							<div>❓ Unknown Operands</div>
							<div>🔴 Outputs</div>
						</div>
					</div>
				</Panel>
			</ReactFlow>
		</div>
	);
}
