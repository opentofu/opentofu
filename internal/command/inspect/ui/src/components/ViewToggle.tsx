interface ViewToggleProps {
  view: 'config' | 'graph'
  onViewChange: (view: 'config' | 'graph') => void
  configCount?: number
  graphCount?: number
}

export default function ViewToggle({ 
  view, 
  onViewChange, 
  configCount = 0, 
  graphCount = 0 
}: ViewToggleProps) {
  return (
    <div className="flex bg-gray-100 rounded-lg p-1 gap-1">
      <button
        onClick={() => onViewChange('config')}
        className={`px-4 py-2 rounded-md text-sm font-medium transition-colors flex items-center gap-2 ${
          view === 'config'
            ? 'bg-white text-gray-900 shadow-sm'
            : 'text-gray-600 hover:text-gray-900'
        }`}
      >
        📄 Config View
        {configCount > 0 && (
          <span className="bg-gray-200 text-gray-700 px-2 py-0.5 rounded-full text-xs">
            {configCount}
          </span>
        )}
      </button>
      
      <button
        onClick={() => onViewChange('graph')}
        className={`px-4 py-2 rounded-md text-sm font-medium transition-colors flex items-center gap-2 ${
          view === 'graph'
            ? 'bg-white text-gray-900 shadow-sm'
            : 'text-gray-600 hover:text-gray-900'
        }`}
      >
        🕸️ Graph View
        {graphCount > 0 && (
          <span className="bg-gray-200 text-gray-700 px-2 py-0.5 rounded-full text-xs">
            {graphCount}
          </span>
        )}
      </button>
    </div>
  )
}

export function ViewDescription({ view }: { view: 'config' | 'graph' }) {
  return (
    <div className="text-xs text-gray-600 mt-2">
      {view === 'config' ? (
        <span>Shows dependencies as explicitly written in configuration files</span>
      ) : (
        <span>Shows the complete dependency graph that OpenTofu's graph walker builds</span>
      )}
    </div>
  )
}