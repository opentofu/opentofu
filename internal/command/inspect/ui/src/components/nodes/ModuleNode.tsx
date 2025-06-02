import { Handle, Position } from "@xyflow/react";
import EditButton from "./EditButton";
import InlineEditor from "./InlineEditor";

interface ModuleNodeProps {
	data: any;
	id: string;
}

export default function ModuleNode({ data, id }: ModuleNodeProps) {
	const inputs = data.inputs || [];
	const outputs = data.outputs || [];
	const { editingNodes, onEditNode, onSaveEdit, onCancelEdit } = data._graphContext || {};
	const isEditing = editingNodes?.has(id);
	const hasSource = !!data.source;

	return (
		<div className="node-module rounded-lg border-3 border-purple-500 bg-purple-50 shadow-lg relative min-w-[300px]">
			{/* Edit button */}
			<EditButton nodeId={id} hasSource={hasSource} onEdit={onEditNode} />
			{/* Module header */}
			<div className="font-bold text-purple-800 py-2 text-center border-b border-purple-200 bg-purple-100">📦 {data.name}</div>

			{/* Main content area with inputs and outputs */}
			<div className="flex">
				{/* Left side - Inputs */}
				<div className="flex-1 p-3">
					{inputs.length > 0 && (
						<>
							<div className="text-xs font-semibold text-purple-700 mb-2">Inputs</div>
							{inputs.map((input: any, index: number) => (
								<div key={input.name} className="text-xs text-purple-600 flex items-center mb-2 relative">
									{/* Handle positioned as the dot icon */}
									<Handle
										type="target"
										position={Position.Left}
										id={`input-${input.name}`}
										style={{
											position: "absolute",
											left: "-15px",
											top: "50%",
											transform: "translateY(-50%)",
											width: "10px",
											height: "10px",
											background: input.required ? "#ef4444" : "#6b7280",
											border: "2px solid white",
											borderRadius: "50%",
										}}
									/>
									<div className="ml-1">
										<div className="font-medium">{input.name}</div>
										<div className="text-gray-500 text-xs">
											{input.type}
											{input.required && <span className="text-red-500 ml-1">*</span>}
										</div>
									</div>
								</div>
							))}
						</>
					)}
				</div>

				{/* Right side - Outputs */}
				<div className="flex-1 p-3">
					{outputs.length > 0 && (
						<>
							<div className="text-xs font-semibold text-purple-700 mb-2 text-right">Outputs</div>
							{outputs.map((output: any, index: number) => (
								<div key={output.name} className="text-xs text-purple-600 flex items-center justify-end mb-2 relative">
									<div className="mr-1 text-right">
										<div className="font-medium">{output.name}</div>
										<div className="text-gray-500 text-xs">
											{output.type}
											{output.sensitive && <span className="text-orange-500 ml-1">🔒</span>}
										</div>
									</div>
									{/* Handle positioned as the dot icon */}
									<Handle
										type="source"
										position={Position.Right}
										id={`output-${output.name}`}
										style={{
											position: "absolute",
											right: "-15px",
											top: "50%",
											transform: "translateY(-50%)",
											width: "10px",
											height: "10px",
											background: output.sensitive ? "#f97316" : "#10b981",
											border: "2px solid white",
											borderRadius: "50%",
										}}
									/>
								</div>
							))}
						</>
					)}
				</div>
			</div>

			{/* Module info footer */}
			{data.source && <div className="text-xs opacity-70 p-2 text-center border-t border-purple-200 bg-purple-100">{data.source.filename}</div>}

			{/* Editor overlay */}
			{isEditing && hasSource && data.source && (
				<div className="absolute inset-0 z-50">
					<InlineEditor nodeId={id} sourceLocation={data.source} onSave={onSaveEdit} onCancel={onCancelEdit} />
				</div>
			)}
		</div>
	);
}
