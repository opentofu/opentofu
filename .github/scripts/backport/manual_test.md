# Manual Test Cases #

## Successful Test Cases ##

**Test Case 1**: Valid backport pull request with single backport label.

**Objective**: Verify that the script backports a valid pull request without issues after adding a backport label.

**Steps**:
- Add a backport label to your pull request.
    - Format backport *<target_branch>*, where *<target_branch>* is the branch you want to backport changes to (eg. backport v1.x).
- Ensure that the pull request is fully approved and merged into the main branch.

**Expected Results**:
- The `Backport Merged Pull Request` workflow initiates automatically upon the addition of the backport label.
- The script *backport/main.sh* begins backporting the changes from the labeled pull request to the specified target branch (*<target_branch>*). 
- Info logs confirming the successful backporting process are displayed, indicating the completion of backporting to *<target_branch>*.
    - For eg: Successfully backported the pull request changes to the branch *<target_branch>*.
- The `Backport Merged Pull Request` workflow completes without encountering any errors.
- A new pull request is created successfully, incorporating only the changes from the labeled pull request that need to be backported.
    - Format: Backport PR *#<pull_request_number>*: *<source_branch>* to *<target_branch>*.
- A comment is added to the original pull request, indicating the completion of the backporting process.

---

**Test Case 2**: Valid backport pull request with multiple backport label.

**Objective**: Verify that the script backports a valid pull request without issues after adding multiple backport labels.

**Steps**:
- Add multiple backport labels to your pull request.
    - Format backport *<target_branch>*, where *<target_branch>* is the branch you want to backport changes to (eg. backport v1.x)
- Ensure that the pull request is fully approved and merged into the main branch.

**Expected Results**:
- The `Backport Merged Pull Request` workflow initiates automatically upon the addition of the backport label.
- The script *backport/main.sh* begins backporting the changes from the labeled pull request to the specified target branch (*<target_branch>*). 
- Info log about the successful backporting to each *<target_branch>* can be found.
    - For eg: Successfully backported the pull request changes to the branch *<target_branch>*.
- Info logs confirming the successful backporting process to all target branches are displayed, indicating the completion of backporting to *<target_branch>*.
Successfully, completed the backporting for the pull request *#<pull_request_number>*.
- The `Backport Merged Pull Request` workflow completes without encountering any errors.
- A new pull request is created successfully for all the target branches, incorporating only the changes from the labeled pull request that need to be backported.
    - Format: Backport PR *#<pull_request_number>*: *<source_branch>* to *<target_branch>*.
- A comment is added to the original pull request, indicating the completion of the backporting process.

---

**Test Case 3**: No backport labels provided. 

**Objective**: Verify that the script doesn’t fail when no backport label is provided. 

**Steps**:
- Ensure that the pull request is fully approved and merged into the main branch without adding any backport labels.
- Backport labels should have a `backport` prefix followed by the *<target_branch>*.

**Expected Results**:
- The `Backport Merged Pull Request` workflow initiates automatically upon the addition of the backport label.
- `Run custom bash script for backporting` step should be skipped in the workflow.
- The script */backport/main.sh* should be skipped.
- The `Backport Merged Pull Request` workflow completes without encountering any errors.

---

**Test Case 4**: Pull request already exists.

**Objective**: Verify that the script successfully shows the info log that pull request already exists between new backport branch and target branch where original pull request changes will be backported to. 

**Steps**:
- The pull request should already exists between target branch and new backport branch.
- Add a backport label to your original pull request.
    - Format backport *<target_branch>*, where *<target_branch>* is the branch you want to backport changes to (eg. backport v1.x).
- Ensure that the pull request is fully approved and merged into the main branch.

**Expected Results**:
- The `Backport Merged Pull Request` workflow initiates automatically upon the addition of the backport label.
- The script *backport/main.sh* begins backporting the changes from the labeled pull request to the specified target branch (*<target_branch>*). 
- Info log mentioning details about skipping backport process can be found.
    - For eg: Pull requests already exist between the branch *<source_branch>* and *<target_branch>*. Skipping backport for *<target_branch>*.
- The `Backport Merged Pull Request` workflow completes without encountering any errors.


## Failure Test Cases ##

**Test Case 1**: Merge conflict while cherry picking.

**Objective**: Verify the script add a comment to the original pull request when merge conflict happens while cherry picking.

**Steps**:
- Add multiple backport labels to your pull request.
    - Format backport *<target_branch>*, where *<target_branch>* is the branch you want to backport changes to (eg. backport v1.x)
- Ensure that the pull request is fully approved and merged into the main branch.

**Error log**: Failed to cherry-pick commit <commit_id>. Added failure comment to the pull request.

**Expected Results**:
- The `Backport Merged Pull Request` workflow initiates automatically upon the addition of the backport label.
- The script *backport/main.sh* begins backporting the changes from the labeled pull request to the specified target branch (*<target_branch>*). 
- Error logs detailing the backport failure are generated.
- Backport Merged Pull Request workflow failed with errors.
- A comment is added to the original pull request, indicating the completion of the backporting process.
    - Error: Cherry-pick commit <commit_id> failed during the backport process to *<target_branch>*. Please resolve any conflicts and manually apply the changes.

---

**Test Case 2**: Unable to fetch commit id. 

**Objective**: Verify that the script failed when commit id was not successfully fetched and proper error log should be generated to identify the issue.

**Steps**:
- Add multiple backport labels to your pull request.
    - Format backport *<target_branch>*, where *<target_branch>* is the branch you want to backport changes to (eg. backport v1.x).
- Ensure that the pull request is fully approved and merged into the main branch.

**Error log**: Failed to fetch the commit id: '<commit_id>.

**Expected Results**:
- The `Backport Merged Pull Request` workflow initiates automatically upon the addition of the backport label.
- The script *backport/main.sh* begins backporting the changes from the labeled pull request to the specified target branch (*<target_branch>*). 
- Error log mentioning details about backport failure can be found.
- Backport Merged Pull Request workflow failed with errors.
- No comments will be added.

---

**Test Case 3**: Unable to fetch target branch.

**Objective**: Verify that the script failed when the target branch was not found.

**Steps**:
- Add a backport label to your pull request.
    - Format backport *<target_branch>*, where *<target_branch>* is the branch you want to backport changes to (eg. backport v1.x)
- Ensure that the pull request is fully approved and merged into the main branch.

**Error log**: Failed to fetch branch: '*<target_branch>*’.

**Expected Results**:
- The `Backport Merged Pull Request` workflow initiates automatically upon the addition of the backport label.
- The script *backport/main.sh* begins backporting the changes from the labeled pull request to the specified target branch (*<target_branch>*). 
- Error log mentioning details about backport failure can be found.
- Backport Merged Pull Request workflow failed with errors.
- No comments will be added.

---

**Test Case 4**: Failed to create the pull request.

**Objective**: Verify that the script failed when the pull request was not created.

**Steps**:
- Add a backport label to your pull request.
    - Format backport *<target_branch>*, where *<target_branch>* is the branch you want to backport changes to (eg. backport v1.x)
- Ensure that the pull request is fully approved and merged into the main branch.

**Error log**: Failed to create the pull request.

**Expected Results**:
- The `Backport Merged Pull Request` workflow initiates automatically upon the addition of the backport label.
- The script *backport/main.sh* begins backporting the changes from the labeled pull request to the specified target branch (*<target_branch>*). 
- Error log mentioning details about backport failure can be found.
- Backport Merged Pull Request workflow failed with errors.
- No comments will be added.

---

**Test Case 5**: Failed to add comment to the original pull request.

**Objective**: Verify that the script failed when the comment was not added to the original pull request.

**Steps**:
- Add a backport label to your pull request.
    - Format backport *<target_branch>*, where *<target_branch>* is the branch you want to backport changes to (eg. backport v1.x)
- Ensure that the pull request is fully approved and merged into the main branch.

**Error log**: Failed to add a comment to the original pull request *#<pull_request_number>* created between *<target_branch>* and <backport_branch>.

**Expected Results**:
- The `Backport Merged Pull Request` workflow initiates automatically upon the addition of the backport label.
- The script *backport/main.sh* begins backporting the changes from the labeled pull request to the specified target branch (*<target_branch>*). 
- Error log mentioning details about backport failure can be found.
- Backport Merged Pull Request workflow failed with errors.
- No comments will be added.
