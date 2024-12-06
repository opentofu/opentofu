# A Pragmatic Approach to Linting for Code Complexity

We currently have a set of aspirational linting rules in the project's `golangci-lint` configuration, but this codebase was derived from a much older codebase that was not written under those lint rules and so we made the pragmatic decision that only code that has changed since the addition of the lint rules is subjected to those lint rules.

That approach aims to make the compromise of encouraging us to gradually improve code "while we're in the area" working on other changes, while avoiding the need for a huge retrofit of existing code.

However, that compromise seems to be less appropriate for the subset of linting rules related to code complexity in particular. That category of rules typically imposes some arbitrary limit on a qualitative metric that the linting tool can measure, such as number of lines or statements in a function, the [cyclomatic complexity](https://en.wikipedia.org/wiki/Cyclomatic_complexity) of a function, or the number of nested `if` statements. These particular rules therefore have a relatively broad scope and tend to require very disruptive changes to existing code in order to resolve them.

Making large changes to the organization of code as part of a pull request focused on a bug report or small feature tends to be inappropriate because it increases the risk of merging the change, and makes the diff harder to review. It _is_ reasonable to make small, localized changes for localized lint rules about style or likely bugs, but for complexity-related concerns we have instead tended to just add `nolint` comments to the codebase instead, which then makes those problems harder to find an fix in future.

## Proposed Solution

The aim of this proposal is to try to find a pragmatic path that will lead to a codebase that _does_ conform to the complexity lint rules in the long run, but to treat those improvements as a separate project in their own right rather than as something we aim to gradually improve as part of other work.

That refactoring work ought to be done in small stages so that we don't get review fatigue from reviewing large diffs that mostly just move existing code around, and so this proposal also asks for temporarily weakening the linting rules in the meantime so that other work can proceed independently from the refactoring work.

The overall goal is to take a preactive approach to fixing this particular subset of lint failures, concurrently with but separately from forthcoming maintenence and project work, so that we can keep this somewhat-risky refactoring work (which should not change OpenTofu's observable behavior _at all_) clearly separated from pull requests that are intentionally changing OpenTofu's observable behavior.

### Temporarily Disable the Complexity-related Linters

In order to make the work in this RFC as independent as possible from other projects, and thus to reduce risk to the schedule of those other projects, we can temporarily disable the four linters that the golangci project classifies as being in the "complexity" category:

- `cyclop`
- `funlen`
- `gocognit`
- `gocyclo`
- `nestif`

We would disable these by adding YAML comments to the relevant lines of the `.golangci.yml` file, to represent that this should be only a temporary state and to make them easy to re-enable with the same settings once the work described in this RFC is complete.

We would also remove any `nolint` directives for these four linters from the codebase at the same time, to ensure that our later work to improve the codebase will not miss any functions we have previously made exceptions for.

The temporary disabling of these lint rules is not intended as license to write new code that would violate these rules, but recognizes that a lot of our work involves localized changes to existing functions and that we should not necessarily force that work to take on the cost and risk of significant code structure changes.

All other linters would remain enabled throughout this project, and for problems detected by those we would continue with our existing practice of fixing them reactively as part of other projects, as we learn of the problems while working on other fixes.

### Proactively Improve Existing Code

At the time of writing, there are 160 lines of code failing at least one of the five complexity-related linters[^1]:

[^1]: I obtained this report by making a temporary copy of `.golangci.yml` and changing the set of enabled linters to only the five listed in the previous section, and also temporarily removing all of the existing `nolint` comments added to defer fixing these problems in earlier work.

    Note that golangci is reporting at most one problem for each distinct line of code, and that some of our functions actually fail more than one linter today.

```
internal/addrs/map_test.go:12:1                                      cyclop    calculated cyclomatic complexity for function TestMap is 21, max is 20
internal/backend/local/backend_apply.go:48                           funlen    Function 'opApply' has too many statements (108 > 50)
internal/backend/local/backend_apply.go:114:2                        nestif    `if op.PlanFile == nil` has complex nested blocks (complexity: 27)
internal/backend/local/backend_local.go:44                           funlen    Function 'localRun' has too many statements (55 > 50)
internal/backend/local/backend_local.go:236                          funlen    Function 'localRunForPlanFile' is too long (122 > 100)
internal/backend/local/backend_plan.go:25                            funlen    Function 'opPlan' has too many statements (81 > 50)
internal/backend/local/hook_state.go:78:2                            nestif    `if h.StateMgr != nil` has complex nested blocks (complexity: 8)
internal/backend/local/hook_state_test.go:40:1                       cyclop    calculated cyclomatic complexity for function TestStateHookStopping is 22, max is 20
internal/backend/remote-state/azure/backend.go:18                    funlen    Function 'New' is too long (165 > 100)
internal/backend/remote-state/azure/backend_state.go:109:2           nestif    `if v == nil` has complex nested blocks (complexity: 10)
internal/backend/remote-state/cos/backend.go:59                      funlen    Function 'New' is too long (131 > 100)
internal/backend/remote-state/cos/backend_state.go:103:2             nestif    `if !exists` has complex nested blocks (complexity: 9)
internal/backend/remote-state/http/backend_test.go:23:1              cyclop    calculated cyclomatic complexity for function TestHTTPClientFactory is 27, max is 20
internal/backend/remote-state/kubernetes/backend.go:40               funlen    Function 'New' is too long (142 > 100)
internal/backend/remote-state/kubernetes/backend.go:306              funlen    Function 'tryLoadingConfigFile' has too many statements (51 > 50)
internal/backend/remote-state/kubernetes/backend_state.go:95:2       nestif    `if v == nil` has complex nested blocks (complexity: 6)
internal/backend/remote-state/oss/backend.go:551:1                   cyclop    calculated cyclomatic complexity for function getConfigFromProfile is 23, max is 20
internal/backend/remote-state/pg/backend_test.go:233:1               gocognit  cognitive complexity 51 of func `TestBackendConfigSkipOptions` is high (> 50)
internal/backend/remote-state/s3/backend.go:61                       funlen    Function 'ConfigSchema' is too long (384 > 100)
internal/backend/remote-state/s3/backend.go:464                      funlen    Function 'PrepareConfig' is too long (167 > 100)
internal/backend/remote-state/s3/backend.go:644                      funlen    Function 'Configure' has too many statements (63 > 50)
internal/backend/remote-state/s3/backend.go:676:2                    nestif    `if ok` has complex nested blocks (complexity: 9)
internal/backend/remote-state/s3/backend_complete_test.go:150:1      gocognit  cognitive complexity 63 of func `TestBackendConfig_Authentication` is high (> 50)
internal/backend/remote-state/s3/backend_state.go:182:2              nestif    `if !exists` has complex nested blocks (complexity: 9)
internal/backend/remote-state/s3/backend_test.go:1163:4              nestif    `if testCase.expectedErr != ""` has complex nested blocks (complexity: 7)
internal/backend/remote-state/s3/backend_test.go:1227:4              nestif    `if testCase.expectedErr != ""` has complex nested blocks (complexity: 7)
internal/backend/remote-state/s3/backend_test.go:1255:1              cyclop    calculated cyclomatic complexity for function TestBackendExtraPaths is 24, max is 20
internal/backend/remote/backend_common.go:466:1                      gocognit  cognitive complexity 83 of func `(*Remote).confirm` is high (> 50)
internal/checks/state_test.go:20:1                                   cyclop    calculated cyclomatic complexity for function TestChecksHappyPath is 26, max is 20
internal/cloud/backend.go:262                                        funlen    Function 'Configure' has too many statements (53 > 50)
internal/cloud/backend.go:646                                        funlen    Function 'StateMgr' has too many statements (52 > 50)
internal/cloud/backend.go:692:2                                      nestif    `if err == tfe.ErrResourceNotFound` has complex nested blocks (complexity: 11)
internal/cloud/backend.go:780                                        funlen    Function 'Operation' has too many statements (61 > 50)
internal/cloud/backend.go:905:2                                      nestif    `if r.Actions.IsCancelable` has complex nested blocks (complexity: 13)
internal/cloud/backend.go:1011:2                                     nestif    `if remoteVersion != nil && remoteVersion.Prerelease() == ""` has complex nested blocks (complexity: 9)
internal/cloud/backend_apply.go:25                                   funlen    Function 'opApply' has too many statements (53 > 50)
internal/cloud/backend_apply.go:97:2                                 nestif    `if ok` has complex nested blocks (complexity: 20)
internal/cloud/backend_apply.go:233:2                                nestif    `if b.CLI != nil` has complex nested blocks (complexity: 12)
internal/cloud/backend_apply_test.go:1948:4                          nestif    `if tc.wantErr != ""` has complex nested blocks (complexity: 14)
internal/cloud/backend_common.go:50                                  funlen    Function 'waitForRun' is too long (144 > 100)
internal/cloud/backend_common.go:83:3                                nestif    `if b.CLI != nil && (i == 0 || current.Sub(updated).Seconds() > 30)` has complex nested blocks (complexity: 29)
internal/cloud/backend_common.go:227                                 funlen    Function 'costEstimate' has too many statements (57 > 50)
internal/cloud/backend_common.go:322                                 funlen    Function 'checkPolicy' has too many statements (57 > 50)
internal/cloud/backend_common.go:403:4                               nestif    `if op.AutoApprove` has complex nested blocks (complexity: 8)
internal/cloud/backend_common.go:442:1                               gocognit  cognitive complexity 84 of func `(*Cloud).confirm` is high (> 50)
internal/cloud/backend_common.go:524:3                               nestif    `if v != keyword` has complex nested blocks (complexity: 8)
internal/cloud/backend_context.go:26                                 funlen    Function 'LocalRun' is too long (114 > 100)
internal/cloud/backend_context.go:92:2                               nestif    `if op.AllowUnsetVariables` has complex nested blocks (complexity: 22)
internal/cloud/backend_context_test.go:218:4                         nestif    `if test.WantError != ""` has complex nested blocks (complexity: 6)
internal/cloud/backend_plan.go:120                                   funlen    Function 'plan' has too many statements (100 > 50)
internal/cloud/backend_plan.go:145:2                                 nestif    `if op.ConfigDir != ""` has complex nested blocks (complexity: 7)
internal/cloud/backend_plan.go:298:2                                 nestif    `if lockTimeout > 0` has complex nested blocks (complexity: 6)
internal/cloud/backend_plan.go:438                                   funlen    Function 'renderPlanLogs' has too many statements (51 > 50)
internal/cloud/backend_plan.go:444:2                                 nestif    `if b.CLI != nil` has complex nested blocks (complexity: 13)
internal/cloud/backend_plan.go:599:2                                 nestif    `if b.client.IsEnterprise()` has complex nested blocks (complexity: 6)
internal/cloud/backend_taskStages_test.go:261:3                      nestif    `if c.isError` has complex nested blocks (complexity: 6)
internal/cloud/e2e/main_test.go:76:1                                 gocognit  cognitive complexity 72 of func `testRunner` is high (> 50)
internal/cloud/e2e/migrate_state_remote_backend_to_tfc_test.go:15:1  cyclop    calculated cyclomatic complexity for function Test_migrate_remote_backend_single_org is 25, max is 20
internal/cloud/state.go:176:2                                        nestif    `if s.readState != nil` has complex nested blocks (complexity: 6)
internal/cloud/tfe_client_mock.go:1201:1                             cyclop    calculated cyclomatic complexity for function ReadWithOptions is 22, max is 20
internal/command/cliconfig/credentials_test.go:20:1                  cyclop    calculated cyclomatic complexity for function TestCredentialsForHost is 24, max is 20
internal/command/cliconfig/credentials_test.go:200:1                 cyclop    calculated cyclomatic complexity for function TestCredentialsStoreForget is 27, max is 20
internal/command/clistate/local_state.go:139:2                       nestif    `if !s.written && (s.stateFileOut == nil || s.Path != s.PathOut)` has complex nested blocks (complexity: 6)
internal/command/e2etest/primary_test.go:28:1                        cyclop    calculated cyclomatic complexity for function TestPrimarySeparatePlan is 22, max is 20
internal/command/e2etest/providers_tamper_test.go:22:1               gocognit  cognitive complexity 79 of func `TestProviderTampering` is high (> 50)
internal/command/fmt.go:331:1                                        cyclop    calculated cyclomatic complexity for function formatValueExpr is 22, max is 20
internal/command/format/diagnostic.go:221:1                          cyclop    calculated cyclomatic complexity for function appendSourceSnippets is 22, max is 20
internal/command/format/diagnostic.go:230:2                          nestif    `if diag.Snippet == nil` has complex nested blocks (complexity: 22)
internal/command/jsonformat/computed/renderers/testing.go:188:1      gocognit  cognitive complexity 61 of func `ValidateBlock` is high (> 50)
internal/command/jsonformat/differ/differ_test.go:121:1              gocognit  cognitive complexity 103 of func `TestValue_ObjectAttributes` is high (> 50)
internal/command/jsonformat/plan.go:51                               funlen    Function 'renderHuman' has too many statements (64 > 50)
internal/command/jsonformat/plan.go:96:2                             nestif    `if len(changes) == 0 && len(outputs) == 0` has complex nested blocks (complexity: 20)
internal/command/jsonformat/plan.go:184:2                            nestif    `if willPrintResourceChanges` has complex nested blocks (complexity: 7)
internal/command/jsonformat/plan.go:371                              funlen    Function 'resourceChangeComment' has too many statements (96 > 50)
internal/command/jsonstate/state.go:338                              funlen    Function 'marshalResources' has too many statements (82 > 50)
internal/command/jsonstate/state.go:399:4                            nestif    `if ri.Current != nil` has complex nested blocks (complexity: 7)
internal/command/login_test.go:28:1                                  gocognit  cognitive complexity 66 of func `TestLogin` is high (> 50)
internal/command/meta_backend_migrate.go:54:1                        cyclop    calculated cyclomatic complexity for function backendMigrateState is 23, max is 20
internal/command/meta_backend_migrate.go:554:1                       cyclop    calculated cyclomatic complexity for function backendMigrateTFC is 21, max is 20
internal/command/views/json/diagnostic.go:136                        funlen    Function 'NewDiagnostic' has too many statements (77 > 50)
internal/command/views/json/diagnostic.go:157:2                      nestif    `if sourceRefs.Subject != nil` has complex nested blocks (complexity: 56)
internal/communicator/ssh/provisioner.go:162:1                       cyclop    calculated cyclomatic complexity for function parseConnectionInfo is 22, max is 20
internal/configs/config.go:648:1                                     gocognit  cognitive complexity 61 of func `(*Config).resolveProviderTypesForTests` is high (> 50)
internal/configs/config_build_test.go:165:1                          gocognit  cognitive complexity 97 of func `TestBuildConfigInvalidModules` is high (> 50)
internal/configs/configschema/internal_validate.go:32:1              cyclop    calculated cyclomatic complexity for function internalValidate is 25, max is 20
internal/configs/hcl2shim/values_equiv.go:29:1                       cyclop    calculated cyclomatic complexity for function ValuesSDKEquivalent is 21, max is 20
internal/configs/parser_config_dir_test.go:30:1                      gocognit  cognitive complexity 60 of func `TestParserLoadConfigDirSuccess` is high (> 50)
internal/depsfile/locks_file_test.go:21:1                            gocognit  cognitive complexity 116 of func `TestLoadLocksFromFile` is high (> 50)
internal/getproviders/http_mirror_source_test.go:23:1                gocognit  cognitive complexity 62 of func `TestHTTPMirrorSource` is high (> 50)
internal/getproviders/memoize_source_test.go:16:1                    gocognit  cognitive complexity 54 of func `TestMemoizeSource` is high (> 50)
internal/getproviders/registry_client_test.go:152:1                  cyclop    calculated cyclomatic complexity for function fakeRegistryHandler is 31, max is 20
internal/instances/expander_test.go:19:1                             gocognit  cognitive complexity 72 of func `TestExpander` is high (> 50)
internal/instances/set_test.go:15:1                                  gocyclo   cyclomatic complexity 48 of func `TestSet` is high (> 30)
internal/lang/functions_test.go:46:1                                 gocognit  cognitive complexity 67 of func `TestFunctions` is high (> 50)
internal/legacy/helper/schema/field_reader.go:60                     funlen    Function 'addrToSchema' has too many statements (58 > 50)
internal/legacy/helper/schema/field_reader_config.go:36              funlen    Function 'readField' has too many statements (55 > 50)
internal/legacy/helper/schema/field_reader_config.go:43:2            nestif    `if !nested` has complex nested blocks (complexity: 7)
internal/legacy/helper/schema/field_reader_config.go:147             funlen    Function 'readMap' has too many statements (52 > 50)
internal/legacy/helper/schema/resource.go:262:2                      nestif    `if ok` has complex nested blocks (complexity: 6)
internal/legacy/helper/schema/resource.go:611:1                      gocognit  cognitive complexity 65 of func `(*Resource).InternalValidate` is high (> 50)
internal/legacy/helper/schema/resource.go:629:2                      nestif    `if r.isTopLevel() && writable` has complex nested blocks (complexity: 16)
internal/legacy/helper/schema/resource_data.go:294:1                 cyclop    calculated cyclomatic complexity for function State is 22, max is 20
internal/legacy/helper/schema/resource_timeout.go:61                 funlen    Function 'ConfigDecode' has too many statements (51 > 50)
internal/legacy/helper/schema/resource_timeout.go:70:2               nestif    `if ok` has complex nested blocks (complexity: 8)
internal/legacy/helper/schema/schema.go:474                          funlen    Function 'Diff' has too many statements (61 > 50)
internal/legacy/helper/schema/schema.go:525:2                        nestif    `if handleRequiresNew` has complex nested blocks (complexity: 23)
internal/legacy/helper/schema/schema.go:692                          funlen    Function 'internalValidate' has too many statements (83 > 50)
internal/legacy/helper/schema/schema.go:757:3                        nestif    `if len(v.ConflictsWith) > 0` has complex nested blocks (complexity: 6)
internal/legacy/helper/schema/schema.go:790:3                        nestif    `if v.Type == TypeList || v.Type == TypeSet` has complex nested blocks (complexity: 7)
internal/legacy/helper/schema/schema.go:918                          funlen    Function 'diffList' has too many statements (54 > 50)
internal/legacy/helper/schema/schema.go:1040:1                       cyclop    calculated cyclomatic complexity for function diffMap is 22, max is 20
internal/legacy/helper/schema/schema.go:1141                         funlen    Function 'diffSet' has too many statements (53 > 50)
internal/legacy/helper/schema/schema.go:1531:1                       cyclop    calculated cyclomatic complexity for function validateMap is 21, max is 20
internal/legacy/helper/schema/serialize.go:15                        funlen    Function 'SerializeValueForHash' has too many statements (52 > 50)
internal/legacy/helper/schema/shims_test.go:497:1                    gocognit  cognitive complexity 64 of func `TestShimSchemaMap_Diff` is high (> 50)
internal/legacy/tofu/diff.go:465                                     funlen    Function 'applyBlockDiff' has too many statements (77 > 50)
internal/legacy/tofu/diff.go:616:3                                   nestif    `if ok` has complex nested blocks (complexity: 18)
internal/legacy/tofu/diff.go:701:2                                   nestif    `if diff == nil` has complex nested blocks (complexity: 7)
internal/legacy/tofu/diff.go:751                                     funlen    Function 'applyCollectionDiff' has too many statements (71 > 50)
internal/legacy/tofu/diff.go:1212                                    funlen    Function 'Same' has too many statements (95 > 50)
internal/legacy/tofu/diff.go:1231:2                                  nestif    `if oldNew && !newNew` has complex nested blocks (complexity: 6)
internal/legacy/tofu/diff.go:1329:3                                  nestif    `if !ok` has complex nested blocks (complexity: 12)
internal/legacy/tofu/provider_mock.go:298:2                          nestif    `if p.ImportStateReturn != nil` has complex nested blocks (complexity: 6)
internal/legacy/tofu/state.go:1218                                   funlen    Function 'String' has too many statements (72 > 50)
internal/legacy/tofu/state.go:1937                                   funlen    Function 'ReadState' has too many statements (53 > 50)
internal/legacy/tofu/state_filter.go:67:1                            gocognit  cognitive complexity 51 of func `(*StateFilter).filterSingle` is high (> 50)
internal/legacy/tofu/state_filter.go:108:4                           nestif    `if f.relevant(a, r)` has complex nested blocks (complexity: 6)
internal/legacy/tofu/upgrade_state_v2_test.go:17:1                   gocyclo   cyclomatic complexity 34 of func `TestReadUpgradeStateV2toV3` is high (> 30)
internal/plans/objchange/compatible.go:35                            funlen    Function 'assertObjectCompatible' has too many statements (80 > 50)
internal/plans/objchange/compatible.go:123:4                         nestif    `if plannedV.Type().IsObjectType()` has complex nested blocks (complexity: 8)
internal/plans/objchange/compatible.go:201                           funlen    Function 'assertValueCompatible' has too many statements (55 > 50)
internal/plans/objchange/lcs.go:54:4                                 nestif    `if eq` has complex nested blocks (complexity: 7)
internal/plans/objchange/normalize_obj.go:32:1                       cyclop    calculated cyclomatic complexity for function normalizeObjectFromLegacySDK is 23, max is 20
internal/plans/objchange/plan_valid.go:44                            funlen    Function 'assertPlanValid' has too many statements (95 > 50)
internal/plans/objchange/plan_valid.go:153:4                         nestif    `if plannedV.Type().IsObjectType()` has complex nested blocks (complexity: 10)
internal/plans/objchange/plan_valid.go:336                           funlen    Function 'assertPlannedObjectValid' has too many statements (76 > 50)
internal/providercache/installer_test.go:31:1                        gocognit  cognitive complexity 169 of func `TestEnsureProviderVersions` is high (> 50)
internal/providercache/installer_test.go:2502:1                      cyclop    calculated cyclomatic complexity for function fakeRegistryHandler is 32, max is 20
internal/refactoring/move_execute.go:33                              funlen    Function 'ApplyMoves' has too many statements (74 > 50)
internal/refactoring/move_validate.go:38                             funlen    Function 'ValidateMoves' is too long (131 > 100)
internal/registry/test/mock_registry.go:134                          funlen    Function 'mockRegHandler' has too many statements (62 > 50)
internal/repl/format.go:21:1                                         cyclop    calculated cyclomatic complexity for function FormatValue is 23, max is 20
internal/states/state_string.go:79                                   funlen    Function 'testString' has too many statements (91 > 50)
internal/states/state_test.go:430:1                                  cyclop    calculated cyclomatic complexity for function TestState_MoveAbsResource is 21, max is 20
internal/states/statefile/version3_upgrade.go:24                     funlen    Function 'upgradeStateV3ToV4' has too many statements (75 > 50)
internal/states/statefile/version3_upgrade.go:94:4                   nestif    `if !exists` has complex nested blocks (complexity: 17)
internal/states/statefile/version3_upgrade.go:275:1                  cyclop    calculated cyclomatic complexity for function upgradeInstanceObjectV3ToV4 is 21, max is 20
internal/states/statefile/version4.go:40                             funlen    Function 'prepareStateV4' has too many statements (144 > 50)
internal/states/statefile/version4.go:360                            funlen    Function 'writeStateV4' has too many statements (63 > 50)
internal/states/statefile/version4.go:851:2                          nestif    `if ki != kj` has complex nested blocks (complexity: 7)
internal/tofu/context_apply2_test.go:1103:1                          cyclop    calculated cyclomatic complexity for function TestContext2Apply_resourceConditionApplyTimeFail is 21, max is 20
internal/tofu/context_apply_test.go:11305:1                          cyclop    calculated cyclomatic complexity for function TestContext2Apply_scaleInCBD is 24, max is 20
internal/tofu/context_functions_test.go:23:1                         gocognit  cognitive complexity 58 of func `TestFunctions` is high (> 50)
internal/tofu/context_plan2_test.go:2993:1                           gocognit  cognitive complexity 62 of func `TestContext2Plan_resourcePreconditionPostcondition` is high (> 50)
internal/tofu/context_plan2_test.go:3282:1                           gocognit  cognitive complexity 61 of func `TestContext2Plan_dataSourcePreconditionPostcondition` is high (> 50)
internal/tofu/context_test.go:787:1                                  gocognit  cognitive complexity 97 of func `legacyDiffComparisonString` is high (> 50)
internal/tofu/eval_variable_test.go:23:1                             gocognit  cognitive complexity 65 of func `TestPrepareFinalInputVariableValue` is high (> 50)
internal/tofu/eval_variable_test.go:1073:1                           gocognit  cognitive complexity 61 of func `TestEvalVariableValidations_jsonErrorMessageEdgeCase` is high (> 50)
internal/tofu/transform_destroy_edge.go:286:1                        gocognit  cognitive complexity 54 of func `(*pruneUnusedNodesTransformer).Transform` is high (> 50)
tools/loggraphdiff/loggraphdiff.go:50:1                              cyclop    calculated cyclomatic complexity for function main is 21, max is 20
```

Although 160 problems is too many to address in a single pull request, it is _not_ too many to attack gradually over multiple pull requests, with each one focusing on one area of the code so that the diffs are approachable.

After we disable these five linters from the set of checks we require for new pull requests, we can begin working systematically through this list and proactively reorganizing functions on a file-by-file or package-by-package basis, depending on the severity of the changes. This work should not change the observable behavior of OpenTofu in any way, and we should reinforce that during our work by modifying test code separately from main code to ensure that all existing tests remain passing throughout the work.

During this work we may conclude that some of the mostly-arbitrary thresholds for the metrics that we've currently configured in our lint configuration are too strict, if we reach a point where it seems that further decomposition would make code harder rather than easier to read. In each such case we will start by re-adding localized `nolint` comments to allow us to keep making localized progress, deferring broader considerations for the end of the project.

It is possible that other concurrent work will introduce new code that violates the rules while we have the linters disabled. It is unlikely that we can add new problems faster than we can fix them, so as long as we continue work on this project consistently until it is completed we should be able to converge on an empty list of failures, at which point we'll proceed to the next section.

### After We've Fixed Everything

Once we've reduced the number of complexity-related lint failures to zero, we will mark the occasion by immediately re-enabling the five linters we disabled at the start of the project, making all new pull requests require those limits to be met.

After the lint rules are reenabled, we will revisit each `nolint` comment that remains in the codebase to decide if there seem to be any common factors that suggest that some of our configured thresholds are too strict. If so, we will adjust the affected metric thresholds and remove the `nolint` directives for them. We may also choose to make other related compromises, such as choosing looser metrics for test code than main code, depending on what patterns we find.

It is _not_ a goal to totally eliminate all `nolint` directives: some code may be truly exceptional in its structure or its constraints, and in that case we should prefer to make a localized exception rather than loosening the constraints for all code written in the future. However, each remaining `nolint` directives should be accompanied by a comment that justifies its use so that future maintainers of the same code can re-evaluate whether the exception is still justified and, if so, can maintain the code in a way that broadly retains the same compromises.
