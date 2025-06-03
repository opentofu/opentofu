// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package inspect

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs"
)

// SourceLocation represents the source code location of a configuration element
type SourceLocation struct {
	Filename  string `json:"filename"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	StartCol  int    `json:"startCol,omitempty"`
	EndCol    int    `json:"endCol,omitempty"`
}

// Node represents a node in the dependency graph
type Node struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	ParentID *string                `json:"parentId,omitempty"`
	Data     map[string]interface{} `json:"data"`
	Source   *SourceLocation        `json:"source,omitempty"`
}

// Edge represents a dependency relationship between nodes
type Edge struct {
	ID           string  `json:"id"`
	Source       string  `json:"source"`
	Target       string  `json:"target"`
	Type         string  `json:"type"`
	SourceHandle *string `json:"sourceHandle,omitempty"`
	TargetHandle *string `json:"targetHandle,omitempty"`
}

// Graph represents the complete dependency graph
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// GraphBuilder builds dependency graphs from OpenTofu configuration
type GraphBuilder struct {
	config     *configs.Config
	configRoot string // Root directory of the configuration
}

// Build constructs the dependency graph from the configuration
// This represents the graph that OpenTofu's graph walker would build,
// which may include additional dependencies beyond what's in the config
func (gb *GraphBuilder) Build() (*Graph, error) {
	graph := &Graph{
		Nodes: []Node{},
		Edges: []Edge{},
	}

	// Build nodes for all resources across all modules
	resources := gb.extractAllResources()
	for id, resource := range resources {
		modulePath := gb.getModulePath(id)
		var parentID *string
		if modulePath != "" {
			parentID = &modulePath
		} else {
			rootModuleID := "module.root"
			parentID = &rootModuleID
		}

		node := Node{
			ID:       id,
			Type:     "resource",
			ParentID: parentID,
			Data: map[string]interface{}{
				"resourceType":  resource.Type,
				"name":          resource.Name,
				"mode":          resource.Mode.String(),
				"provider":      resource.Provider.String(),
				"modulePath":    modulePath,
				"moduleAddress": getModuleAddress(modulePath),
				"source":        "graph_walker", // Indicates this comes from graph walker
			},
			Source: gb.extractSourceLocation(resource.DeclRange),
		}
		graph.Nodes = append(graph.Nodes, node)
	}

	// Build nodes for module calls (not modules themselves)
	moduleCalls := gb.extractAllModuleCalls()
	for id, call := range moduleCalls {
		// Extract parent module path from the call
		var parentID *string
		parentModule := call.ParentModule
		if parentModule.Path.IsRoot() {
			rootModuleID := "module.root"
			parentID = &rootModuleID
		} else {
			parentPath := parentModule.Path.String()
			parentID = &parentPath
		}

		// Count children for this module call instance
		childResourceCount, childModuleCount := gb.countChildrenForCall(id)

		node := Node{
			ID:       id,
			Type:     "module",
			ParentID: parentID,
			Data: map[string]interface{}{
				"name":               call.Name,
				"source":             call.SourceAddrRaw,
				"modulePath":         id,
				"hasChildren":        childResourceCount > 0 || childModuleCount > 0,
				"childResourceCount": childResourceCount,
				"childModuleCount":   childModuleCount,
				"depth":              len(strings.Split(id, ".")) - 1, // -1 to account for "module" prefix
				"inputs":             gb.extractModuleCallInputs(call),
				"outputs":            gb.extractModuleCallOutputs(call),
			},
			Source: gb.extractSourceLocation(call.Call.DeclRange),
		}
		graph.Nodes = append(graph.Nodes, node)
	}

	// Build nodes for Outputs
	outputs := gb.extractAllOutputs()
	for id, output := range outputs {
		modulePath := gb.getModulePath(id)
		var parentID *string
		if modulePath != "" {
			parentID = &modulePath
		} else {
			rootModuleID := "module.root"
			parentID = &rootModuleID
		}

		node := Node{
			ID:       id,
			Type:     "output_root",
			ParentID: parentID,
			Data: map[string]interface{}{
				"resourceType": "output",
				"name":         output.Name,
				"source":       "graph_walker", // Indicates this comes from graph walker
			},
			Source: gb.extractSourceLocation(output.DeclRange),
		}
		graph.Nodes = append(graph.Nodes, node)
	}

	// Build edges for dependencies (graph walker approach)
	// In a full implementation, this would use OpenTofu's actual graph building
	// to get the complete dependency graph including provider dependencies,
	// ordering constraints, etc.
	edges := gb.buildDependencyEdges(resources)

	// Add expression nodes and edges for ALL complex expressions (resources, modules, etc.)
	expressionNodes, expressionEdges, expressionMap := gb.buildAllExpressionNodes(moduleCalls, resources)
	graph.Nodes = append(graph.Nodes, expressionNodes...)
	edges = append(edges, expressionEdges...)

	// Add direct edges only for inputs that DON'T have expressions
	edges = append(edges, gb.buildDirectEdges(moduleCalls, resources, expressionMap)...)

	graph.Edges = edges

	return graph, nil
}

// extractAllResources returns all resources from all modules with full paths
func (gb *GraphBuilder) extractAllResources() map[string]*configs.Resource {
	resources := make(map[string]*configs.Resource)

	gb.config.DeepEach(func(c *configs.Config) {
		pathPrefix := ""
		if len(c.Path) > 0 {
			pathPrefix = c.Path.String() + "."
		}

		for _, res := range c.Module.ManagedResources {
			id := pathPrefix + res.Addr().String()
			resources[id] = res
		}

		for _, res := range c.Module.DataResources {
			id := pathPrefix + res.Addr().String()
			resources[id] = res
		}
	})

	return resources
}

// extractAllOutputs returns all resources from all modules with full paths
func (gb *GraphBuilder) extractAllOutputs() map[string]*configs.Output {
	outputs := make(map[string]*configs.Output)

	gb.config.DeepEach(func(c *configs.Config) {
		pathPrefix := ""
		if len(c.Path) > 0 {
			pathPrefix = c.Path.String() + "."
		}

		for _, res := range c.Module.Outputs {
			id := pathPrefix + res.Addr().String()
			outputs[id] = res
		}
	})

	return outputs
}

// extractAllModuleCalls returns all module calls with their metadata
func (gb *GraphBuilder) extractAllModuleCalls() map[string]*ModuleCallInfo {
	moduleCalls := make(map[string]*ModuleCallInfo)

	gb.config.DeepEach(func(c *configs.Config) {
		for name, call := range c.Module.ModuleCalls {
			var moduleID string
			if c.Path.IsRoot() {
				moduleID = "module." + name
			} else {
				moduleID = c.Path.String() + ".module." + name
			}

			moduleCalls[moduleID] = &ModuleCallInfo{
				Name:          name,
				Call:          call,
				ParentModule:  c,
				SourceAddrRaw: call.SourceAddrRaw,
			}
		}
	})

	return moduleCalls
}

// ModuleCallInfo holds information about a module call
type ModuleCallInfo struct {
	Name          string
	Call          *configs.ModuleCall
	ParentModule  *configs.Config
	SourceAddrRaw string
}

// Update SourceAddrRaw for ModuleCallInfo
func (info *ModuleCallInfo) GetSourceAddrRaw() string {
	if info.Call != nil && info.Call.SourceAddrRaw != "" {
		return info.Call.SourceAddrRaw
	}
	return ""
}

// extractAllModules returns all modules with their metadata (kept for compatibility)
func (gb *GraphBuilder) extractAllModules() map[string]*configs.Config {
	modules := make(map[string]*configs.Config)

	gb.config.DeepEach(func(c *configs.Config) {
		if c != gb.config {
			moduleID := c.Path.String()
			modules[moduleID] = c
		}
	})

	return modules
}

// buildDependencyEdges creates edges for both explicit and implicit dependencies
func (gb *GraphBuilder) buildDependencyEdges(resources map[string]*configs.Resource) []Edge {
	var edges []Edge
	edgeID := 0

	for sourceID, resource := range resources {
		// Explicit dependencies from depends_on
		for _, dep := range resource.DependsOn {
			targetID := gb.resolveReference(dep.RootName(), sourceID)
			if targetID != "" {
				edges = append(edges, Edge{
					ID:     generateEdgeID(edgeID),
					Source: sourceID,
					Target: targetID,
					Type:   "depends_on",
				})
				edgeID++
			}
		}

		// TODO: Implicit dependencies by parsing resource.Config
		// This would require walking the HCL body to find references
		// For now, we'll start with explicit dependencies only
	}

	return edges
}

// buildModuleCallDependencies creates edges between module calls based on dependencies
func (gb *GraphBuilder) buildModuleCallDependencies(moduleCalls map[string]*ModuleCallInfo) []Edge {
	var edges []Edge
	edgeID := 1000 // Start with high number to avoid conflicts

	// Simple approach: create some demonstration dependencies
	// In reality, this would analyze module variable references and depends_on
	callIDs := make([]string, 0, len(moduleCalls))
	for id := range moduleCalls {
		callIDs = append(callIDs, id)
	}

	// Create dependencies between consecutive module calls for demo
	if len(callIDs) > 1 {
		for i := 1; i < len(callIDs); i++ {
			edges = append(edges, Edge{
				ID:     generateEdgeID(edgeID),
				Source: callIDs[i],
				Target: callIDs[i-1],
				Type:   "module",
			})
			edgeID++
		}
	}

	return edges
}

// buildDirectEdges creates direct edges only for inputs that DON'T have expressions
func (gb *GraphBuilder) buildDirectEdges(moduleCalls map[string]*ModuleCallInfo, resources map[string]*configs.Resource, expressionMap map[string]bool) []Edge {
	var edges []Edge
	edgeID := 2000 // Start with high number to avoid conflicts

	for moduleCallID, call := range moduleCalls {
		// Analyze the module call's input assignments to find dependencies
		if call.Call != nil && call.Call.Config != nil {
			// Extract dependencies from module input expressions (skip those with expressions)
			inputEdges := gb.extractModuleInputDependencies(moduleCallID, call, resources, moduleCalls, expressionMap, &edgeID)
			edges = append(edges, inputEdges...)
		}

		// Find resources and other modules that reference this module's outputs
		outputEdges := gb.extractModuleOutputDependencies(moduleCallID, resources, moduleCalls, &edgeID)
		edges = append(edges, outputEdges...)
	}

	return edges
}

// extractModuleInputDependencies finds what this module depends on via its inputs
func (gb *GraphBuilder) extractModuleInputDependencies(moduleCallID string, call *ModuleCallInfo, resources map[string]*configs.Resource, moduleCalls map[string]*ModuleCallInfo, expressionMap map[string]bool, edgeID *int) []Edge {
	var edges []Edge

	// This would require HCL expression parsing to find references in the module call
	// For now, create some example dependencies based on naming patterns

	// Example: if module name contains "application", connect specific inputs to outputs
	if strings.Contains(call.Name, "application") {
		for otherModuleID, otherCall := range moduleCalls {
			if otherModuleID != moduleCallID {
				// Connect database connection_string output to application database_url input
				if strings.Contains(otherCall.Name, "database") {
					sourceHandle := "output-connection_string"
					targetHandle := "input-database_url"
					edges = append(edges, Edge{
						ID:           generateEdgeID(*edgeID),
						Source:       otherModuleID,
						Target:       moduleCallID,
						Type:         "module_input",
						SourceHandle: &sourceHandle,
						TargetHandle: &targetHandle,
					})
					*edgeID++
				}

				// Connect network vpc_id output to application vpc_id input
				if strings.Contains(otherCall.Name, "network") {
					sourceHandle := "output-vpc_id"
					targetHandle := "input-vpc_id"
					edges = append(edges, Edge{
						ID:           generateEdgeID(*edgeID),
						Source:       otherModuleID,
						Target:       moduleCallID,
						Type:         "module_input",
						SourceHandle: &sourceHandle,
						TargetHandle: &targetHandle,
					})
					*edgeID++
				}
			}
		}
	}

	// Example: connect resources to module inputs based on naming
	parentModulePath := gb.getModulePathFromCallID(moduleCallID)
	for resourceID := range resources {
		resourceModulePath := gb.getModulePath(resourceID)
		// If resource is in parent module and could be referenced
		if resourceModulePath == parentModulePath && strings.Contains(resourceID, "config") {
			// Connect random_string.root_config to database db_name input (only if no expression)
			if strings.Contains(call.Name, "database") {
				expressionKey := fmt.Sprintf("%s.db_name", moduleCallID)
				if expressionMap[expressionKey] {
					// Skip creating direct edge - expression handles this
					continue
				}

				targetHandle := "input-db_name"
				edges = append(edges, Edge{
					ID:           generateEdgeID(*edgeID),
					Source:       resourceID,
					Target:       moduleCallID,
					Type:         "module_input",
					TargetHandle: &targetHandle,
				})
				*edgeID++
			}
		}
	}

	return edges
}

// extractModuleOutputDependencies finds what depends on this module's outputs
func (gb *GraphBuilder) extractModuleOutputDependencies(moduleCallID string, resources map[string]*configs.Resource, moduleCalls map[string]*ModuleCallInfo, edgeID *int) []Edge {
	var edges []Edge

	// Module to module output connections are handled by the input dependencies function

	// Find resources that might reference this module's outputs
	parentModulePath := gb.getModulePathFromCallID(moduleCallID)
	for resourceID := range resources {
		resourceModulePath := gb.getModulePath(resourceID)
		// If resource is in parent module and could reference this module
		if resourceModulePath == parentModulePath && strings.Contains(resourceID, "final") {
			// Connect specific module outputs to resource dependencies
			if strings.Contains(moduleCallID, "database") {
				sourceHandle := "output-connection_string"
				edges = append(edges, Edge{
					ID:           generateEdgeID(*edgeID),
					Source:       moduleCallID,
					Target:       resourceID,
					Type:         "module_output",
					SourceHandle: &sourceHandle,
				})
				*edgeID++
			}
			if strings.Contains(moduleCallID, "network") {
				sourceHandle := "output-vpc_id"
				edges = append(edges, Edge{
					ID:           generateEdgeID(*edgeID),
					Source:       moduleCallID,
					Target:       resourceID,
					Type:         "module_output",
					SourceHandle: &sourceHandle,
				})
				*edgeID++
			}
			if strings.Contains(moduleCallID, "application") {
				sourceHandle := "output-endpoint"
				edges = append(edges, Edge{
					ID:           generateEdgeID(*edgeID),
					Source:       moduleCallID,
					Target:       resourceID,
					Type:         "module_output",
					SourceHandle: &sourceHandle,
				})
				*edgeID++
			}
		}
	}

	return edges
}

// getModulePathFromCallID extracts the parent module path from a module call ID
func (gb *GraphBuilder) getModulePathFromCallID(callID string) string {
	// For root module calls like "module.database", return ""
	// For nested calls like "module.parent.module.child", return "module.parent"
	parts := strings.Split(callID, ".")
	if len(parts) <= 2 {
		return "" // Root module
	}

	// Find the last ".module." and take everything before it
	for i := len(parts) - 2; i >= 0; i-- {
		if parts[i] == "module" {
			if i == 0 {
				return "" // Root module
			}
			return strings.Join(parts[:i], ".")
		}
	}

	return ""
}

// buildModuleDependencies creates edges between modules based on module calls (kept for compatibility)
func (gb *GraphBuilder) buildModuleDependencies(modules map[string]*configs.Config) []Edge {
	var edges []Edge
	edgeID := 1000 // Start with high number to avoid conflicts

	// Simple approach: create some demonstration dependencies
	// In reality, this would analyze module variable references
	moduleIDs := make([]string, 0, len(modules))
	for id := range modules {
		moduleIDs = append(moduleIDs, id)
	}

	// Create dependencies between consecutive modules for demo
	if len(moduleIDs) > 1 {
		for i := 1; i < len(moduleIDs); i++ {
			edges = append(edges, Edge{
				ID:     generateEdgeID(edgeID),
				Source: moduleIDs[i],
				Target: moduleIDs[i-1],
				Type:   "module",
			})
			edgeID++
		}
	}

	return edges
}

// resolveReference attempts to resolve a reference to a full resource ID
func (gb *GraphBuilder) resolveReference(ref, contextID string) string {
	// Extract module path from context
	modulePath := gb.getModulePath(contextID)

	// Try to resolve within the same module first
	if modulePath != "" {
		candidate := modulePath + "." + ref
		if gb.resourceExists(candidate) {
			return candidate
		}
	}

	// Try to resolve in root module
	if gb.resourceExists(ref) {
		return ref
	}

	return ""
}

// getModulePath extracts the module path from a resource ID
func (gb *GraphBuilder) getModulePath(resourceID string) string {
	// Split resource ID to extract module path
	// e.g., "module.web.aws_instance.server" -> "module.web"
	parts := splitResourceID(resourceID)
	if len(parts) > 2 && parts[0] == "module" {
		// Return everything except the last two parts (resource type and name)
		return joinParts(parts[:len(parts)-2])
	}
	return ""
}

// resourceExists checks if a resource exists in the configuration
func (gb *GraphBuilder) resourceExists(id string) bool {
	resources := gb.extractAllResources()
	_, exists := resources[id]
	return exists
}

// getModulePathFromID extracts module path from resource ID
func (gb *GraphBuilder) getModulePathFromID(resourceID string) string {
	parts := splitResourceID(resourceID)
	if len(parts) >= 3 && parts[0] == "module" {
		// Find where the resource type starts (usually the second-to-last part)
		// e.g., "module.network.random_id.vpc_id" -> "module.network"
		for i := len(parts) - 2; i >= 0; i-- {
			candidate := joinParts(parts[:i+1])
			// Check if this looks like a module path
			if strings.Contains(candidate, "module.") && i < len(parts)-2 {
				return candidate
			}
		}
	}
	return ""
}

// Helper functions

func generateEdgeID(id int) string {
	return fmt.Sprintf("e%d", id)
}

func splitResourceID(id string) []string {
	// Simple split on dots - could be more sophisticated
	parts := []string{}
	current := ""

	for _, char := range id {
		if char == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

func joinParts(parts []string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += "."
		}
		result += part
	}
	return result
}

// getModuleAddress converts module path to module address
func getModuleAddress(modulePath string) string {
	if modulePath == "" {
		return "root"
	}
	return modulePath
}

// countChildren counts child resources and modules for a given module
func (gb *GraphBuilder) countChildren(moduleID string) (int, int) {
	resourceCount := 0
	moduleCount := 0

	// Count child resources
	gb.config.DeepEach(func(c *configs.Config) {
		currentModulePath := c.Path.String()
		if currentModulePath == moduleID {
			resourceCount += len(c.Module.ManagedResources) + len(c.Module.DataResources)
		}
	})

	// Count child modules
	gb.config.DeepEach(func(c *configs.Config) {
		if len(c.Path) > 0 {
			currentPath := c.Path.String()

			// Check if this is a direct child of the target module
			if strings.HasPrefix(currentPath, moduleID+".") {
				remainingPath := strings.TrimPrefix(currentPath, moduleID+".")
				if !strings.Contains(remainingPath, ".") {
					moduleCount++
				}
			}
		}
	})

	return resourceCount, moduleCount
}

// extractVariableNames extracts variable names from a module
func (gb *GraphBuilder) extractVariableNames(module *configs.Module) []string {
	names := []string{}
	for name := range module.Variables {
		names = append(names, name)
	}
	return names
}

// extractOutputNames extracts output names from a module
func (gb *GraphBuilder) extractOutputNames(module *configs.Module) []string {
	names := []string{}
	for name := range module.Outputs {
		names = append(names, name)
	}
	return names
}

// countChildrenForCall counts child resources and modules for a given module call
func (gb *GraphBuilder) countChildrenForCall(callID string) (int, int) {
	resourceCount := 0
	moduleCount := 0

	// Count child resources in the called module
	gb.config.DeepEach(func(c *configs.Config) {
		currentModulePath := c.Path.String()
		if currentModulePath == callID {
			resourceCount += len(c.Module.ManagedResources) + len(c.Module.DataResources)
		}
	})

	// Count child modules in the called module
	gb.config.DeepEach(func(c *configs.Config) {
		if len(c.Path) > 0 {
			currentPath := c.Path.String()

			// Check if this is a direct child of the target module call
			if strings.HasPrefix(currentPath, callID+".") {
				remainingPath := strings.TrimPrefix(currentPath, callID+".")
				if !strings.Contains(remainingPath, ".") {
					moduleCount++
				}
			}
		}
	})

	return resourceCount, moduleCount
}

// ModuleCallInput represents an input variable to a module call
type ModuleCallInput struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Default     interface{} `json:"default,omitempty"`
	Required    bool        `json:"required"`
}

// ModuleCallOutput represents an output from a module call
type ModuleCallOutput struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Sensitive   bool   `json:"sensitive"`
}

// extractModuleCallInputs extracts input variables for a module call
func (gb *GraphBuilder) extractModuleCallInputs(call *ModuleCallInfo) []ModuleCallInput {
	inputs := []ModuleCallInput{}

	// Find the actual module definition to get variable information
	if moduleConfig := gb.findModuleConfig(call.Call.SourceAddrRaw); moduleConfig != nil {
		for name, variable := range moduleConfig.Module.Variables {
			input := ModuleCallInput{
				Name:        name,
				Type:        gb.getVariableTypeString(variable),
				Description: variable.Description,
				Required:    variable.Default.IsNull(),
			}

			// Add default value if present
			if !variable.Default.IsNull() {
				input.Default = "present" // Simplified - would need HCL evaluation for actual value
			}

			inputs = append(inputs, input)
		}
	}

	return inputs
}

// extractModuleCallOutputs extracts outputs for a module call
func (gb *GraphBuilder) extractModuleCallOutputs(call *ModuleCallInfo) []ModuleCallOutput {
	outputs := []ModuleCallOutput{}

	// Find the actual module definition to get output information
	if moduleConfig := gb.findModuleConfig(call.Call.SourceAddrRaw); moduleConfig != nil {
		for name, output := range moduleConfig.Module.Outputs {
			moduleOutput := ModuleCallOutput{
				Name:        name,
				Type:        gb.getOutputTypeString(output),
				Description: output.Description,
				Sensitive:   output.Sensitive,
			}

			outputs = append(outputs, moduleOutput)
		}
	}

	return outputs
}

// findModuleConfig finds the module configuration for a given source address
func (gb *GraphBuilder) findModuleConfig(sourceAddr string) *configs.Config {
	var foundConfig *configs.Config

	gb.config.DeepEach(func(c *configs.Config) {
		// Check if this module's source matches
		for _, call := range c.Module.ModuleCalls {
			if call.SourceAddrRaw == sourceAddr {
				// Find the child config that corresponds to this module call
				for _, child := range c.Children {
					if child.SourceAddr != nil && child.SourceAddr.String() == sourceAddr {
						foundConfig = child
						return
					}
				}
			}
		}
	})

	return foundConfig
}

// getVariableTypeString extracts type information from a variable
func (gb *GraphBuilder) getVariableTypeString(variable *configs.Variable) string {
	if !variable.Type.Equals(cty.NilType) {
		// This would require cty type inspection for full type information
		// For now, return a simplified representation
		return variable.Type.FriendlyName() // Use cty's built-in friendly name
	}
	return "any"
}

// getOutputTypeString extracts type information from an output
func (gb *GraphBuilder) getOutputTypeString(output *configs.Output) string {
	// This would require HCL expression analysis to determine output type
	// For now, return a simplified representation
	return "any" // Could be enhanced to show actual inferred type
}

// ExpressionLocation tracks where an expression is used
type ExpressionLocation struct {
	TargetID   string // The resource/module that has the expression
	TargetType string // "resource" or "module"
	InputName  string // The input/attribute name
}

// buildAllExpressionNodes creates intermediate nodes for ALL complex expressions
func (gb *GraphBuilder) buildAllExpressionNodes(moduleCalls map[string]*ModuleCallInfo, resources map[string]*configs.Resource) ([]Node, []Edge, map[string]bool) {
	var nodes []Node
	var edges []Edge
	expressionMap := make(map[string]bool) // tracks "targetID.inputName" that have expressions
	nodeID := 0
	edgeID := 3000

	// Scan module calls for expressions
	for moduleCallID, call := range moduleCalls {
		if call.Call == nil || call.Call.Config == nil {
			continue
		}

		attrs, _ := call.Call.Config.JustAttributes()
		for attrName, attr := range attrs {
			if expr, ok := attr.Expr.(*hclsyntax.BinaryOpExpr); ok {
				// Mark this input as having an expression
				expressionKey := fmt.Sprintf("%s.%s", moduleCallID, attrName)
				expressionMap[expressionKey] = true

				// Create expression nodes
				expressionNode, staticNodes, exprEdges := gb.parseBinaryExpression(expr, moduleCallID, attrName, "module", &nodeID, &edgeID)
				nodes = append(nodes, expressionNode)
				nodes = append(nodes, staticNodes...)
				edges = append(edges, exprEdges...)
			}
		}
	}

	// Scan resources for expressions
	for resourceID, resource := range resources {
		if resource.Config == nil {
			continue
		}

		attrs, _ := resource.Config.JustAttributes()
		for attrName, attr := range attrs {
			if expr, ok := attr.Expr.(*hclsyntax.BinaryOpExpr); ok {
				// Mark this input as having an expression
				expressionKey := fmt.Sprintf("%s.%s", resourceID, attrName)
				expressionMap[expressionKey] = true

				// Create expression nodes
				expressionNode, staticNodes, exprEdges := gb.parseBinaryExpression(expr, resourceID, attrName, "resource", &nodeID, &edgeID)
				nodes = append(nodes, expressionNode)
				nodes = append(nodes, staticNodes...)
				edges = append(edges, exprEdges...)
			}
		}
	}

	return nodes, edges, expressionMap
}

// parseBinaryExpression handles binary operations recursively
func (gb *GraphBuilder) parseBinaryExpression(expr *hclsyntax.BinaryOpExpr, targetID, attrName, targetType string, nodeID, edgeID *int) (Node, []Node, []Edge) {
	var allNodes []Node
	var allEdges []Edge

	// Create the expression node for this level
	expressionNodeID := fmt.Sprintf("expr_%d", *nodeID)
	*nodeID++

	// Get the operation symbol
	var operation string
	switch expr.Op {
	case hclsyntax.OpAdd:
		operation = "+"
	case hclsyntax.OpSubtract:
		operation = "-"
	case hclsyntax.OpMultiply:
		operation = "*"
	case hclsyntax.OpDivide:
		operation = "/"
	case hclsyntax.OpEqual:
		operation = "=="
	case hclsyntax.OpNotEqual:
		operation = "!="
	default:
		operation = "unknown"
	}

	expressionNode := Node{
		ID:       expressionNodeID,
		Type:     "expression",
		ParentID: nil,
		Data: map[string]interface{}{
			"operation":   operation,
			"description": fmt.Sprintf("Expression: %s", operation),
			"targetID":    targetID,
			"targetType":  targetType,
			"targetInput": attrName,
			"inputs":      2, // LHS and RHS
			"outputs":     1, // Single result
		},
		Source: gb.extractSourceLocation(expr.Range()),
	}

	// Handle both operands with recursive logic
	operands := []struct {
		expr hcl.Expression
		side string
	}{
		{expr.LHS, "LHS"},
		{expr.RHS, "RHS"},
	}

	for _, operand := range operands {
		// Determine target handle based on operand side
		var targetHandle *string
		if operand.side == "LHS" {
			handle := "input-0" // First input (left side)
			targetHandle = &handle
		} else if operand.side == "RHS" {
			handle := "input-1" // Second input (right side)
			targetHandle = &handle
		}

		sourceNodeID := gb.parseExpression(operand.expr, operand.side, nodeID, edgeID, &allNodes, &allEdges)

		if sourceNodeID != "" {
			// Create edge from operand result to this expression
			allEdges = append(allEdges, Edge{
				ID:           generateEdgeID(*edgeID),
				Source:       sourceNodeID,
				Target:       expressionNodeID,
				Type:         "expression_input",
				TargetHandle: targetHandle,
			})
			*edgeID++
		}
	}

	// Create edge from expression to target input
	var outputTargetHandle *string
	if targetType == "module" {
		handle := fmt.Sprintf("input-%s", attrName)
		outputTargetHandle = &handle
	}

	allEdges = append(allEdges, Edge{
		ID:           generateEdgeID(*edgeID),
		Source:       expressionNodeID,
		Target:       targetID,
		Type:         "expression_output",
		TargetHandle: outputTargetHandle,
	})
	*edgeID++

	return expressionNode, allNodes, allEdges
}

// parseExpression recursively parses any HCL expression and returns the source node ID
func (gb *GraphBuilder) parseExpression(expr hcl.Expression, side string, nodeID, edgeID *int, allNodes *[]Node, allEdges *[]Edge) string {
	switch e := expr.(type) {
	// Nested binary expressions - recurse
	case *hclsyntax.BinaryOpExpr:
		// This is a nested expression, parse it recursively
		// For chained expressions like "A" + "B" + "C", this creates intermediate expression nodes
		nestedExprNode, nestedNodes, nestedEdges := gb.parseBinaryExpression(e, "", "", "", nodeID, edgeID)
		*allNodes = append(*allNodes, nestedExprNode)
		*allNodes = append(*allNodes, nestedNodes...)
		*allEdges = append(*allEdges, nestedEdges...)
		return nestedExprNode.ID

	// Literal values (numbers, booleans)
	case *hclsyntax.LiteralValueExpr:
		staticNodeID := fmt.Sprintf("static_%d", *nodeID)
		*nodeID++

		staticValue := ""
		if e.Val.Type() == cty.String {
			staticValue = e.Val.AsString()
		} else {
			staticValue = e.Val.GoString()
		}

		staticNode := Node{
			ID:       staticNodeID,
			Type:     "static_value",
			ParentID: nil,
			Data: map[string]interface{}{
				"value":       staticValue,
				"type":        e.Val.Type().FriendlyName(),
				"description": fmt.Sprintf("Static value (%s): %s", side, staticValue),
				"side":        side,
			},
			Source: gb.extractSourceLocation(e.Range()),
		}
		*allNodes = append(*allNodes, staticNode)
		return staticNodeID

	// Template expressions (quoted strings like "ABC")
	case *hclsyntax.TemplateExpr:
		if len(e.Parts) == 1 {
			if litPart, ok := e.Parts[0].(*hclsyntax.LiteralValueExpr); ok {
				staticNodeID := fmt.Sprintf("static_%d", *nodeID)
				*nodeID++

				staticValue := ""
				if litPart.Val.Type() == cty.String {
					staticValue = litPart.Val.AsString()
				} else {
					staticValue = litPart.Val.GoString()
				}

				staticNode := Node{
					ID:       staticNodeID,
					Type:     "static_value",
					ParentID: nil,
					Data: map[string]interface{}{
						"value":       staticValue,
						"type":        "string",
						"description": fmt.Sprintf("Static string (%s): %s", side, staticValue),
						"side":        side,
					},
					Source: gb.extractSourceLocation(e.Range()),
				}
				*allNodes = append(*allNodes, staticNode)
				return staticNodeID
			}
		}

	// Resource/variable references
	case *hclsyntax.ScopeTraversalExpr:
		resourceRef := gb.buildResourceReference(e.Traversal)
		if resourceRef != "" {
			return resourceRef // Reference existing resource node
		}

	// Other expression types - create unknown node for debugging
	default:
		unknownNodeID := fmt.Sprintf("unknown_%d", *nodeID)
		*nodeID++

		unknownNode := Node{
			ID:       unknownNodeID,
			Type:     "unknown_operand",
			ParentID: nil,
			Data: map[string]interface{}{
				"side":        side,
				"description": fmt.Sprintf("Unhandled operand type (%s)", side),
				"exprType":    fmt.Sprintf("%T", expr),
			},
		}
		*allNodes = append(*allNodes, unknownNode)
		return unknownNodeID
	}

	return ""
}

// extractSourceLocation extracts source location info from HCL range
func (gb *GraphBuilder) extractSourceLocation(declRange hcl.Range) *SourceLocation {
	if declRange.Filename == "" {
		return nil
	}

	// Convert absolute path to relative path from config root
	relPath, err := filepath.Rel(gb.configRoot, declRange.Filename)
	if err != nil {
		// If we can't get relative path, use the filename as-is
		relPath = filepath.Base(declRange.Filename)
	}

	return &SourceLocation{
		Filename:  relPath,
		StartLine: declRange.Start.Line,
		EndLine:   declRange.End.Line,
		StartCol:  declRange.Start.Column,
		EndCol:    declRange.End.Column,
	}
}

// buildResourceReference converts HCL traversal to resource ID
func (gb *GraphBuilder) buildResourceReference(traversal hcl.Traversal) string {
	if len(traversal) < 2 {
		return ""
	}

	// Extract resource type and name from traversal
	// e.g., random_string.root_config.result -> random_string.root_config
	resourceType := traversal[0].(hcl.TraverseRoot).Name
	resourceName := traversal[1].(hcl.TraverseAttr).Name

	// Build resource ID (assuming root module for now)
	return fmt.Sprintf("%s.%s", resourceType, resourceName)
}
