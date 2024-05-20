#!/bin/bash

set -e

# Function to handle errors
handle_error() {
    echo "Error: $1" >&2
    exit 1
}

# Setting up Committer identity.
git config user.email "noreply@github.com"
git config user.name "GitHub Actions"

# Get backport label from all the labels of the PR
backport_label=""
branch_name=""

echo "Starting the process of identifying a backport label from the list of labels... "
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
    echo "The backport label is not found on the Pull Request. Skipping the backport process"
    exit 0
fi
echo "Successfully found backport label: $backport_label"

# Check if GitHub token is set
if [ -z "$GITHUB_TOKEN" ]; then
    handle_error "GitHub token is not available. Please ensure a secret named 'GITHUB_TOKEN' is defined."
fi

# Check if gh is logged in
if ! gh auth status >/dev/null 2>&1; then
    handle_error "Authentication check for 'gh' failed."
fi

if [ -z "$OWNER" ] || [ -z "$REPO" ] || [ -z "$PR_NUMBER" ] || [ -z "$ISSUE_NUMBER" ] || [ -z "$HEAD_BRANCH" ]; then
    handle_error "Required environment variables (OWNER, REPO, PR_NUMBER, ISSUE_NUMBER, HEAD_BRANCH) are not set."
fi

# Checkout the version branch
echo "Fetching branch ${branch_name}..."
if ! git fetch origin "$branch_name"; then
    handle_error "Failed to fetch branch ${branch_name}"
fi

echo "Checking out branch ${branch_name}..."
if ! git checkout "$branch_name"; then
    handle_error "Failed to checkout branch ${branch_name}"
fi

# Checkout new backport branch
new_branch="backport/$branch_name/$ISSUE_NUMBER"
echo "Checking out new backport branch: ${new_branch}"
if ! git checkout -b "$new_branch"; then 
    handle_error "Failed to create new backport branch ${new_branch}"
fi

if ! git push origin "$new_branch"; then
    handle_error "Failed to push new backport branch ${new_branch}"
fi

# Get the commit SHAs associated with the pull request
echo "Starting the cherry-picking process for pull request #${PR_NUMBER}..."
if ! commit_shas=$(gh api "/repos/$OWNER/$REPO/pulls/$PR_NUMBER/commits" --jq '.[].sha'); then
    handle_error "Failed to fetch commit SHAs from pull request #${PR_NUMBER}."
fi

# Check if commit_shas is empty
echo "Checking if any commit SHAs were fetched..."
if [ -z "$commit_shas" ]; then
    handle_error "No commit SHAs were found in pull request #${PR_NUMBER}."
fi
echo "Commit SHAs were successfully fetched."

echo "$commit_shas" | while IFS= read -r commit_sha; do
    echo "Cherry-picking commit: ${commit_sha}"
    if ! git fetch origin "$commit_sha"; then
        handle_error "Failed to fetch commit id ${commit_sha}"
    fi
    if git cherry-pick "$commit_sha" ; then
        echo "Successfully cherry-picked commit: ${commit_sha}"
    else
        # Add a failure comment to the pull request
        echo "Error: Failed to cherry-pick commit ${commit_sha}" >&2
        failureComment="Error: Cherry-picking commit $commit_sha failed during the backport process to $branch_name. Please resolve any conflicts and manually apply the changes."
        if ! gh pr comment "$PR_NUMBER" --body "$failureComment"; then
            handle_error "Failed to add failure comment to pull request #${PR_NUMBER}."
        fi
    fi
done
echo "Cherry-picking process completed successfully for pull request #${PR_NUMBER}."

# Push changes to the new backport branch
echo "Pushing changes to the ${new_branch}..."
if ! git push origin "$new_branch"; then
    handle_error "Failed to push changes to the ${new_branch}."
fi
echo "Successfully pushed changes to the ${new_branch}"

# Create the pull request.
title="Backport PR #$ISSUE_NUMBER"
body="This pull request backports changes from $HEAD_BRANCH to $branch_name."
echo "Creating pull request from $HEAD_BRANCH to $branch_name."
if ! url=$(gh pr create --title "$title" --body "$body" --base "$branch_name" --head "$new_branch"); then
    handle_error "Failed to create pull request."
fi
echo "Pull request created successfully. You can view the pull request at: ${url}"

# Add comment to the original PR.
comment="This pull request has been successfully backported. Link to the new pull request: ${url}"
echo "Adding comment to the original pull request #${PR_NUMBER}..."
if ! gh pr comment "$PR_NUMBER" --body "$comment"; then
    handle_error "Failed to add comment to the original pull request #${PR_NUMBER}."
fi
echo "Comment added successfully to the original pull request #${PR_NUMBER}."