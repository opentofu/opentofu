# OpenTofu Change Classification System

## Overview

The change classification system automatically analyzes OpenTofu plans and determines the risk level for each change. This enables smarter CI/CD pipelines with automatic approval of safe changes.

## Safety Levels

### Safe
Changes that can be auto-approved:
- **Create** - Creating new resources
- **Read** - Read operations (data sources)
- **NoOp** - No changes

### Risky
Changes requiring review:
- **Update** - Updating existing resources (may affect availability)

### Destructive
Changes that should block automatic approval:
- **Delete** - Deleting resources
- **DeleteThenCreate** - Replace (recreate)
- **CreateThenDelete** - Replace (create first)

## CLI Usage

### Plan with classification

```bash
tofu plan -classify-changes -out=plan.tfplan
```

Output example:
```
Total resource changes: 5
Classified changes: 5

--- Change Classification ---
Safe changes: 2
Risky changes: 1
Destructive changes: 2
```

### Apply with auto-approval

```bash
tofu apply -auto-approve-safe plan.tfplan
```

Behavior:
- Plans with only safe changes → auto-approved
- Plans with risky/destructive changes → manual confirmation required

## Programmatic Usage

```go
import "github.com/opentofu/opentofu/internal/plans/classifier"

c := classifier.NewResourceClassifier()

classifications := c.ClassifyPlan(plan)

counts := c.CountBySafetyLevel(plan)
fmt.Printf("Safe: %d, Risky: %d, Destructive: %d\n",
    counts[classifier.SafetySafe],
    counts[classifier.SafetyRisky],
    counts[classifier.SafetyDestructive])

if c.HasOnlySafeChanges(plan) {
    fmt.Println("Safe to auto-apply")
}

if c.HasDestructiveChanges(plan) {
    fmt.Println("Manual review required")
}

for _, change := range plan.Changes.Resources {
    classification := c.ClassifyResourceChange(change)
    fmt.Printf("%s: %s (%s)\n",
        change.Addr,
        classification.SafetyLevel,
        classification.Reason)
}
```

## CI/CD Integration

### GitHub Actions

```yaml
- name: OpenTofu Plan
  run: tofu plan -classify-changes -out=plan.tfplan

- name: Auto Apply Safe Changes
  run: tofu apply -auto-approve-safe plan.tfplan
```

## Architecture

### Components

```
internal/plans/classifier/
├── classifier.go           # Base classifier
├── resource_classifier.go  # Resource classifier
├── classifier_test.go      # Unit tests
└── integration_test.go     # Integration tests
```

### Data Structures

```go
type SafetyLevel int

const (
    SafetyUnknown     SafetyLevel = iota
    SafetySafe
    SafetyRisky
    SafetyDestructive
)

type ChangeClassification struct {
    SafetyLevel SafetyLevel
    Reason      string
    Description string
}
```

## Testing

```bash
# Run all tests
go test -v ./internal/plans/classifier/

# Run with coverage
go test -cover ./internal/plans/classifier/
```


