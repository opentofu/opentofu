import { Handle, Position } from "@xyflow/react";
import EditButton from "./EditButton";
import InlineEditor from "./InlineEditor";

interface UnknownOperandNodeProps {
	data: any;
	id: string;
}

export default function UnknownOperandNode({ data, id }: UnknownOperandNodeProps) {
	const { editingNodes, onEditNode, onSaveEdit, onCancelEdit } = data._graphContext || {};
	const isEditing = editingNodes?.has(id);
	const hasSource = !!data.source;

	if (isEditing && hasSource && data.source) {
		return <InlineEditor nodeId={id} sourceLocation={data.source} onSave={onSaveEdit} onCancel={onCancelEdit} />;
	}

	return (
		<div className="node-unknown-operand rounded-lg border-2 border-gray-400 bg-gray-100 shadow-md relative min-w-[100px] max-w-[140px]">
			{/* Edit button */}
			<EditButton nodeId={id} hasSource={hasSource} onEdit={onEditNode} />
			{/* Unknown operand content */}
			<div className="p-2 text-center">
				<div className="font-bold text-gray-700 text-sm mb-1">❓ {data.side}</div>
				<div className="text-xs text-gray-600 opacity-70">{data.exprType}</div>
				<div className="text-xs text-gray-500 mt-1">{data.description}</div>
			</div>

			{/* Output handle */}
			<Handle
				type="source"
				position={Position.Right}
				style={{
					position: "absolute",
					right: "-8px",
					top: "50%",
					transform: "translateY(-50%)",
					width: "10px",
					height: "10px",
					background: "#6b7280",
					border: "2px solid white",
					borderRadius: "50%",
				}}
			/>
		</div>
	);
}