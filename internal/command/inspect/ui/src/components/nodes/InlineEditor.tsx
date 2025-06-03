import { Handle, Position } from "@xyflow/react";
import { useEffect, useState } from "react";

import type { SourceLocation } from "../../api/schemas";
import { useSourceContent } from "../../api/queries";

interface InlineEditorProps {
	nodeId: string;
	sourceLocation: SourceLocation;
	onSave: (nodeId: string, content: string) => void;
	onCancel: (nodeId: string) => void;
}

export default function InlineEditor({ nodeId, sourceLocation, onSave, onCancel }: InlineEditorProps) {
	const [editContent, setEditContent] = useState("");
	const [isLoading, setIsLoading] = useState(true);

	// Fetch source content
	const { data: sourceData, isLoading: sourceLoading, error } = useSourceContent(
		sourceLocation.filename,
		sourceLocation.startLine,
		sourceLocation.startCol,
	);

	useEffect(() => {
		if (sourceData && !sourceLoading) {
			setEditContent(sourceData.content);
			setIsLoading(false);
		}
	}, [sourceData, sourceLoading]);

	const handleSave = () => {
		onSave(nodeId, editContent);
	};

	const handleCancel = () => {
		onCancel(nodeId);
	};

	if (isLoading || sourceLoading) {
		return (
			<div className="p-3 bg-white border-2 border-blue-500 rounded-lg shadow-lg">
				<div className="flex items-center justify-center h-20">
					<div className="text-sm text-gray-600">Loading source...</div>
				</div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="p-3 bg-white border-2 border-red-500 rounded-lg shadow-lg">
				<div className="text-sm text-red-600 mb-2">Error loading source</div>
				<button type="button" onClick={handleCancel} className="px-2 py-1 bg-gray-500 text-white text-xs rounded hover:bg-gray-600">
					Cancel
				</button>
			</div>
		);
	}

	return (
		<div className="p-3 bg-white border-2 border-blue-500 rounded-lg shadow-lg min-w-[400px]">
			<div className="mb-2">
				<div className="text-xs text-gray-600 mb-1">
					{sourceLocation.filename} (lines {sourceLocation.startLine}-{sourceLocation.endLine})
				</div>
				<textarea
					value={editContent}
					onChange={(e) => setEditContent(e.target.value)}
					className="w-full h-32 text-xs font-mono border border-gray-300 rounded p-2 resize-none focus:outline-none focus:ring-2 focus:ring-blue-500"
					placeholder="Edit source code..."
				/>
			</div>
			<div className="flex justify-end gap-2">
				<button type="button" onClick={handleCancel} className="px-3 py-1 bg-gray-500 text-white text-xs rounded hover:bg-gray-600">
					Cancel
				</button>
				<button type="button" onClick={handleSave} className="px-3 py-1 bg-blue-500 text-white text-xs rounded hover:bg-blue-600">
					Save
				</button>
			</div>
		</div>
	);
}
