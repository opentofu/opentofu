import { Handle, Position } from "@xyflow/react";
import EditButton from "./EditButton";
import InlineEditor from "./InlineEditor";

interface ExpressionNodeProps {
	data: any;
	id: string;
}

export default function ExpressionNode({ data, id }: ExpressionNodeProps) {
	const inputCount = data.inputs || 0;
	const outputCount = data.outputs || 0;
	const { editingNodes, onEditNode, onSaveEdit, onCancelEdit } = data._graphContext || {};
	const isEditing = editingNodes?.has(id);
	const hasSource = !!data.source;

	if (isEditing && hasSource && data.source) {
		return <InlineEditor nodeId={id} sourceLocation={data.source} onSave={onSaveEdit} onCancel={onCancelEdit} />;
	}

	return (
		<div className="node-expression rounded-lg border-2 border-orange-500 bg-orange-50 shadow-md relative min-w-[120px] min-h-[80px]">
			{/* Edit button */}
			<EditButton nodeId={id} hasSource={hasSource} onEdit={onEditNode} />
			{/* Input handles */}
			{Array.from({ length: inputCount }).map((_, i) => (
				<Handle
					key={`input-${i}`}
					type="target"
					position={Position.Left}
					id={`input-${i}`}
					style={{
						position: "absolute",
						left: "-8px",
						top: `${((i + 1) * 100) / (inputCount + 1)}%`,
						transform: "translateY(-50%)",
						width: "12px",
						height: "12px",
						background: "#f97316",
						border: "2px solid white",
						borderRadius: "50%",
					}}
				/>
			))}

			{/* Expression content */}
			<div className="p-3 text-center">
				<div className="font-bold text-orange-800 text-lg mb-1">{data.operation}</div>
				<div className="text-xs text-orange-600 mb-1">{data.description}</div>
				<div className="text-xs text-gray-500">→ {data.targetInput}</div>
			</div>

			{/* Output handles */}
			{Array.from({ length: outputCount }).map((_, i) => (
				<Handle
					key={`output-${i}`}
					type="source"
					position={Position.Right}
					id={`output-${i}`}
					style={{
						position: "absolute",
						right: "-8px",
						top: `${((i + 1) * 100) / (outputCount + 1)}%`,
						transform: "translateY(-50%)",
						width: "12px",
						height: "12px",
						background: "#f97316",
						border: "2px solid white",
						borderRadius: "50%",
					}}
				/>
			))}
		</div>
	);
}