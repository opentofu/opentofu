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
echo "Label Name: $backport_label"

# Checkout the version branch
git fetch origin $branch_name
git checkout $branch_name
echo "Version branch: $branch_name"

# Checkout new backport branch
new_branch="backport/$ISSUE_NUMBER"
git checkout -b $new_branch
git push origin $new_branch
echo "New backport branch: $new_branch"

# Get the commit SHAs associated with the pull request
commit_ids=$(curl -sS -H "Authorization: token $GITHUB_TOKEN" \
     -H "Accept: application/vnd.github.v3+json" \
     "https://api.github.com/repos/$OWNER/$REPO/pulls/$PR_NUMBER/commits" | \
     jq -r '.[].sha')

# Store the commit SHAs in an array
commit_ids_array=($commit_ids)
for commit_sha in "${commit_ids_array[@]}"; do
    git fetch origin $commit_sha
    git cherry-pick $commit_sha
    echo "Cherry-picking commit: $commit_sha"
done
echo "Cherry Pick done!"

git push origin $new_branch
echo "Successfully pushed!"

# Create the pull request.
title="Backport PR #$ISSUE_NUMBER"
body="This pull request backports changes from $head_branch to $branch_name."
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