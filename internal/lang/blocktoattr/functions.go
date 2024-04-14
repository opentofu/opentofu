package blocktoattr

/*
func ExpandedFunctions(body hcl.Body, schema *configschema.Block) []string {
	rootNode := dynblock.WalkVariables(body)
	return walkFunctions(rootNode, body, schema)
}

func walkFunctions(node dynblock.WalkVariablesNode, body hcl.Body, schema *configschema.Block) []string {
	givenRawSchema := hcldec.ImpliedSchema(schema.DecoderSpec())
	ambiguousNames := ambiguousNames(schema)
	effectiveRawSchema := effectiveSchema(givenRawSchema, body, ambiguousNames, false)
	vars, children := node.Visit(effectiveRawSchema)

	for _, child := range children {
		if blockS, exists := schema.BlockTypes[child.BlockTypeName]; exists {
			vars = append(vars, walkFunctions(child.Node, child.Body(), &blockS.Block)...)
		} else if attrS, exists := schema.Attributes[child.BlockTypeName]; exists && attrS.Type.IsCollectionType() && attrS.Type.ElementType().IsObjectType() {
			// ☝️Check for collection type before element type, because if this is a mis-placed reference,
			// a panic here will prevent other useful diags from being elevated to show the user what to fix
			synthSchema := SchemaForCtyElementType(attrS.Type.ElementType())
			vars = append(vars, walkFunctions(child.Node, child.Body(), synthSchema)...)
		}
	}

	return vars
}*/
