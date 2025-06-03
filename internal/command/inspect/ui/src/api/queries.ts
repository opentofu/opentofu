import { api, queryKeys } from './client'

import { useQuery } from '@tanstack/react-query'

// Health check query
export const useHealth = () => {
  return useQuery({
    queryKey: queryKeys.health,
    queryFn: api.health,
    // Check health every 30 seconds
    refetchInterval: 30000,
  })
}

// Configuration query
export const useConfig = () => {
  return useQuery({
    queryKey: queryKeys.config,
    queryFn: api.config,
    // Config changes less frequently
    staleTime: 10 * 60 * 1000, // 10 minutes
  })
}

// Graph query  
export const useGraph = () => {
  return useQuery({
    queryKey: queryKeys.graph,
    queryFn: api.graph,
    // Graph might change if config changes
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

// Resource detail query
export const useResource = (id: string | undefined) => {
  return useQuery({
    queryKey: queryKeys.resource(id || ''),
    queryFn: () => api.resource(id as string),
    enabled: !!id, // Only run if id is provided
    staleTime: 10 * 60 * 1000, // 10 minutes
  })
}

// Source files query
export const useSourceFiles = () => {
  return useQuery({
    queryKey: queryKeys.sourceFiles,
    queryFn: api.sourceFiles,
    staleTime: 15 * 60 * 1000, // 15 minutes
  })
}

// Source content query
export const useSourceContent = (filename: string | undefined, startLine?: number, startCol?: number) => {
  return useQuery({
    queryKey: queryKeys.sourceContent(filename || '', startLine, startCol),
    queryFn: () => api.sourceContent(filename as string, startLine, startCol),
    enabled: !!filename, // Only run if filename is provided
    staleTime: 15 * 60 * 1000, // 15 minutes
  })
}
