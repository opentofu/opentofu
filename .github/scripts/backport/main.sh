#!/bin/bash

set -e

# Setting up Committer identity.
git config user.email "noreply@github.com"
git config user.name "GitHub Actions"

# Get backport label from all the labels of the PR
backport_label=""
branch_name=""
readarray -t labelArray < <(echo "$LABELS" | jq -r '.[]')
for label in "${labelArray[@]}"; do
    if [[ $label == "backport"* ]]; then
        backport_label=$label
        branch_name=${label#backport }
        break
    fi
done

# Exit if backport label not found
if [ -z "$backport_label" ]; then
    echo "Warning: The backport label is not found on the Pull Request. Skipping the backport process"
    exit 0
fi

# Check if GitHub token is set
if [ -z "$GITHUB_TOKEN" ]; then
    echo "Error: GitHub token is not available. Please ensure a secret named 'GITHUB_TOKEN' is defined."
    exit 1
fi

# Check if gh is logged in
if ! gh auth status >/dev/null 2>&1; then
    echo "Error: Authentication check for 'gh' failed."
    exit 1
fi

# Checkout the version branch
echo "Fetching branch ${branch_name}..."
git fetch origin "$branch_name"
echo "Checking out branch ${branch_name}..."
git checkout "$branch_name"

# Checkout new backport branch
new_branch="backport/$branch_name/$ISSUE_NUMBER"
git checkout -b "$new_branch"
git push origin "$new_branch"
echo "Checking out new backport branch: ${new_branch}"

# Get the commit SHAs associated with the pull request
gh api "/repos/$OWNER/$REPO/pulls/$PR_NUMBER/commits" --jq '.[].sha' |
while IFS= read -r commit_sha; do
    git fetch origin "$commit_sha"
    if git cherry-pick "$commit_sha" ; then
        echo "Cherry-picking commit: ${commit_sha}"
    else
        # Add a failure comment to the pull request
        echo "Error: Failed to cherry-pick commit ${commit_sha}"
        failureComment="Error: Cherry-picking commit $commit_sha failed during the backport process to $branch_name. Please resolve any conflicts and manually apply the changes."
        gh pr comment "$PR_NUMBER" --body "$failureComment"
        exit 1
    fi
done
echo "Cherry-picking process completed successfully for pull request #${PR_NUMBER}."

git push origin "$new_branch"
echo "Successfully pushed changes to the ${new_branch}"

# Create the pull request.
title="Backport PR #$ISSUE_NUMBER"
body="This pull request backports changes from $HEAD_BRANCH to $branch_name."
url=$(gh pr create --title "$title" --body "$body" --base "$branch_name" --head "$new_branch")
echo "Pull request created successfully. You can view the pull request at: ${url}"

# Add comment to the original PR.
comment="This pull request has been successfully backported. Link to the new pull request: ${url}"
gh pr comment "$PR_NUMBER" --body "$comment"
echo "Comment added successfully to the original pull request ${PR_NUMBER}."