// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"context"
	_ "embed"
	"log"
	"os"
	"slices"
	"strconv"
	"text/template"

	"github.com/google/go-github/v61/github"
)

//go:embed ranking.tmpl
var rankingTemplateContent []byte

func main() {
	owner := os.Args[1]
	repo := os.Args[2]
	issueNumberToUpdate, err := strconv.Atoi(os.Args[3])
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Initialize client.
	client := github.NewClient(nil)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		client = client.WithAuthToken(token)
	}

	// List all open issues.
	listOpts := &github.IssueListByRepoOptions{
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	var issues []*github.Issue
	for {
		curIssues, resp, err := client.Issues.ListByRepo(ctx, owner, repo, listOpts)
		if err != nil {
			log.Fatal(err)
		}
		issues = append(issues, curIssues...)
		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	// Filter to those that have at least 2 thumbs-up.
	allIssues := issues
	issues = nil
	for _, issue := range allIssues {
		if issue.GetReactions().GetPlusOne() < 2 {
			continue
		}
		issues = append(issues, issue)
	}

	// Sort by thumbs-up descending.
	slices.SortFunc(issues, func(a, b *github.Issue) int {
		return b.GetReactions().GetPlusOne() - a.GetReactions().GetPlusOne()
	})

	templateParams := struct {
		EnhancementIssues []*github.Issue
		BugIssues         []*github.Issue
	}{
		getTopIssuesByLabel("enhancement", issues),
		getTopIssuesByLabel("bug", issues),
	}

	// Render template for issue body.
	var rankingBodyBuffer bytes.Buffer
	rankingTemplate := template.Must(template.New("ranking").Parse(string(rankingTemplateContent)))
	if err := rankingTemplate.Execute(&rankingBodyBuffer, templateParams); err != nil {
		log.Fatal(err)
	}
	rankingBodyString := rankingBodyBuffer.String()

	// Update the issue.
	_, _, err = client.Issues.Edit(ctx, owner, repo, issueNumberToUpdate, &github.IssueRequest{
		Body: &rankingBodyString,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func getTopIssuesByLabel(label string, issues []*github.Issue) []*github.Issue {
	var labelledIssues []*github.Issue
	for _, issue := range issues {
		if hasLabel(label, issue) {
			labelledIssues = append(labelledIssues, issue)
		}
	}

	// Just the top 20.
	if len(labelledIssues) > 20 {
		labelledIssues = labelledIssues[:20]
	}

	return labelledIssues
}

func hasLabel(label string, issue *github.Issue) bool {
	for _, issueLabel := range issue.Labels {
		if issueLabel.GetName() == label {
			return true
		}
	}
	return false
}
