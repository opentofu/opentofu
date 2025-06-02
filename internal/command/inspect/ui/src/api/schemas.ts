import { z } from 'zod'

// Health endpoint schema
export const HealthResponseSchema = z.object({
  status: z.string(),
  config_path: z.string(),
})

// Resource dependency schema
export const ResourceDependenciesSchema = z.object({
  explicit: z.array(z.string()),
  implicit: z.array(z.string()),
  crossModule: z.array(z.string()).optional(),
})

// Module context schema (defined early as it's used by other schemas)
export const ModuleContextSchema = z.object({
  path: z.string(),
  name: z.string(),
  source: z.string(),
  depth: z.number(),
  parent: z.string().nullable(),
  ancestorPath: z.array(z.string()),
})

// Resource schema
export const ResourceSchema = z.object({
  id: z.string(),
  type: z.string(),
  name: z.string(),
  mode: z.string(),
  provider: z.string(),
  parentId: z.string().nullable().optional(),
  module: ModuleContextSchema,
  dependencies: ResourceDependenciesSchema,
})

// Module call schema
export const ModuleCallSchema = z.object({
  name: z.string(),
  source: z.string(),
  version: z.string().nullable().optional(),
  inputs: z.record(z.any()).optional(),
  dependencies: z.array(z.string()).optional(),
})

// Module children schema
export const ModuleChildrenSchema = z.object({
  modules: z.array(z.string()),
  resources: z.array(z.string()),
})

// Module schema
export const ModuleSchema = z.object({
  id: z.string(),
  name: z.string(),
  path: z.string(),
  source: z.string(),
  calls: z.array(ModuleCallSchema).nullable(),
})

// Module hierarchy schema
export const ModuleHierarchySchema = z.object({
  id: z.string(),
  name: z.string(),
  path: z.string(),
  source: z.string(),
  depth: z.number(),
  parent: z.string().nullable(),
  children: ModuleChildrenSchema,
  variables: z.array(z.string()),
  outputs: z.array(z.string()),
  calls: z.array(ModuleCallSchema),
})

// Config response schema
export const ConfigResponseSchema = z.object({
  modules: z.array(ModuleSchema).nullable(),
  resources: z.array(ResourceSchema).nullable(),
  providers: z.array(z.object({
    name: z.string(),
    alias: z.string(),
  })).nullable(),
  variables: z.array(z.object({
    id: z.string(),
    name: z.string(),
    description: z.string(),
    sensitive: z.boolean(),
    parentId: z.string().nullable().optional(),
    module: ModuleContextSchema,
  })).nullable(),
  outputs: z.array(z.object({
    id: z.string(),
    name: z.string(),
    description: z.string(),
    sensitive: z.boolean(),
    parentId: z.string().nullable().optional(),
    module: ModuleContextSchema,
  })).nullable(),
})

// Graph node data schemas for different node types
export const ResourceNodeDataSchema = z.object({
  resourceType: z.string(),
  name: z.string(),
  mode: z.string(),
  provider: z.string(),
  modulePath: z.string().optional(),
  moduleAddress: z.string().optional(),
  source: z.string().optional(),
})

// Module call input schema
export const ModuleCallInputSchema = z.object({
  name: z.string(),
  type: z.string(),
  description: z.string(),
  default: z.any().optional(),
  required: z.boolean(),
})

// Module call output schema
export const ModuleCallOutputSchema = z.object({
  name: z.string(),
  type: z.string(),
  description: z.string(),
  sensitive: z.boolean(),
})

export const ModuleNodeDataSchema = z.object({
  name: z.string(),
  source: z.string(),
  modulePath: z.string(),
  hasChildren: z.boolean(),
  childResourceCount: z.number(),
  childModuleCount: z.number(),
  depth: z.number(),
  inputs: z.array(ModuleCallInputSchema),
  outputs: z.array(ModuleCallOutputSchema),
})

// Expression node data schema
export const ExpressionNodeDataSchema = z.object({
  operation: z.string(), // "+", "-", "*", "/", "==", "!=", etc.
  description: z.string(),
  targetID: z.string(),
  targetType: z.string(), // "module" or "resource"
  targetInput: z.string(),
  inputs: z.number(), // Number of input handles (usually 2)
  outputs: z.number(), // Number of output handles (usually 1)
})

// Static value node data schema
export const StaticValueNodeDataSchema = z.object({
  value: z.string(),
  type: z.string(),
  description: z.string(),
  side: z.enum(['LHS', 'RHS']), // Which side of the operation
})

// Unknown operand node data schema
export const UnknownOperandNodeDataSchema = z.object({
  side: z.string(),
  description: z.string(),
  exprType: z.string(),
})

// Source location schema
export const SourceLocationSchema = z.object({
  filename: z.string(),
  startLine: z.number(),
  endLine: z.number(),
  startCol: z.number().optional(),
  endCol: z.number().optional(),
})

// Graph node schema
export const GraphNodeSchema = z.object({
  id: z.string(),
  type: z.enum(['resource', 'module', 'provider', 'variable', 'output', 'expression', 'static_value', 'unknown_operand']),
  parentId: z.string().nullable().optional(),
  data: z.union([
    ResourceNodeDataSchema,
    ModuleNodeDataSchema,
    ExpressionNodeDataSchema,
    StaticValueNodeDataSchema,
    UnknownOperandNodeDataSchema,
    z.record(z.any()), // Fallback for other node types
  ]),
  source: SourceLocationSchema.optional(),
})

// Graph edge schema
export const GraphEdgeSchema = z.object({
  id: z.string(),
  source: z.string(),
  target: z.string(),
  type: z.enum(['depends_on', 'implicit', 'provider', 'module', 'module_input', 'module_output', 'expression_input', 'expression_output']),
  sourceHandle: z.string().optional(),
  targetHandle: z.string().optional(),
})

// Graph response schema
export const GraphResponseSchema = z.object({
  nodes: z.array(GraphNodeSchema).nullable(),
  edges: z.array(GraphEdgeSchema).nullable(),
})

// Resource detail response schema
export const ResourceDetailSchema = z.object({
  id: z.string(),
  type: z.string(),
  name: z.string(),
  mode: z.string(),
  provider: z.string(),
  module: ModuleContextSchema,
  dependencies: ResourceDependenciesSchema,
  attributes: z.record(z.any()).optional(),
})

// Hierarchy response schema
export const HierarchyResponseSchema = z.object({
  modules: z.record(ModuleHierarchySchema),
})

// Source file info schema
export const SourceFileInfoSchema = z.object({
  path: z.string(),
  size: z.number(),
  lines: z.number(),
})

// Source files response schema
export const SourceFilesResponseSchema = z.object({
  files: z.array(SourceFileInfoSchema),
})

// Source content response schema
export const SourceContentResponseSchema = z.object({
  filename: z.string(),
  content: z.string(),
  lines: z.array(z.string()),
  startLine: z.number(),
  endLine: z.number(),
  totalLines: z.number(),
})


// Export types
export type HealthResponse = z.infer<typeof HealthResponseSchema>
export type ResourceDependencies = z.infer<typeof ResourceDependenciesSchema>
export type Resource = z.infer<typeof ResourceSchema>
export type ModuleCall = z.infer<typeof ModuleCallSchema>
export type ModuleChildren = z.infer<typeof ModuleChildrenSchema>
export type Module = z.infer<typeof ModuleSchema>
export type ModuleHierarchy = z.infer<typeof ModuleHierarchySchema>
export type ModuleContext = z.infer<typeof ModuleContextSchema>
export type ConfigResponse = z.infer<typeof ConfigResponseSchema>
export type ResourceNodeData = z.infer<typeof ResourceNodeDataSchema>
export type ModuleCallInput = z.infer<typeof ModuleCallInputSchema>
export type ModuleCallOutput = z.infer<typeof ModuleCallOutputSchema>
export type ModuleNodeData = z.infer<typeof ModuleNodeDataSchema>
export type ExpressionNodeData = z.infer<typeof ExpressionNodeDataSchema>
export type StaticValueNodeData = z.infer<typeof StaticValueNodeDataSchema>
export type UnknownOperandNodeData = z.infer<typeof UnknownOperandNodeDataSchema>
export type SourceLocation = z.infer<typeof SourceLocationSchema>
export type GraphNode = z.infer<typeof GraphNodeSchema>
export type GraphEdge = z.infer<typeof GraphEdgeSchema>
export type GraphResponse = z.infer<typeof GraphResponseSchema>
export type ResourceDetail = z.infer<typeof ResourceDetailSchema>
export type HierarchyResponse = z.infer<typeof HierarchyResponseSchema>
export type SourceFileInfo = z.infer<typeof SourceFileInfoSchema>
export type SourceFilesResponse = z.infer<typeof SourceFilesResponseSchema>
export type SourceContentResponse = z.infer<typeof SourceContentResponseSchema>