// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package moduletest

import (
	"maps"
	"slices"

	"github.com/opentofu/opentofu/internal/moduletest/junitxml"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (s *Suite) ToJUnit() *junitxml.JUnit {
	junit := junitxml.JUnit{}

	keys := slices.Collect(maps.Keys(s.Files))
	slices.Sort(keys)

	for _, k := range keys {
		f := s.Files[k]
		suite := junitxml.Testsuite{
			Name:      f.Name,
			File:      f.Name,
			Tests:     len(f.Runs),
			Timestamp: junitxml.RFC3339{Time: f.Started},
		}
		for _, r := range f.Runs {
			tc := junitxml.Testcase{
				Name:       r.Name,
				Assertions: len(r.Config.CheckRules) + len(r.Config.ExpectFailures),
				Time:       r.Duration.Seconds(),
				File:       f.Name,
				Line:       r.Config.NameDeclRange.Start.Line,
			}
			suite.Time += r.Duration.Seconds()
			suite.Assertions += len(r.Config.CheckRules) + len(r.Config.ExpectFailures)
			switch r.Status {
			case Fail:
				suite.Failures += 1
				tc.Line = getLineFromDiagnostics(r.Diagnostics)
				tc.Failure = &junitxml.Failure{
					Message: r.Diagnostics.Err().Error(),
				}
			case Error:
				suite.Errors += 1
				tc.Line = getLineFromDiagnostics(r.Diagnostics)
				tc.Error = &junitxml.JunitError{
					Message: r.Diagnostics.Err().Error(),
				}
			case Skip:
				suite.Skipped += 1
				tc.Skipped = &junitxml.Skipped{
					Message: "Skipped due to error or cancelation.",
				}
			}
			suite.Testcase = append(suite.Testcase, tc)
		}
		junit.Suites = append(junit.Suites, suite)
	}

	return &junit
}

func getLineFromDiagnostics(d tfdiags.Diagnostics) int {
	if len(d) >= 1 {
		source := d[len(d)-1].Source()
		if source.Subject != nil {
			return source.Subject.Start.Line
		}
	}
	return 0
}
