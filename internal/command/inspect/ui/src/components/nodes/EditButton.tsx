interface EditButtonProps {
	nodeId: string;
	hasSource: boolean;
	onEdit: (nodeId: string) => void;
}

export default function EditButton({ nodeId, hasSource, onEdit }: EditButtonProps) {
	if (!hasSource) return null;

	return (
		<button
			type="button"
			onClick={(e) => {
				e.stopPropagation();
				onEdit(nodeId);
			}}
			className="absolute top-1 right-1 w-6 h-6 bg-gray-600 hover:bg-gray-800 text-white text-xs rounded flex items-center justify-center transition-colors z-10"
			title="Edit source code"
		>
			✏️
		</button>
	);
}