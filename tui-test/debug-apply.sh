#!/bin/bash
# Enable debug logging to a file
export TF_LOG=DEBUG
export TF_LOG_PATH=tui-debug.log

# Clear the log file
> tui-debug.log

# Run tofu apply with TUI
echo "Running tofu apply with TUI and debug logging..."
../tofu apply -tui -auto-approve

echo -e "\n\nRelevant debug logs:"
grep -E "\[DEBUG\].*TUI|\[DEBUG\].*Terminal|\[ERROR\]" tui-debug.log | head -50