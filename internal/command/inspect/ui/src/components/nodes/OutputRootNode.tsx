import { Handle, Position } from "@xyflow/react";

interface ResourceNodeProps {
    data: any;
    id: string;
}

export default function OutputRootNode({ data, id }: ResourceNodeProps) {
    return (
        <div
            className={`relative px-3 py-2 rounded-lg text-xs font-medium border-2 border-red-500 bg-red-50 text-red-800 min-w-[120px]`}
        >
            <Handle type="target" position={Position.Left} />
            <div className="font-semibold">{data.name}</div>
            <div className="text-xs opacity-80">{data.resourceType}</div>
            {data.module && <div className="text-xs opacity-60 mt-1">📁 {data.module}</div>}
            <Handle type="source" position={Position.Right} />
        </div>
    );
}
