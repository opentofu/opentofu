package funcs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

var RegoFunc = function.New(&function.Spec{
	Description: `rego is a function that allows you to use Rego policies directly in OpenTF. It takes a Rego policy as a string and returns a map of the results.`,
	Params: []function.Parameter{
		{
			Name: "query",
			Type: cty.String,
		},
		{
			Name: "policy",
			Type: cty.String,
		},
		{
			Name: "input",
			Type: cty.DynamicPseudoType,
		},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		query, policy, input := args[0], args[1], args[2]

		// The only way we can know the return type is by running the evaluation,
		// and that requires all arguments to be known.
		if !query.IsWhollyKnown() || !policy.IsWhollyKnown() || !input.IsWhollyKnown() {
			return cty.DynamicPseudoType, nil
		}

		// If we want to know the type of the result, we need to actually
		// evaluate the Rego query. There's no clever inference we can do here.
		data, err := perform(query.AsString(), policy.AsString(), input)
		if err != nil {
			return cty.DynamicPseudoType, err
		}

		return ctyjson.ImpliedType(data)
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		query, policy, input := args[0], args[1], args[2]

		if !query.IsWhollyKnown() || !policy.IsWhollyKnown() || !input.IsWhollyKnown() {
			return cty.UnknownVal(retType).WithSameMarks(query, policy, input), nil
		}

		data, err := perform(query.AsString(), policy.AsString(), input)
		if err != nil {
			return cty.NilVal, err
		}

		out, err := ctyjson.Unmarshal(data, retType)
		if err != nil {
			return cty.NilVal, err
		}

		// Copy the inputs marks to the output.
		return out.WithSameMarks(query, policy, input), nil
	},
})

func perform(query, policy string, input cty.Value) ([]byte, error) {
	// In order to feed the cty entity into the Rego engine in a way that's
	// recognizable to the user, we need to turn it into a very simple JSON
	// object.
	jsonObj, err := ctyjson.Marshal(input, input.Type())
	if err != nil {
		return nil, fmt.Errorf("could not marshal Terraform input: %w", err)
	}

	var jsonInput any
	if err := json.Unmarshal(jsonObj, &jsonInput); err != nil {
		return nil, fmt.Errorf("could not unmarshal input: %w", err)
	}

	// Allow 5 seconds for the Rego query to run.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	prepared, err := rego.New(
		rego.Query(query),
		rego.Module("policy", policy),
		// Support the "print" function in Rego.
		// This is useful for debugging - the output will be printed to the
		// Terraform log with "INFO" severity.
		rego.EnablePrintStatements(true),
		rego.PrintHook(topdown.NewPrintHook(log.Writer())),
	).PrepareForEval(ctx)

	if err != nil {
		return nil, fmt.Errorf("could not prepare Rego query: %w", err)
	}

	resultSet, err := prepared.Eval(ctx, rego.EvalInput(jsonInput))
	if err != nil {
		return nil, fmt.Errorf("could not evaluate Rego query: %w", err)
	}

	// If the Rego query produced no result, then we can return a proper null
	// JSON value to the caller.
	if len(resultSet) < 1 {
		return []byte("null"), nil
	}

	// TODO: Is it actually possible? If so, how? Find out with the Styra folks.
	if len(resultSet[0].Expressions) != 1 {
		return nil, fmt.Errorf("expected exactly one expression from Rego query, got %d", len(resultSet[0].Expressions))
	}

	return json.Marshal(resultSet[0].Expressions[0].Value)
}
