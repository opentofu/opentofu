#!/bin/bash

set -e

# Setting up Committer identity.
git config --global user.email "noreply@github.com"
git config --global user.name "GitHub Actions"

# Get backport label from all the labels of the PR
backport_label=""
branch_name=""
for label in "${LABELS[@]}"; do
    echo "each label - $label"
    if [[ $label == "backport"* ]]; then
        backport_label=$label
        branch_name=${label#backport }
        break
    fi
done

# Exit if backport label not found
echo "Label Name: $backport_label"
if [ -z "$backport_label" ]; then
    echo "Backport label not found. Exiting."
    exit 1
fi

# Checkout the version branch
git fetch origin "$branch_name"
git checkout "$branch_name"
echo "Version branch: $branch_name"

# Checkout new backport branch
new_branch="backport/$ISSUE_NUMBER"
git checkout -b "$new_branch"
git push origin "$new_branch"
echo "New backport branch: $new_branch"

# Get the commit SHAs associated with the pull request
curl -sS -H "Authorization: token $GITHUB_TOKEN" \
     -H "Accept: application/vnd.github.v3+json" \
     "https://api.github.com/repos/$OWNER/$REPO/pulls/$PR_NUMBER/commits" | \
jq -r '.[].sha' |
while IFS= read -r commit_sha; do
    git fetch origin "$commit_sha"
    if git cherry-pick "$commit_sha" ; then
        echo "Cherry-picking commit: $commit_sha"
    else
        # Add a failure comment to the pull request
        echo "Error: Failed to cherry-pick commit $commit_sha"
        failureComment="Cherry-picking commit $commit_sha failed during the backport process to $branch_name."
        curl -sS -X POST -H "Authorization: token $GITHUB_TOKEN" \
          -d "{\"body\":\"$failureComment\"}" \
          "https://api.github.com/repos/$GITHUB_REPOSITORY/issues/$PR_NUMBER/comments" >/dev/null
        exit 1
    fi
done
echo "Cherry Pick done!"

git push origin "$new_branch"
echo "Successfully pushed!"

# Create the pull request.
title="Backport PR #$ISSUE_NUMBER"
body="This pull request backports changes from $HEAD_BRANCH to $branch_name."
url=$(curl -sS -X POST -H "Authorization: token $GITHUB_TOKEN" \
  -d "{\"title\":\"$title\",\"body\":\"$body\",\"head\":\"$new_branch\",\"base\":\"$branch_name\"}" \
  "https://api.github.com/repos/$GITHUB_REPOSITORY/pulls" | jq -r '.html_url')

echo "PR created successfully!"

# Add comment to the original PR.
comment="This pull request has been successfully backported. Link to the new pull request: $url"
curl -sS -X POST -H "Authorization: token $GITHUB_TOKEN" \
  -d "{\"body\":\"$comment\"}" \
  "https://api.github.com/repos/$GITHUB_REPOSITORY/issues/$PR_NUMBER/comments" >/dev/null

echo "Added comment to the original PR."