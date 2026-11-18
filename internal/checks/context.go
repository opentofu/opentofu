// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package checks

import (
	"context"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
)

type ReportedChecks struct {
	sync.Mutex
	entries []reportedCheck
}

type reportedCheck struct {
	addr           addrs.Checkable
	ruleType       addrs.CheckRuleType
	status         Status
	failureMessage string
}

func (r *ReportedChecks) State() *State {
	r.Lock()
	defer r.Unlock()

	state := &State{
		statuses:    addrs.MakeMap[addrs.ConfigCheckable, *configCheckableState](),
		failureMsgs: addrs.MakeMap[addrs.CheckRule, string](),
	}

	for _, entry := range r.entries {
		addrConfig := entry.addr.ConfigCheckable()

		stat, ok := state.statuses.GetOk(addrConfig)
		if !ok {
			stat = &configCheckableState{
				// We intentially don't use checkTypes here as it's an internal artefact of the
				// older reporting subsystem
				// We could try to re-construct that data, but I don't see where it's currently useful.
				objects: addrs.MakeMap[addrs.Checkable, map[addrs.CheckRuleType][]Status](),
			}
			state.statuses.Put(addrConfig, stat)
		}

		// TODO handle wildcards in entry.addr for deferral

		obj, ok := stat.objects.GetOk(entry.addr)
		if !ok {
			obj = map[addrs.CheckRuleType][]Status{}
			stat.objects.Put(entry.addr, obj)
		}

		// NOTE: This assumes that for a given addrs.Checkable, we always iterate through the
		// check rules in the same order.  Given that it's just HCL evaluation, there's no
		// strong reason to do this in parallle. It *could* have some very minor edge case
		// improvements during heavy targeting, but it's not worth optimizing at this point.

		obj[entry.ruleType] = append(obj[entry.ruleType], entry.status)

		if entry.failureMessage != "" {
			index := len(obj[entry.ruleType])
			checkAddr := addrs.NewCheckRule(entry.addr, entry.ruleType, index)
			state.failureMsgs.Put(checkAddr, entry.failureMessage)
		}
	}

	return state
}

type contextKey rune

const reportContextKey = contextKey('R')

func ContextWithCheckReporting(parent context.Context, report *ReportedChecks) context.Context {
	return context.WithValue(parent, reportContextKey, report)
}
func ReportCheckInContext(
	ctx context.Context,
	addr addrs.Checkable,
	ruleType addrs.CheckRuleType,
	status Status,
	failureMessage string,
) {
	checks, ok := ctx.Value(reportContextKey).(*ReportedChecks)
	if !ok {
		return
	}
	checks.Lock()
	defer checks.Unlock()
	checks.entries = append(checks.entries, reportedCheck{
		addr:           addr,
		ruleType:       ruleType,
		status:         status,
		failureMessage: failureMessage,
	})
}
