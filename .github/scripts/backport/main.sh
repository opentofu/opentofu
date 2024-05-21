#!/bin/bash

set -e

# Function to handle errors
log_error() {
    if [ -z "$1" ]; then
        echo "Error: No error message provided." >&2
        exit 1
    fi

    echo "Error: $1" >&2
    exit 1
}

# Function to output info logs
log_info() {
    if [ -z "$1" ]; then
        echo "No message provided."
        return
    fi

    echo "$1"
}

# Setting up Committer identity.
log_info "Setting up Committer identity..."
if ! git config user.email "noreply@github.com"; then
    log_error "Failed to set email."
fi

if ! git config user.name "GitHub Actions"; then
    log_error "Failed to set username"
fi
log_info "Successfully set the committer identity."

# Get backport label from all the labels of the PR
backport_label=""
branch_name=""

log_info "Starting the process of identifying a backport label from the list of labels... "
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
    log_info "The backport label is not found on the Pull Request. Skipping the backport process"
    exit 0
fi
log_info "Successfully found backport label: $backport_label"

# Check if GitHub token is set
if [ -z "$GITHUB_TOKEN" ]; then
    log_error "GitHub token is not available. Please ensure a secret named 'GITHUB_TOKEN' is defined."
fi

# Check if gh is logged in
if ! gh auth status >/dev/null 2>&1; then
    log_error "Authentication check for 'gh' failed."
fi

if [ -z "$OWNER" ] || [ -z "$REPO" ] || [ -z "$PR_NUMBER" ] || [ -z "$ISSUE_NUMBER" ] || [ -z "$HEAD_BRANCH" ]; then
    log_error "One or more required environment variables (OWNER, REPO, PR_NUMBER, ISSUE_NUMBER, HEAD_BRANCH) are not set."
fi

# Checkout the version branch
log_info "Fetching branch ${branch_name}..."
if ! git fetch origin "$branch_name"; then
    log_error "Failed to fetch branch ${branch_name}"
fi

log_info "Checking out branch ${branch_name}..."
if ! git checkout "$branch_name"; then
    log_error "Failed to checkout branch ${branch_name}"
fi

# Checkout new backport branch
new_branch="backport/$branch_name/$ISSUE_NUMBER"
log_info "Checking out new backport branch ${new_branch}..."
if ! git checkout -b "$new_branch"; then 
    log_error "Failed to create new backport branch ${new_branch}"
fi

if ! git push origin "$new_branch"; then
    log_error "Failed to push new backport branch ${new_branch}"
fi

# Get the commit SHAs associated with the pull request
log_info "Starting the cherry-picking process for pull request #${PR_NUMBER}..."
if ! commit_shas=$(gh api "/repos/$OWNER/$REPO/pulls/$PR_NUMBER/commits" --jq '.[].sha'); then
    log_error "Failed to fetch commit SHAs from pull request #${PR_NUMBER}."
fi

# Check if commit_shas is empty
log_info "Checking if any commit SHAs were fetched..."
if [ -z "$commit_shas" ]; then
    log_error "No commit SHAs were found in pull request #${PR_NUMBER}."
fi
log_info "Commit SHAs were successfully fetched."

echo "$commit_shas" | while IFS= read -r commit_sha; do
    log_info "Cherry-picking commit: ${commit_sha}"
    if ! git fetch origin "$commit_sha"; then
        log_error "Failed to fetch commit id ${commit_sha}"
    fi
    if git cherry-pick "$commit_sha" ; then
        log_info "Successfully cherry-picked commit: ${commit_sha}"
    else
        # Add a failure comment to the pull request
        log_info "Error: Failed to cherry-pick commit ${commit_sha}" >&2
        failureComment="Error: Cherry-picking commit $commit_sha failed during the backport process to $branch_name. Please resolve any conflicts and manually apply the changes."
        if ! gh pr comment "$PR_NUMBER" --body "$failureComment"; then
            log_error "Failed to add failure comment to pull request #${PR_NUMBER}."
        fi
    fi
done
log_info "Cherry-picking process completed successfully for pull request #${PR_NUMBER}."

# Push changes to the new backport branch
log_info "Pushing changes to the ${new_branch}..."
if ! git push origin "$new_branch"; then
    log_error "Failed to push changes to the ${new_branch}."
fi
log_info "Successfully pushed changes to the ${new_branch}"

# Create the pull request.
title="Backport PR #$ISSUE_NUMBER"
body="This pull request backports changes from $HEAD_BRANCH to $branch_name."
log_info "Creating pull request from $HEAD_BRANCH to $branch_name."
if ! url=$(gh pr create --title "$title" --body "$body" --base "$branch_name" --head "$new_branch"); then
    log_error "Failed to create pull request."
fi
log_info "Pull request created successfully. You can view the pull request at: ${url}"

# Add comment to the original PR.
comment="This pull request has been successfully backported. Link to the new pull request: ${url}"
log_info "Adding comment to the original pull request #${PR_NUMBER}..."
if ! gh pr comment "$PR_NUMBER" --body "$comment"; then
    log_error "Failed to add comment to the original pull request #${PR_NUMBER}."
fi
log_info "Comment added successfully to the original pull request #${PR_NUMBER}."