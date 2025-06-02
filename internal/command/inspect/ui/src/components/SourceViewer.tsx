import { useSourceContent } from "../api/queries";
import type { SourceLocation } from "../api/schemas";

interface SourceViewerProps {
  filename: string;
  highlightStart?: number;
  highlightEnd?: number;
}

export default function SourceViewer({ filename, highlightStart, highlightEnd }: SourceViewerProps) {
  const { data: sourceContent, isLoading, error } = useSourceContent(filename, highlightStart, highlightEnd);

  if (isLoading) {
    return (
      <div className="source-viewer border border-gray-200 rounded-lg overflow-hidden">
        <div className="source-header bg-gray-50 px-4 py-3 border-b border-gray-200">
          <div className="flex items-center gap-2">
            <div className="text-sm font-medium text-gray-700">📄 {filename}</div>
            {highlightStart && highlightEnd && (
              <div className="text-xs text-gray-500">Lines {highlightStart}-{highlightEnd}</div>
            )}
          </div>
        </div>
        <div className="p-4 text-center text-gray-500">
          <div className="loading-spinner mx-auto mb-2" />
          Loading source code...
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="source-viewer border border-red-200 rounded-lg overflow-hidden">
        <div className="source-header bg-red-50 px-4 py-3 border-b border-red-200">
          <div className="flex items-center gap-2">
            <div className="text-sm font-medium text-red-700">📄 {filename}</div>
            <div className="text-xs text-red-500">Error loading file</div>
          </div>
        </div>
        <div className="p-4 text-center text-red-600">
          <div className="text-sm">Failed to load source code</div>
          <div className="text-xs mt-1">{error.message}</div>
        </div>
      </div>
    );
  }

  if (!sourceContent) {
    return null;
  }

  return (
    <div className="source-viewer border border-gray-200 rounded-lg overflow-hidden shadow-sm">
      {/* Header */}
      <div className="source-header bg-gray-50 px-4 py-3 border-b border-gray-200">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="text-sm font-medium text-gray-700">📄 {sourceContent.filename}</div>
            {sourceContent.startLine !== sourceContent.endLine && (
              <div className="text-xs text-gray-500">
                Lines {sourceContent.startLine}-{sourceContent.endLine}
              </div>
            )}
          </div>
          <div className="text-xs text-gray-500">
            {sourceContent.totalLines} total lines
          </div>
        </div>
      </div>

      {/* Source code */}
      <div className="source-code bg-gray-50 max-h-96 overflow-auto">
        <pre className="text-sm leading-relaxed m-0 p-0">
          {sourceContent.lines.map((line, index) => {
            const lineNumber = sourceContent.startLine + index;
            const isHighlighted = 
              highlightStart && highlightEnd &&
              lineNumber >= highlightStart && 
              lineNumber <= highlightEnd;

            return (
              <div
                key={index}
                className={`source-line flex hover:bg-gray-100 ${
                  isHighlighted ? 'bg-yellow-100 border-l-3 border-yellow-400' : ''
                }`}
              >
                <div className="line-number select-none text-gray-400 text-right min-w-[3rem] px-3 py-1 bg-gray-100 border-r border-gray-200">
                  {lineNumber}
                </div>
                <div className="line-content flex-1 px-3 py-1 font-mono whitespace-pre-wrap break-all">
                  {line || '\u00A0'} {/* Non-breaking space for empty lines */}
                </div>
              </div>
            );
          })}
        </pre>
      </div>
    </div>
  );
}

interface SourceInfoProps {
  source: SourceLocation;
  onViewSource?: () => void;
}

export function SourceInfo({ source, onViewSource }: SourceInfoProps) {
  return (
    <div className="source-info flex items-center gap-2 px-2 py-1 bg-gray-100 rounded text-xs text-gray-600">
      <span>📄 {source.filename}:{source.startLine}-{source.endLine}</span>
      {onViewSource && (
        <button
          type="button"
          onClick={onViewSource}
          className="text-blue-600 hover:text-blue-800 hover:underline"
        >
          View Source
        </button>
      )}
    </div>
  );
}