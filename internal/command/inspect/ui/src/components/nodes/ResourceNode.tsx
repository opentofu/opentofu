import { Handle, Position } from "@xyflow/react";
import EditButton from "./EditButton";
import InlineEditor from "./InlineEditor";

interface ResourceNodeProps {
	data: any;
	id: string;
}

export default function ResourceNode({ data, id }: ResourceNodeProps) {
	const isDataSource = data.mode === "DataResourceMode";
	const { editingNodes, onEditNode, onSaveEdit, onCancelEdit } = data._graphContext || {};
	const isEditing = editingNodes?.has(id);
	const hasSource = !!data.source;

	return (
		<div
			className={`relative px-3 py-2 rounded-lg text-xs font-medium border-2 ${
				isDataSource ? "border-blue-500 bg-blue-50 text-blue-800" : "border-green-500 bg-green-50 text-green-800"
			} min-w-[120px]`}
		>
			<Handle type="target" position={Position.Left} />
			<EditButton nodeId={id} hasSource={hasSource} onEdit={onEditNode} />
			<div className="font-semibold">{data.name}</div>
			<div className="text-xs opacity-80">{data.resourceType}</div>
			{data.module && <div className="text-xs opacity-60 mt-1">📁 {data.module}</div>}
			<Handle type="source" position={Position.Right} />
			
			{/* Editor overlay */}
			{isEditing && hasSource && data.source && (
				<div className="absolute inset-0 z-50">
					<InlineEditor nodeId={id} sourceLocation={data.source} onSave={onSaveEdit} onCancel={onCancelEdit} />
				</div>
			)}
		</div>
	);
}
