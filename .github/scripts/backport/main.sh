#!/bin/bash

set -eou pipefail

# Color codes for logs
COLOR_RESET="\033[0m"
COLOR_GREEN="\033[32m"
COLOR_RED="\033[31m"

# This function outputs error logs.
log_error() {
    if [ -z "$1" ]; then
        echo -e "${COLOR_RED}Error: No error message provided.${COLOR_RESET}" >&2
        exit 1
    fi
    echo -e "${COLOR_RED}Error: $1${COLOR_RESET}" >&2
    exit 1
}

# This function outputs info logs.
log_info() {
    if [ -z "$1" ]; then
        echo -e "${COLOR_RED}Error: No info message provided.${COLOR_RESET}"
        return
    fi
    echo -e "${COLOR_GREEN}$1${COLOR_RESET}"
}

# This function sets up the committer identity.
setup_committer_identity() {
    log_info "Setting up committer identity..."
    if ! git config user.email "noreply@github.com"; then
        log_error "Failed to set email."
    fi

    if ! git config user.name "GitHub Actions"; then
        log_error "Failed to set a username."
    fi
    log_info "Successfully set the committer identity."
}

# This function checks the GITHUB_TOKEN, gh auth and environment variables are set or not.
validate_github_auth_and_env_vars() {
    # Check if GitHub token is set
    log_info "Checking if GITHUB_TOKEN is set..."
    if [ -z "$GITHUB_TOKEN" ]; then
        log_error "GitHub token is not available. Please ensure a secret named 'GITHUB_TOKEN' is defined."
    fi
    log_info "GITHUB_TOKEN is set successfully."

    log_info "Checking if required environment variables are set..."
    if [ -z "$OWNER" ] || [ -z "$REPO" ] || [ -z "$PR_NUMBER" ] || [ -z "$ISSUE_NUMBER" ] || [ -z "$HEAD_BRANCH" ]; then
        log_error "One or more required environment variables (OWNER, REPO, PR_NUMBER, ISSUE_NUMBER, HEAD_BRANCH) are not set."
    fi
    log_info "All the environment variables are set successfully."

    # Check if gh is logged in
    log_info "Checking if 'gh' auth is set..."
    if ! gh auth status >/dev/null 2>&1; then
        log_error "Authentication check for 'gh' failed."
    fi
    log_info "'gh' authentication is set up successfully."
}

# This function cherry-picks the commits onto the new backport branch.
cherry_pick_commits() {
    # Get the commit SHAs associated with the pull request
    log_info "Starting the cherry-pick process for the pull request #${PR_NUMBER}..."
    if ! commit_shas=$(gh api "/repos/$OWNER/$REPO/pulls/$PR_NUMBER/commits" --jq '.[].sha'); then
        log_error "Failed to fetch commit SHAs from pull request #${PR_NUMBER}."
    fi

    # Check if commit_shas is empty
    log_info "Checking if any commit SHAs were fetched..."
    if [ -z "$commit_shas" ]; then
        log_error "No commit SHAs were found in pull request #${PR_NUMBER}."
    fi
    log_info "Commit SHAs were successfully fetched."

    cherry_pick_failed=false # Flag to track cherry pick status.

    echo "$commit_shas" | while IFS= read -r commit_sha; do
        log_info "Fetching the commit id: '${commit_sha}'..."
        if ! git fetch origin "$commit_sha"; then
            cherry_pick_failed=true
            log_error "Failed to fetch the commit id: '${commit_sha}'"
        fi
        log_info "Successfully fetched the commit id: '${commit_sha}'"

        log_info "Cherry-picking the commit id: '${commit_sha}'..."
        if git cherry-pick "$commit_sha" ; then
            log_info "Successfully cherry-picked the commit id: '${commit_sha}'"
        else
            # Add a failure comment to the pull request
            failureComment="Error: Cherry-pick commit '$commit_sha' failed during the backport process to '$1'. Please resolve any conflicts and manually apply the changes."
            if ! gh pr comment "$PR_NUMBER" --body "$failureComment"; then
                log_error "Failed to add failure comment to the pull request #${PR_NUMBER}."
            fi
            log_info "Successfully added failure comment to the pull request."

            cherry_pick_failed=true
            log_error "Error: Failed to cherry-pick commit '${commit_sha}'. Added failure comment to the pull request."
        fi
    done

    if ! "$cherry_pick_failed"; then
        return 1
    fi

    log_info "Cherry-pick completed successfully for the pull request #${PR_NUMBER}."
}

# This function pushes the latest changes to the backport branch.
push_changes_to_branch() {
    log_info "Pushing changes to the branch: '$1'..."
    if ! git push origin "$1"; then
        log_error "Failed to push changes to the branch: '$1'."
    fi
    log_info "Successfully pushed changes to the branch: '$1'"
}

# This function creates the pull request.
create_pull_request() {
    title="Backport PR #$ISSUE_NUMBER: '$HEAD_BRANCH' to '$1'"
    body="This pull request backports changes from branch '$HEAD_BRANCH' to branch '$1'."
    if ! url=$(gh pr create --title "$title" --body "$body" --base "$1" --head "$2"); then
        log_error "Failed to create pull request."
    fi
    echo "${url}"
}

# This function performs all the operations required to backport a pull request to a target branch.
backport_branch() {
    # Fetch and checkout the target branch.
    log_info "Fetching branch '$1'..."
    if ! git fetch origin "$1"; then
        log_error "Failed to fetch branch: '$1'."
    fi
    log_info "Successfully fetched branch: '$1'."

    log_info "Checking out branch '$1'..."
    if ! git checkout "$1"; then
        log_error "Failed to checkout branch: '$1'"
    fi
    log_info "Successfully checked out branch: '$1'."

    # Checkout new backport branch
    log_info "Checking out new backport branch '$2'..."
    if ! git checkout -b "$2"; then 
        log_error "Failed to create new backport branch: '$2'"
    fi
    log_info "Successfully checked out new backport branch: '$2'."

    # Cherry-pick commits
    if ! cherry_pick_commits "$1"; then
        log_error "Failed to cherry-pick commits for the pull request."
    fi

    # Push final changes to the backport branch
    if ! push_changes_to_branch "$2"; then
        log_error "Failed to push latest changes to branch: '$2'."
    fi
    
    # Create the pull request 
    log_info "Creating pull request between '$1' and '$2'..."
    if ! pull_request_url=$(create_pull_request "$1" "$2"); then
        log_error "Failed to create the pull request."
    fi
    log_info "Pull request created successfully. You can view the pull request at: ${pull_request_url}."

    # Add comment to the pull request
    comment="This pull request has been successfully backported. Link to the new pull request: ${pull_request_url}."
    log_info "Adding a comment to the original pull request #${PR_NUMBER} created between '$1' and '$2'..."
    if ! gh pr comment "$PR_NUMBER" --body "$comment"; then
        log_error "Failed to add a comment to the original pull request #${PR_NUMBER} created between '$1' and '$2'."
    fi
    log_info "Successfully added a comment to the original pull request #${PR_NUMBER} created between '$1' and '$2'."
}

main() {
    log_info "Starting the process of creating pull requests for backporting merged changes to a designated target branch."
    
    # Set up committer identity
    setup_committer_identity
    # Validate GitHub token, gh auth, and environment variables
    validate_github_auth_and_env_vars
 
    # Retrieve the list of branches for backporting the pull request changes.
    log_info "Starting the process of identifying a list of branches where pull request changes will be backported to... "
    target_branches=()
    readarray -t labelArray < <(echo "$LABELS" | jq -r '.[]')
    for label in "${labelArray[@]}"; do
        if [[ $label == "backport"* ]]; then
            target_branches+=("${label#backport }")
        fi
    done

    log_info "Validate all the target branches to which the pull request changes will be backported..."
    if [ ${#target_branches[@]} -eq 0 ]; then
        log_info "Failed to retrieve the list of target branches."
        exit 0
    fi
    log_info "Successfully found the following target branches: ${target_branches[*]}"

    # Iterate over each target branch and attempt to backport the changes
    for branch in "${target_branches[@]}"; do
        new_backport_branch="backport/$branch/$ISSUE_NUMBER"
        
        log_info "Checking if pull request already exists between the branch '$branch' and '$new_backport_branch'."
        pull_request_list=$(gh pr list --base="$branch" --head="$new_backport_branch" --json number,title)

        # If a pull request already exists between branch and new_backport_branch, 
        # then skip backporting the pull request changes.
        if [ "$(echo "$pull_request_list" | jq length)" -eq 0 ]; then
            log_info "Starting the backport process for branch: $branch."
            if ! backport_branch "$branch" "$new_backport_branch"; then
                # Continue the backporting process for the remaining branches if a failure occurs on any target branch, 
                # instead of terminating the script.
                log_error "Failed to backport changes to the branch: $branch."
            fi
            log_info "Successfully created the pull request for backporting changes to the branch '$branch'."
        else
            log_info "Pull requests already exist between the branch '$branch' and '$new_backport_branch'. Skipping backport for '$branch'"
        fi
    done

    log_info "Successfully, completed the backporting for the pull request #${PR_NUMBER}."
}

main