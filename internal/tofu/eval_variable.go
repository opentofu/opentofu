// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func prepareFinalInputVariableValue(addr addrs.AbsInputVariableInstance, raw *InputValue, cfg *configs.Variable) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	convertTy := cfg.ConstraintType
	log.Printf("[TRACE] prepareFinalInputVariableValue: preparing %s", addr)

	var defaultVal cty.Value
	if cfg.Default != cty.NilVal {
		log.Printf("[TRACE] prepareFinalInputVariableValue: %s has a default value", addr)
		var err error
		defaultVal, err = convert.Convert(cfg.Default, convertTy)
		if err != nil {
			// Validation of the declaration should typically catch this,
			// but we'll check it here too to be robust.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid default value for module argument",
				Detail: fmt.Sprintf(
					"The default value for variable %q is incompatible with its type constraint: %s.",
					cfg.Name, err,
				),
				Subject: &cfg.DeclRange,
			})
			// We'll return a placeholder unknown value to avoid producing
			// redundant downstream errors.
			return cty.UnknownVal(cfg.Type), diags
		}
	}

	var sourceRange tfdiags.SourceRange
	var nonFileSource string
	if raw.HasSourceRange() {
		sourceRange = raw.SourceRange
	} else {
		// If the value came from a place that isn't a file and thus doesn't
		// have its own source range, we'll use the declaration range as
		// our source range and generate some slightly different error
		// messages.
		sourceRange = tfdiags.SourceRangeFromHCL(cfg.DeclRange)
		switch raw.SourceType {
		case ValueFromCLIArg:
			nonFileSource = fmt.Sprintf("set using -var=\"%s=...\"", addr.Variable.Name)
		case ValueFromEnvVar:
			nonFileSource = fmt.Sprintf("set using the TF_VAR_%s environment variable", addr.Variable.Name)
		case ValueFromInput:
			nonFileSource = "set using an interactive prompt"
		default:
			nonFileSource = "set from outside of the configuration"
		}
	}

	given := raw.Value
	if given == cty.NilVal { // The variable wasn't set at all (even to null)
		log.Printf("[TRACE] prepareFinalInputVariableValue: %s has no defined value", addr)
		if cfg.Required() {
			// NOTE: The CLI layer typically checks for itself whether all of
			// the required _root_ module variables are set, which would
			// mask this error with a more specific one that refers to the
			// CLI features for setting such variables. We can get here for
			// child module variables, though.
			log.Printf("[ERROR] prepareFinalInputVariableValue: %s is required but is not set", addr)
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Required variable not set`,
				Detail:   fmt.Sprintf(`The variable %q is required, but is not set.`, addr.Variable.Name),
				Subject:  cfg.DeclRange.Ptr(),
			})
			// We'll return a placeholder unknown value to avoid producing
			// redundant downstream errors.
			return cty.UnknownVal(cfg.Type), diags
		}

		given = defaultVal // must be set, because we checked above that the variable isn't required
	}

	// Apply defaults from the variable's type constraint to the converted value,
	// unless the converted value is null. We do not apply defaults to top-level
	// null values, as doing so could prevent assigning null to a nullable
	// variable.
	if cfg.TypeDefaults != nil && !given.IsNull() {
		given = cfg.TypeDefaults.Apply(given)
	}

	val, err := convert.Convert(given, convertTy)
	if err != nil {
		log.Printf("[ERROR] prepareFinalInputVariableValue: %s has unsuitable type\n  got:  %s\n  want: %s", addr, given.Type(), convertTy)
		var detail string
		var subject *hcl.Range
		if nonFileSource != "" {
			detail = fmt.Sprintf(
				"Unsuitable value for %s %s: %s.",
				addr, nonFileSource, err,
			)
			subject = cfg.DeclRange.Ptr()
		} else {
			detail = fmt.Sprintf(
				"The given value is not suitable for %s declared at %s: %s.",
				addr, cfg.DeclRange.String(), err,
			)
			subject = sourceRange.ToHCL().Ptr()

			// In some workflows, the operator running tofu does not have access to the variables
			// themselves. They are for example stored in encrypted files that will be used by the CI toolset
			// and not by the operator directly. In such a case, the failing secret value should not be
			// displayed to the operator
			if cfg.Sensitive {
				detail = fmt.Sprintf(
					"The given value is not suitable for %s, which is sensitive: %s. Invalid value defined at %s.",
					addr, err, sourceRange.ToHCL(),
				)
				subject = cfg.DeclRange.Ptr()
			}
		}

		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid value for input variable",
			Detail:   detail,
			Subject:  subject,
		})
		// We'll return a placeholder unknown value to avoid producing
		// redundant downstream errors.
		return cty.UnknownVal(cfg.Type), diags
	}

	// By the time we get here, we know:
	// - val matches the variable's type constraint
	// - val is definitely not cty.NilVal, but might be a null value if the given was already null.
	//
	// That means we just need to handle the case where the value is null,
	// which might mean we need to use the default value, or produce an error.
	//
	// For historical reasons we do this only for a "non-nullable" variable.
	// Nullable variables just appear as null if they were set to null,
	// regardless of any default value.
	if val.IsNull() && !cfg.Nullable {
		log.Printf("[TRACE] prepareFinalInputVariableValue: %s is defined as null", addr)
		if defaultVal != cty.NilVal {
			val = defaultVal
		} else {
			log.Printf("[ERROR] prepareFinalInputVariableValue: %s is non-nullable but set to null, and is required", addr)
			if nonFileSource != "" {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  `Required variable not set`,
					Detail: fmt.Sprintf(
						"Unsuitable value for %s %s: required variable may not be set to null.",
						addr, nonFileSource,
					),
					Subject: cfg.DeclRange.Ptr(),
				})
			} else {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  `Required variable not set`,
					Detail: fmt.Sprintf(
						"The given value is not suitable for %s defined at %s: required variable may not be set to null.",
						addr, cfg.DeclRange.String(),
					),
					Subject: sourceRange.ToHCL().Ptr(),
				})
			}
			// Stub out our return value so that the semantic checker doesn't
			// produce redundant downstream errors.
			val = cty.UnknownVal(cfg.Type)
		}
	}

	return val, diags
}

// evalVariableValidations ensures that all of the configured custom validations
// for a variable are passing.
//
// This must be used only after any side-effects that make the value of the
// variable available for use in expression evaluation, such as
// EvalModuleCallArgument for variables in descendent modules.
func evalVariableValidations(addr addrs.AbsInputVariableInstance, config *configs.Variable, expr hcl.Expression, ctx EvalContext) (diags tfdiags.Diagnostics) {
	if config == nil || len(config.Validations) == 0 {
		log.Printf("[TRACE] evalVariableValidations: no validation rules declared for %s, so skipping", addr)
		return nil
	}
	log.Printf("[TRACE] evalVariableValidations: validating %s", addr)

	checkState := ctx.Checks()
	if !checkState.ConfigHasChecks(addr.ConfigCheckable()) {
		// We have nothing to do if this object doesn't have any checks,
		// but the "rules" slice should agree that we don't.
		if ct := len(config.Validations); ct != 0 {
			panic(fmt.Sprintf("check state says that %s should have no rules, but it has %d", addr, ct))
		}
		return diags
	}

	// Variable nodes evaluate in the parent module to where they were declared
	// because the value expression (n.Expr, if set) comes from the calling
	// "module" block in the parent module.
	//
	// Validation expressions are statically validated (during configuration
	// loading) to refer only to the variable being validated, so we can
	// bypass our usual evaluation machinery here and just produce a minimal
	// evaluation context containing just the required value, and thus avoid
	// the problem that ctx's evaluation functions refer to the wrong module.
	val := ctx.GetVariableValue(addr)
	if val == cty.NilVal {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "No final value for variable",
			Detail:   fmt.Sprintf("OpenTofu doesn't have a final value for %s during validation. This is a bug in OpenTofu; please report it!", addr),
		})
		return diags
	}
	if config.Sensitive {
		// Normally this marking is handled automatically during construction
		// of the HCL eval context the evaluation scope, but this codepath
		// augments that scope with the not-yet-finalized input variable
		// value, so we need to apply this mark separately.
		val = val.Mark(marks.Sensitive)
	}
	for ix, validation := range config.Validations {
		condRefs, condDiags := lang.ReferencesInExpr(addrs.ParseRef, validation.Condition)
		diags = diags.Append(condDiags)
		errRefs, errDiags := lang.ReferencesInExpr(addrs.ParseRef, validation.ErrorMessage)
		diags = diags.Append(errDiags)

		if diags.HasErrors() {
			continue
		}

		hclCtx, ctxDiags := ctx.WithPath(addr.Module).EvaluationScope(nil, nil, EvalDataForNoInstanceKey).EvalContext(append(condRefs, errRefs...))
		diags = diags.Append(ctxDiags)
		if diags.HasErrors() {
			continue
		}

		// Inject known value into hclCtx as it's not technically available until the check status has been reported
		if vars, ok := hclCtx.Variables["var"]; ok {
			// Clone map
			varMap := vars.AsValueMap() // explicit copy
			varMap[config.Name] = val
			hclCtx.Variables["var"] = cty.ObjectVal(varMap)
		} else {
			// new map
			hclCtx.Variables["var"] = cty.ObjectVal(map[string]cty.Value{
				config.Name: val,
			})
		}

		result, ruleDiags := evalVariableValidation(validation, hclCtx, addr, config, expr, ix)
		diags = diags.Append(ruleDiags)

		log.Printf("[TRACE] evalVariableValidations: %s status is now %s", addr, result.Status)
		if result.Status == checks.StatusFail {
			checkState.ReportCheckFailure(addr, addrs.InputValidation, ix, result.FailureMessage)
		} else {
			checkState.ReportCheckResult(addr, addrs.InputValidation, ix, result.Status)
		}
	}

	return diags
}

func evalVariableValidation(validation *configs.CheckRule, hclCtx *hcl.EvalContext, addr addrs.AbsInputVariableInstance, config *configs.Variable, expr hcl.Expression, ix int) (checkResult, tfdiags.Diagnostics) {
	const errInvalidCondition = "Invalid variable validation result"
	const errInvalidValue = "Invalid value for variable"
	var diags tfdiags.Diagnostics

	result, moreDiags := validation.Condition.Value(hclCtx)
	diags = diags.Append(moreDiags)
	errorValue, errorDiags := validation.ErrorMessage.Value(hclCtx)

	// The following error handling is a workaround to preserve backwards
	// compatibility. Due to an implementation quirk, all prior versions of
	// Terraform would treat error messages specified using JSON
	// configuration syntax (.tf.json) as string literals, even if they
	// contained the "${" template expression operator. This behaviour did
	// not match that of HCL configuration syntax, where a template
	// expression would result in a validation error.
	//
	// As a result, users writing or generating JSON configuration syntax
	// may have specified error messages which are invalid template
	// expressions. As we add support for error message expressions, we are
	// unable to perfectly distinguish between these two cases.
	//
	// To ensure that we don't break backwards compatibility, we have the
	// below fallback logic if the error message fails to evaluate. This
	// should only have any effect for JSON configurations. The gohcl
	// DecodeExpression function behaves differently when the source of the
	// expression is a JSON configuration file and a nil context is passed.
	if errorDiags.HasErrors() {
		// Attempt to decode the expression as a string literal. Passing
		// nil as the context forces a JSON syntax string value to be
		// interpreted as a string literal.
		var errorString string
		moreErrorDiags := gohcl.DecodeExpression(validation.ErrorMessage, nil, &errorString)
		if !moreErrorDiags.HasErrors() {
			// Decoding succeeded, meaning that this is a JSON syntax
			// string value. We rewrap that as a cty value to allow later
			// decoding to succeed.
			errorValue = cty.StringVal(errorString)

			// This warning diagnostic explains this odd behaviour, while
			// giving us an escape hatch to change this to a hard failure
			// in some future OpenTofu 1.x version.
			errorDiags = hcl.Diagnostics{
				&hcl.Diagnostic{
					Severity:    hcl.DiagWarning,
					Summary:     "Validation error message expression is invalid",
					Detail:      fmt.Sprintf("The error message provided could not be evaluated as an expression, so OpenTofu is interpreting it as a string literal.\n\nIn future versions of OpenTofu, this will be considered an error. Please file a GitHub issue if this would break your workflow.\n\n%s", errorDiags.Error()),
					Subject:     validation.ErrorMessage.Range().Ptr(),
					Context:     validation.DeclRange.Ptr(),
					Expression:  validation.ErrorMessage,
					EvalContext: hclCtx,
				},
			}
		}

		// We want to either report the original diagnostics if the
		// fallback failed, or the warning generated above if it succeeded.
		diags = diags.Append(errorDiags)
	}

	if diags.HasErrors() {
		log.Printf("[TRACE] evalVariableValidations: %s rule %s check rule evaluation failed: %s", addr, validation.DeclRange, diags.Err().Error())
	}
	if !result.IsKnown() {
		log.Printf("[TRACE] evalVariableValidations: %s rule %s condition value is unknown, so skipping validation for now", addr, validation.DeclRange)

		return checkResult{Status: checks.StatusUnknown}, diags // We'll wait until we've learned more, then.
	}
	if result.IsNull() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     errInvalidCondition,
			Detail:      "Validation condition expression must return either true or false, not null.",
			Subject:     validation.Condition.Range().Ptr(),
			Expression:  validation.Condition,
			EvalContext: hclCtx,
		})
		return checkResult{Status: checks.StatusError}, diags
	}
	var err error
	result, err = convert.Convert(result, cty.Bool)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     errInvalidCondition,
			Detail:      fmt.Sprintf("Invalid validation condition result value: %s.", tfdiags.FormatError(err)),
			Subject:     validation.Condition.Range().Ptr(),
			Expression:  validation.Condition,
			EvalContext: hclCtx,
		})
		return checkResult{Status: checks.StatusError}, diags
	}

	// Validation condition may be marked if the input variable is bound to
	// a sensitive value. This is irrelevant to the validation process, so
	// we discard the marks now.
	result, _ = result.Unmark()
	status := checks.StatusForCtyValue(result)

	if status != checks.StatusFail {
		return checkResult{Status: status}, diags
	}

	var errorMessage string
	if !errorDiags.HasErrors() && !errorValue.IsKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid error message",
			Detail:      "Value used in error message is not known and can not be used in error_message until after apply.",
			Subject:     validation.ErrorMessage.Range().Ptr(),
			Expression:  validation.ErrorMessage,
			EvalContext: hclCtx,
			Extra:       evalchecks.DiagnosticCausedByUnknown(true),
		})
		// Return early
		return checkResult{
			Status:         status,
			FailureMessage: errorMessage,
		}, diags
	}
	if !errorDiags.HasErrors() && errorValue.IsKnown() && !errorValue.IsNull() {
		var err error
		errorValue, err = convert.Convert(errorValue, cty.String)
		if err != nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Invalid error message",
				Detail:      fmt.Sprintf("Unsuitable value for error message: %s.", tfdiags.FormatError(err)),
				Subject:     validation.ErrorMessage.Range().Ptr(),
				Expression:  validation.ErrorMessage,
				EvalContext: hclCtx,
			})
		} else {
			if marks.Has(errorValue, marks.Sensitive) {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,

					Summary: "Error message refers to sensitive values",
					Detail: `The error expression used to explain this condition refers to sensitive values. OpenTofu will not display the resulting message.

You can correct this by removing references to sensitive values, or by carefully using the nonsensitive() function if the expression will not reveal the sensitive data.`,

					Subject:     validation.ErrorMessage.Range().Ptr(),
					Expression:  validation.ErrorMessage,
					EvalContext: hclCtx,
					Extra:       evalchecks.DiagnosticCausedBySensitive(true),
				})
				errorMessage = "The error message included a sensitive value, so it will not be displayed."
			} else {
				errorMessage = strings.TrimSpace(errorValue.AsString())
			}
		}
	}
	if errorMessage == "" {
		errorMessage = "Failed to evaluate condition error message."
	}

	if expr != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     errInvalidValue,
			Detail:      fmt.Sprintf("%s\n\nThis was checked by the validation rule at %s.", errorMessage, validation.DeclRange.String()),
			Subject:     expr.Range().Ptr(),
			Expression:  validation.Condition,
			EvalContext: hclCtx,
			Extra: &addrs.CheckRuleDiagnosticExtra{
				CheckRule: addr.CheckRule(addrs.InputValidation, ix),
			},
		})
	} else {
		// Since we don't have a source expression for a root module
		// variable, we'll just report the error from the perspective
		// of the variable declaration itself.
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     errInvalidValue,
			Detail:      fmt.Sprintf("%s\n\nThis was checked by the validation rule at %s.", errorMessage, validation.DeclRange.String()),
			Subject:     config.DeclRange.Ptr(),
			Expression:  validation.Condition,
			EvalContext: hclCtx,
			Extra: &addrs.CheckRuleDiagnosticExtra{
				CheckRule: addr.CheckRule(addrs.InputValidation, ix),
			},
		})
	}

	return checkResult{
		Status:         status,
		FailureMessage: errorMessage,
	}, diags
}

// evalVariableDeprecation checks if a variable is deprecated and if so it returns a warning diagnostic to be shown to the user
func evalVariableDeprecation(addr addrs.AbsInputVariableInstance, config *configs.Variable, expr hcl.Expression, ctx EvalContext) tfdiags.Diagnostics {
	if config.Deprecated == "" {
		log.Printf("[TRACE] evalVariableDeprecation: variable %s does not have deprecation configured", addr)
		return nil
	}
	// if the variable is not given in the module call, do not show a warning
	if expr == nil {
		log.Printf("[TRACE] evalVariableDeprecation: variable %s is marked as deprecated but is not used", addr)
		return nil
	}
	val := ctx.GetVariableValue(addr)
	if val == cty.NilVal {
		log.Printf("[TRACE] evalVariableDeprecation: variable %s is marked as deprecated but no value given", addr)
		return nil
	}
	log.Printf("[TRACE] evalVariableDeprecation: usage of deprecated variable %q detected", addr)
	var diags tfdiags.Diagnostics
	return diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  fmt.Sprintf(`The variable %q is marked as deprecated by module author`, config.Name),
		Detail:   fmt.Sprintf("This variable is marked as deprecated with the following message:\n%s", config.Deprecated),
		Subject:  expr.Range().Ptr(),
	})
}
