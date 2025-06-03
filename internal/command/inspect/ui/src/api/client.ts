import { QueryClient } from '@tanstack/react-query'
import { 
  HealthResponseSchema, 
  ConfigResponseSchema, 
  GraphResponseSchema, 
  ResourceDetailSchema,
  SourceFilesResponseSchema,
  SourceContentResponseSchema,
  type HealthResponse,
  type ConfigResponse,
  type GraphResponse,
  type ResourceDetail,
  type SourceFilesResponse,
  type SourceContentResponse,
} from './schemas'

// Create a query client with default options - stable across HMR
let globalQueryClient: QueryClient | undefined = undefined

function getQueryClient() {
  if (!globalQueryClient) {
    globalQueryClient = new QueryClient({
      defaultOptions: {
        queries: {
          staleTime: 5 * 60 * 1000, // 5 minutes
          refetchOnWindowFocus: false,
          retry: 2,
        },
      },
    })
  }
  return globalQueryClient
}

export const queryClient = getQueryClient()

// Base API URL - detect if we're in development mode
const API_BASE = import.meta.env?.DEV 
  ? 'http://localhost:8080/api'  // Development server
  : '/api'                       // Production (same origin)

// Generic fetch wrapper with error handling
async function apiFetch<T>(endpoint: string, schema: any): Promise<T> {
  const response = await fetch(`${API_BASE}${endpoint}`)
  
  if (!response.ok) {
    throw new Error(`API Error: ${response.status} ${response.statusText}`)
  }
  
  const data = await response.json()
  
  // Validate response with Zod schema
  const parsed = schema.parse(data)
  return parsed as T
}

// API functions
export const api = {
  // Health check
  health: (): Promise<HealthResponse> =>
    apiFetch('/health', HealthResponseSchema),
  
  // Get configuration structure
  config: (): Promise<ConfigResponse> =>
    apiFetch('/config', ConfigResponseSchema),
  
  // Get graph data
  graph: (): Promise<GraphResponse> =>
    apiFetch('/graph', GraphResponseSchema),
  
  // Get resource details
  resource: (id: string): Promise<ResourceDetail> =>
    apiFetch(`/resource/${encodeURIComponent(id)}`, ResourceDetailSchema),
  
  // Get source files list
  sourceFiles: (): Promise<SourceFilesResponse> =>
    apiFetch('/source/files', SourceFilesResponseSchema),
  
  // Get source content
  sourceContent: (filename: string, startLine?: number, startCol?: number): Promise<SourceContentResponse> => {
    const params = new URLSearchParams({ file: filename });
    if (startLine) params.set('startLine', startLine.toString());
    if (startCol) params.set('startCol', startCol.toString());
    return apiFetch(`/source/content?${params}`, SourceContentResponseSchema);
  },
}

// Query keys for TanStack Query
export const queryKeys = {
  health: ['health'] as const,
  config: ['config'] as const,
  graph: ['graph'] as const,
  resource: (id: string) => ['resource', id] as const,
  sourceFiles: ['source', 'files'] as const,
  sourceContent: (filename: string, startLine?: number, startCol?: number) =>
    ['source', 'content', filename, startLine, startCol] as const,
}
