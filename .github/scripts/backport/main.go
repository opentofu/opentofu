package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-github/v61/github"
)

type GithubEvent struct {
	Number      int `json:"number"`
	PullRequest struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		Assignee struct {
			Login string `json:"login"`
		} `json:"assignee"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	} `json:"pull_request"`
	Repository struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
}

type Commit struct {
	SHA string `json:"sha"`
}

func main() {
	githubEvent := os.Getenv("GITHUB_EVENT")

	ctx := context.Background()
	gitClient := github.NewClient(nil).WithAuthToken(os.Getenv("GITHUB_TOKEN"))

	// Parse the JSON data into a PullRequestEvent struct
	var event GithubEvent
	err := json.Unmarshal([]byte(githubEvent), &event)
	if err != nil {
		fmt.Println("Error decoding GitHub event:", err)
		return
	}

	owner := event.Repository.Owner.Login
	repo := event.Repository.Name
	pullNumber := event.Number
	issueNumber := event.Number

	commitIds := getCommitIds(ctx, owner, repo, pullNumber, gitClient)

	// There can be many labels
	var backportLabel string
	for _, label := range event.PullRequest.Labels {
		if strings.Contains(label.Name, "backport") {
			backportLabel = label.Name
			break
		}
	}
	backportBranch := getBranchFromLabel(backportLabel)

	newBranch, err := checkoutBranch(ctx, owner, repo, backportBranch, issueNumber, gitClient)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	err = cherryPickCommits(ctx, owner, repo, newBranch, backportBranch, commitIds, gitClient)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	// Create a pull request from the test branch to the target branch
	link, err := createPullRequest(ctx, owner, repo, newBranch, backportBranch, issueNumber, gitClient)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	// Create comment
	comment := &github.IssueComment{
		Body: github.String("Backport pull request created successfully! You can view the pull request: " + link),
	}
	err = addCommentToPullRequest(ctx, owner, repo, link, comment, pullNumber, gitClient)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
}

// cherryPickCommit cherry-picks a commit onto a branch
func cherryPickCommit(ctx context.Context, client *github.Client, owner, repo, branchName, commitSHA string) error {
	// Get the details of the commit to cherry-pick
	commit, _, err := client.Git.GetCommit(ctx, owner, repo, commitSHA)
	if err != nil {
		log.Fatalf("Error getting commit details: %v", err)
	}

	// Create a new commit object with the cherry-picked changes
	newCommit := &github.Commit{
		Message: commit.Message,
		Author:  commit.Author,
		Committer: &github.CommitAuthor{
			Name:  github.String("githubbot"),
			Email: github.String("noreply@github.com"),
		},
		Tree:    commit.Tree,
		Parents: commit.Parents,
	}

	// Create the commit
	createdCommit, _, err := client.Git.CreateCommit(ctx, owner, repo, newCommit, nil)
	if err != nil {
		log.Fatalf("Error creating commit: %v", err)
	}

	// Update the target branch reference to point to the new commit
	refName := "refs/heads/" + branchName
	ref, _, err := client.Git.GetRef(ctx, owner, repo, refName)
	if err != nil {
		return fmt.Errorf("error getting branch reference: %v", err)
	}

	// Perform a fast-forward merge to update the branch
	ref.Object.SHA = createdCommit.SHA
	_, _, err = client.Git.UpdateRef(ctx, owner, repo, ref, true)
	if err != nil {
		return fmt.Errorf("error updating branch reference: %v", err)
	}

	fmt.Println("Commit cherry-picked successfully", branchName)

	return nil
}

// Cherry-pick each commit onto the target branch
func cherryPickCommits(ctx context.Context, owner, repo, newBranch, backportBranch string, commitIds []*github.RepositoryCommit, gitClient *github.Client) error {
	var err error
	for _, commit := range commitIds {
		err = cherryPickCommit(ctx, gitClient, owner, repo, newBranch, *commit.SHA)
		if err != nil {
			return fmt.Errorf("error cherry-picking commit %s : %v", *commit.SHA, err)
		}
		fmt.Println("Success cherry-picking commit --> ", *commit.SHA)
	}
	return nil
}

func checkoutBranch(ctx context.Context, owner, repo, backportBranch string, issueNumber int, gitClient *github.Client) (string, error) {
	// New branch name
	newBranch := "backport/" + fmt.Sprint(issueNumber)

	baseRef, _, err := gitClient.Git.GetRef(ctx, owner, repo, "refs/heads/"+backportBranch)
	if err != nil {
		return "", fmt.Errorf("error getting base reference SHA: %v", err)
	}

	// Create a reference for the new branch
	reference := &github.Reference{
		Ref: github.String("refs/heads/" + newBranch),
		Object: &github.GitObject{
			SHA: baseRef.Object.SHA,
		},
	}

	// Create the new branch
	_, _, err = gitClient.Git.CreateRef(ctx, owner, repo, reference)
	if err != nil {
		return "", fmt.Errorf("error creating new branch: %v", err)
	}

	fmt.Println("New branch created:", newBranch)
	return newBranch, nil
}

// label format backport <version_branch>
func getBranchFromLabel(label string) string {
	re := regexp.MustCompile(`^backport ([^ ]+)$`)
	match := re.FindStringSubmatch(label)

	if len(match) > 1 {
		branch := match[1]
		fmt.Println("Backport Branch:", branch)
		return branch
	}

	fmt.Println("No backport branch found for label" + label)
	return ""
}

// Fetch commits for the pull request
func getCommitIds(ctx context.Context, owner, repo string, pullNumber int, gitClient *github.Client) []*github.RepositoryCommit {
	commits, _, err := gitClient.PullRequests.ListCommits(ctx, owner, repo, pullNumber, nil)
	if err != nil {
		fmt.Println("Error fetching commits:", err)
		return nil
	}
	return commits
}

// Create a pull request from the test branch to the target branch
func createPullRequest(ctx context.Context, owner, repo, branch, backportBranch string, issueNumber int, gitClient *github.Client) (string, error) {
	pr, _, err := gitClient.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: github.String("Backporting changes to " + backportBranch),
		Head:  github.String(branch),
		Base:  github.String(backportBranch),
		Body:  github.String("Resolves: " + strconv.Itoa(issueNumber)),
	})
	if err != nil {
		return "", fmt.Errorf("error creating pull request: %v", err)
	}
	fmt.Println("Pull Request successfully created: ", *pr.HTMLURL)
	return *pr.HTMLURL, nil
}

// Add comment to the PR
func addCommentToPullRequest(ctx context.Context, owner, repo, prLink string, comment *github.IssueComment, prNumber int, gitClient *github.Client) error {
	_, _, err := gitClient.Issues.CreateComment(ctx, owner, repo, prNumber, comment)
	if err != nil {
		return fmt.Errorf("error adding comment to PR: %v", err)
	}
	fmt.Println("Comment added successfully.")
	return nil
}
