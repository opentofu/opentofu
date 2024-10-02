package gitlab

import (
	"context"
	"fmt"
	"sort"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/shurcooL/graphql"
)

// Workspaces returns a list of names for the workspaces found in Gitlab.
// The default workspace is always returned as the first element in the slice.
func (b *Backend) Workspaces() ([]string, error) {
	graphqlEndpoint := b.address.JoinPath("api", "graphql").String()
	graphqlClient := graphql.NewClient(graphqlEndpoint, b.httpClient.StandardClient())

	var query struct {
		Project struct {
			TerraformStates struct {
				Nodes []struct {
					Name string
				}
			}
		} `graphql:"project(fullPath: $projectPath)"`
	}

	variables := map[string]interface{}{
		"projectPath": b.project,
	}

	if err := graphqlClient.Query(context.Background(), &query, variables); err != nil {
		return nil, fmt.Errorf("unable to execute graphql query: %w", err)
	}

	states := []string{backend.DefaultStateName}

	for _, v := range query.Project.TerraformStates.Nodes {
		if v.Name != backend.DefaultStateName {
			states = append(states, v.Name)
		}
	}

	// The default state always comes first.
	sort.Strings(states[1:])

	return states, nil
}

func (b *Backend) StateMgr(stateName string) (statemgr.Full, error) {
	return remote.NewState(b.remoteClientFor(stateName), b.encryption), nil
}

func (b *Backend) DeleteWorkspace(stateName string, _ bool) error {
	if stateName == backend.DefaultStateName || stateName == "" {
		return fmt.Errorf("can't delete default state")
	}

	return b.remoteClientFor(stateName).Delete()
}
