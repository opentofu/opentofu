import { useResource } from '../api/queries'
import type { Resource } from '../api/schemas'

interface ResourceDetailProps {
  resourceId: string | null
  onClose: () => void
}

function DependencyList({ 
  title, 
  dependencies, 
  type 
}: { 
  title: string
  dependencies: string[]
  type: 'explicit' | 'implicit'
}) {
  if (!dependencies || dependencies.length === 0) {
    return null
  }

  return (
    <div className="mb-4">
      <h4 className="text-sm font-medium text-gray-900 mb-2 flex items-center gap-2">
        {type === 'explicit' ? '🔗' : '⚡'} {title}
      </h4>
      <div className="space-y-1">
        {dependencies.map((dep, index) => (
          <div
            key={index}
            className={`text-xs px-2 py-1 rounded ${
              type === 'explicit' 
                ? 'bg-red-50 text-red-700 border border-red-200'
                : 'bg-blue-50 text-blue-700 border border-blue-200'
            }`}
          >
            {dep}
          </div>
        ))}
      </div>
    </div>
  )
}

function ResourceCard({ resource }: { resource: Resource }) {
  const isDataSource = resource.mode === 'DataResourceMode'
  
  return (
    <div className="bg-white border border-gray-200 rounded-lg p-4">
      <div className="flex items-start gap-3 mb-3">
        <div className={`text-2xl ${isDataSource ? '📖' : '🏗️'}`}>
          {isDataSource ? '📖' : '🏗️'}
        </div>
        <div className="flex-1 min-w-0">
          <h3 className="text-lg font-semibold text-gray-900 truncate">
            {resource.name}
          </h3>
          <p className="text-sm text-gray-600">{resource.type}</p>
          <p className="text-xs text-gray-500 mt-1">
            {isDataSource ? 'Data Source' : 'Managed Resource'}
          </p>
        </div>
      </div>

      <div className="space-y-3">
        <div>
          <span className="text-xs font-medium text-gray-700">Provider:</span>
          <div className="text-sm text-gray-900 font-mono bg-gray-50 px-2 py-1 rounded mt-1">
            {resource.provider}
          </div>
        </div>

        <div>
          <span className="text-xs font-medium text-gray-700">Full ID:</span>
          <div className="text-sm text-gray-900 font-mono bg-gray-50 px-2 py-1 rounded mt-1 break-all">
            {resource.id}
          </div>
        </div>

        <DependencyList
          title="Explicit Dependencies"
          dependencies={resource.dependencies.explicit}
          type="explicit"
        />

        <DependencyList
          title="Implicit Dependencies"
          dependencies={resource.dependencies.implicit}
          type="implicit"
        />

        {(!resource.dependencies.explicit.length && !resource.dependencies.implicit.length) && (
          <div className="text-center py-4 text-gray-500">
            <div className="text-2xl mb-2">🔓</div>
            <p className="text-sm">No dependencies found</p>
          </div>
        )}
      </div>
    </div>
  )
}

export default function ResourceDetail({ resourceId, onClose }: ResourceDetailProps) {
  const { data: resource, isLoading, error } = useResource(resourceId || undefined)

  if (!resourceId) {
    return (
      <div className="h-full flex items-center justify-center text-gray-500">
        <div className="text-center">
          <div className="text-4xl mb-2">👆</div>
          <p className="text-sm">Click on a resource to see details</p>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-4">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Resource Details</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            ✕
          </button>
        </div>
        <div className="error-message">
          <h3 className="font-semibold mb-2">Error</h3>
          <p>Failed to load resource details.</p>
          <p className="text-sm mt-2">{error.message}</p>
        </div>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="p-4">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Resource Details</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            ✕
          </button>
        </div>
        <div className="text-center py-8">
          <div className="loading-spinner mx-auto mb-4" />
          <p className="text-gray-600">Loading resource details...</p>
        </div>
      </div>
    )
  }

  if (!resource) {
    return (
      <div className="p-4">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Resource Details</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            ✕
          </button>
        </div>
        <div className="text-center py-8 text-gray-500">
          <div className="text-4xl mb-2">❓</div>
          <p>Resource not found</p>
        </div>
      </div>
    )
  }

  return (
    <div className="p-4 h-full overflow-y-auto">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold">Resource Details</h2>
        <button
          type="button"
          onClick={onClose}
          className="text-gray-400 hover:text-gray-600 text-xl"
        >
          ✕
        </button>
      </div>
      <ResourceCard resource={resource} />
    </div>
  )
}