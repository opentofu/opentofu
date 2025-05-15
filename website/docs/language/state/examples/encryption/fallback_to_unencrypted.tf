terraform {
  encryption {
    ## Step 1: Leave the original encryption method unchanged:
    method "some_method_type" "old_method_name" {
      ## Parameters for the old method here.
    }

    # Step 2: Add the unencrypted method here:
    method "unencrypted" "migrate" {}

    state {
      ## Step 3: Disable or remove the "enforced" option:
      enforced = false

      ## Step 4: Move the original encryption method into the "fallback" block:
      fallback {
        method = method.some_method_type.old_method_name
      }

      ## Step 5: Reference the unencrypted method as your primary "encryption" method.
      method = method.unencrypted.migrate
    }

    ## Step 6: Run "tofu apply".

    ## Step 7: Remove the "state" block once the migration is complete.

    ## Step 8: Repeat steps 3-7 for plan{} if needed.
  }
}
