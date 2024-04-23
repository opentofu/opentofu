#!/bin/bash

GITHUB_TOKEN=$GITHUB_TOKEN

git config --global user.email "noreply@github.com"
git config --global user.name "GitHub Actions"

git config --global credential.helper "store --file=~/.git-credentials"
git config --global credential.https://github.com.username $GITHUB_TOKEN

echo "Successfully authenticated!"

git checkout $GITHUB_EVENT_BRANCH
echo "Checked out main"

# Get backport label from all the labels of the PR
labels=$(jq --raw-output '.pull_request.labels[].name' "$GITHUB_EVENT_PATH")
backport_label=""
branch_name=""
for label in "${labels[@]}"; do
    echo "each label - $label"
    if [[ $label == "backport"* ]]; then
        backport_label=$label
        branch_name=${label#backport }
        break
    fi
done
echo "Label Name: $backport_label"

# Checkout the version branch
git fetch --all
git checkout $branch_name
echo "Version branch: $branch_name"

# Checkout new backport branch
ISSUE_NUMBER=$(jq --raw-output .pull_request.number "$GITHUB_EVENT_PATH")
newBranch="backport/$ISSUE_NUMBER"
git checkout -b $newBranch
git push origin $newBranch
echo "New backport branch: $newBranch"

# Get pull request and repository info.
pullRequestNumber=$(jq --raw-output .pull_request.number "$GITHUB_EVENT_PATH")
owner=$(jq -r '.repository.owner.login' "$GITHUB_EVENT_PATH")
repo=$(jq -r '.repository.name' "$GITHUB_EVENT_PATH")
echo "Pull Request Number: $pullRequestNumber"
echo "Owner: $owner"
echo "Repo: $repo"

# Get the commit SHAs associated with the pull request
commit_ids=$(curl -s -H "Authorization: token $GITHUB_TOKEN" \
     -H "Accept: application/vnd.github.v3+json" \
     "https://api.github.com/repos/$owner/$repo/pulls/$pullRequestNumber/commits" | \
     jq -r '.[].sha')

# Store the commit SHAs in an array
commit_ids_array=($commit_ids)
for sha in "${commit_ids_array[@]}"; do
    echo "Commit SHA: $sha"
done

# Count the number of commits
commitCount=$(echo "$commitIDs" | wc -l)
echo "Number of commits: $commitCount"

# Cherry pick commits
for commitID in "${commit_ids_array[@]}"; do
    git fetch origin $commitID
    git cherry-pick $commitID
    echo "Cherry-picking commit: $commitID"
done
echo "Cherry Pick done!"

git commit -am "Backport PR #$ISSUE_NUMBER"
echo "Successfully committed!"

git push origin $newBranch
echo "Successfully pushed!"

head_branch=$(jq -r '.pull_request.head.ref' "$GITHUB_EVENT_PATH")
echo "Head branch name: $head_branch"

gh auth login --with-token $GITHUB_TOKEN
gh pr create \
  -B "$branch_name" \
  -H "$newBranch" \
  -t "Backport PR #$ISSUE_NUMBER" \
  -b "This pull request backports changes from $head_branch to $branch_name"

echo "PR created successfully!"