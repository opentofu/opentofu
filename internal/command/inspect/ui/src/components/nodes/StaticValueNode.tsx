import { Handle, Position } from "@xyflow/react";
import EditButton from "./EditButton";
import InlineEditor from "./InlineEditor";

interface StaticValueNodeProps {
	data: any;
	id: string;
}

export default function StaticValueNode({ data, id }: StaticValueNodeProps) {
	// Color coding by type
	const getTypeColors = (type: string) => {
		switch (type.toLowerCase()) {
			case "string":
				return { border: "border-blue-500", bg: "bg-blue-50", text: "text-blue-800", handle: "#3b82f6" };
			case "number":
				return { border: "border-green-500", bg: "bg-green-50", text: "text-green-800", handle: "#10b981" };
			case "boolean":
			case "bool":
				return { border: "border-orange-500", bg: "bg-orange-50", text: "text-orange-800", handle: "#f97316" };
			default:
				return { border: "border-gray-500", bg: "bg-gray-50", text: "text-gray-800", handle: "#6b7280" };
		}
	};

	const colors = getTypeColors(data.type);
	const sideLabel = data.side ? `(${data.side})` : "";
	const { editingNodes, onEditNode, onSaveEdit, onCancelEdit } = data._graphContext || {};
	const isEditing = editingNodes?.has(id);
	const hasSource = !!data.source;

	if (isEditing && hasSource && data.source) {
		return <InlineEditor nodeId={id} sourceLocation={data.source} onSave={onSaveEdit} onCancel={onCancelEdit} />;
	}

	return (
		<div className={`node-static-value rounded-lg border-2 ${colors.border} ${colors.bg} shadow-md relative min-w-[100px] max-w-[140px]`}>
			{/* Edit button */}
			<EditButton nodeId={id} hasSource={hasSource} onEdit={onEditNode} />
			{/* Static value content */}
			<div className="p-2 text-center">
				<div className={`font-bold ${colors.text} text-sm mb-1`}>{data.value}</div>
				<div className={`text-xs ${colors.text} opacity-70`}>
					{data.type} {sideLabel}
				</div>
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
					background: colors.handle,
					border: "2px solid white",
					borderRadius: "50%",
				}}
			/>
		</div>
	);
}