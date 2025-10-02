package configgraph

import (
	"sync/atomic"

	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

/*
When the scope is built, it assumes that everything in scope references everything else within scope
*scope -> module scope to be more precise
It increments a counter on each scope entry to match the potential references that could exist
The Value() function of each scope item includes a check of marks at the end.  It decrements the counters of each entry within scope that does not have a corresponding mark within the result value.
The rest of the marked values are left un-decremented until that scope item is Closed()/Freed() itself. (edited)
Therefore each scope item knows when it is first accessed (Value()'d), and when it is last referenced Closed()/Freed()) and can add any hooks it needs within there
I hope some of that made sense
I think it's analogous to "destructors" within a reference counting GC?





From the perspective of a local value:
A local exists and can be referenced within a module instance scope
We do not know how many things will reference it within the module instance scope
However, we know that there is a finite number of things within that scope that *could* reference it
Therefore, we coult those potential references before we create the local.  In practice, this is the number of elements within the module instance scope.
A local contains no special "close/free" logic and would have no custom handler.  () -> {}
Within the local's OnceValuer.Value() function call, once the Value() is known, it will:
  Determine if the value is marked by other items within the scope
  Notify the scope that any scope item that is *not* referenced by this local can decrement it's reference counter
  Ask the scope which mark referenced item it found exist within the scope
  Add/Update the close/free callback on the local:
  	for each of the items referenced by the local within the scope:
		decrement the reference counter for that item
			if that item's references go to zero, fire the close/free callback\
  Add the local's ReferenceTrackerMark to the result value



From the perspective of an input variable:
It is used within a module instance scope, but references things in both scopes, due to validation
Interstingingly, the value is "derived" from a single "ModuleCallInstance.Input" value.  I'm curious why this is not a scope instead.  Therefore we need to propogate the close/free through this layer.
Given that:
A variable exists and can be referenced within a module instance scope
We do not know how many things will reference it within the module instance scope
However, we know that there is a finite number of things within that scope that *could* reference it
Therefore, we coult those potential references before we create the variable.  In practice, this is the number of elements within the module instance scope.
A variable must propogate it's close/free callback through to the parent module scope via the ModuleCallInstance.Input mechanism.
Within the variable's OnceValuer.Value() function call, once the Value() is known, it will:
  Determine if the value is marked by other items within the scope
  Notify the scope that any scope item that is *not* referenced by this variable can decrement it's reference counter
  Ask the scope which mark referenced item it found exist within the scope
  Add/Update the close/free callback on the variable:
  	for each of the items referenced by the variable within the scope:
		decrement the reference counter for that item
			if that item's references go to zero, fire the close/free callback
  Add the variable's ReferenceTrackerMark to the result value


From the perspective of an output variable:
The scope's close dependes on the module call instance being closed, the value is prepared via configgraph.ModuleInstance.
Therefore, the output "close" is a simple link to the
Note: This constraint is violated in the testing framework.




Questions:
Could this be made easier with a different "Scope" abstraction?
Should this be it's own sidecar structure to start?
How does this work with my idea of target?








*/

// TODO builder pattern to lock this down once it's defined, or at least some locking
type ReferenceContainer struct {
	trackedMarks map[*referenceTrackerMark]struct{}
}

func NewReferenceContainer() *ReferenceContainer {
	return &ReferenceContainer{
		trackedMarks: map[*referenceTrackerMark]struct{}{},
	}
}

// TODO Could also return an "AddExternalReference" callback
func (r *ReferenceContainer) AddCounted(valuer exprs.Valuer, freeCallback func()) *OnceValuer {
	for containedMark := range r.trackedMarks {
		containedMark.activeReferences.Add(1)
	}

	var chained []*referenceTrackerMark
	mark := &referenceTrackerMark{
		activeReferences: &atomic.Int64{},
		free: func() {
			// We are no longer referenced and can be free'd
			if freeCallback != nil {
				freeCallback()
			}
			// We can now report this to any dependencies we discovered within the Value func below
			for _, chained := range chained {
				chained.Unref()
			}
		},
	}
	mark.activeReferences.Store(int64(len(r.trackedMarks)))

	r.trackedMarks[mark] = struct{}{}

	return ValuerOnce(exprs.DerivedValuer(valuer, func(v cty.Value, diags tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
		_, marks := v.UnmarkDeep()
		for containedMark := range r.trackedMarks {
			if _, ok := marks[containedMark]; ok {
				// This value was built using the containedMark and needs to chain it's closure
				chained = append(chained, containedMark)
			} else {
				// This value was not built using the containedMark and therefore does not need to chain it's closure
				containedMark.Unref()
			}

		}

		v = v.Mark(mark)
		// QUESTION: do we remove other referenceTrackerMarks from our result?

		return v, diags
	}))
}

type referenceTrackerMark struct {
	activeReferences *atomic.Int64
	free             func()
}

func (r *referenceTrackerMark) Unref() {
	refs := r.activeReferences.Add(-1)
	if refs > 0 {
		panic("ReferenceTracker is negative, this is a bug in OpenTofu's Engine")
	}
	if refs == 0 {
		r.free()
	}
}
