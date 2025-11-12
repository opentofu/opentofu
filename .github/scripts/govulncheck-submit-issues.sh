#!/usr/bin/env bash
# SPDX-License-Identifier: MPL-2.0

# This requires a "results" directory containing files with the name of the branch that the results are associated with.
# The files inside need to contain just a one liner JSON array like: ["vulnKey1","vulnKey2",...].
# Eg: `$ tail results/*`
#  ==> results/main <==
#  []
#
#  ==> results/v1.7 <==
#  ["GO-2024-2947","GO-2024-2948","GO-2025-3447","GO-2025-3487","GO-2025-3503"]
#
#  ==> results/v1.8 <==
#  ["GO-2025-3447","GO-2025-3487","GO-2025-3503"]
#
#  ==> results/v1.9 <==
#  ["GO-2025-3447","GO-2025-3487","GO-2025-3503"]

github_run_url="${1}"

# This first part converts the results of type ["vulnKey1","vulnKey2",...] from the files inside the "results" directory
# to {"vulnKey1": ["main", "v1.7", ...], "vulnKey2": ["main", "v1.7", ...]}

# Start the script with an empty json object
vuln_to_versions="{}"
cd results
for version in *; do
  while IFS= read -r vuln;
  do
    [[ -z "${vuln}" ]] && continue
    # Actions from this like:
    # * is giving the shell var for the vulnerability key to jq (--arg vl "${vuln}")
    # * is giving the shell var that contains the scanned version to jq (--arg vers "${version}"
    # * is getting from vuln_to_versions the key name that needs to be a vulnerability key and assigns to its array the currently proceesed version ('.[$vl] = .[$vl] + [$vers]')
    # * this is generating outputs like {"vulnKey": ["version1", "version2"]}. Eg: {"GO-2024-2947":["v1.7"]}
    # * in the end overwrites the content of vuln_to_versions with the newly generated content
    vuln_to_versions="$(echo "${vuln_to_versions}" | jq -c --arg vl "${vuln}" --arg vers "${version}" '.[$vl] = .[$vl] + [$vers]')"
  done <<< "$(cat $version | jq -r '.[]')" # This one is exploding a json array into multiple lines
done

# This second part is just using the ".key" that is the vulnerability key and ".value" that is the list of affected version(s)
# to generate the commands to create GitHub issues.
while IFS= read -r vuln;
do
  vuln_key="$(echo ${vuln} | jq -r '.key')"
  affected_versions="$(echo ${vuln} | jq -r '.value[]' | xargs)"
  ticket_title="${vuln_key} reported"

  reported_issues="$(gh issue -R opentofu/opentofu list --search "\"${ticket_title}\"" --state "all" --json number)"
  no_of_issues="$(echo ${reported_issues} | jq -r '. | length')"
  reported_issues="$(echo $reported_issues| jq -r '.[] | .number' | xargs)"
  [[ ${no_of_issues} -ge 1 ]] && echo "Vulnerabilties found but already reported for ${vuln_key} in: ${reported_issues}" && continue

  echo "--> Creating issue <--"
  echo "This vulnerability might affect the following versions: ${affected_versions}" > ticket_content
  echo "" >> ticket_content
  echo "*Vulnerability info:* https://pkg.go.dev/vuln/${vuln_key}" >> ticket_content
  echo "*Pipeline run:* ${github_run_url}" >> ticket_content
  echo "Create issue..."
  echo "Title: ${ticket_title}"
  echo "Content:"
  cat ticket_content

  gh issue create --repo opentofu/opentofu --label "govulncheck" --title "${ticket_title}" --body-file ticket_content
  echo "--> Creating issue (END) <--"
done <<< "$(echo "$vuln_to_versions"  | jq -c 'to_entries[]')"
# ^ This is converting a json object that looks something like:
# {"GO-2024-2947":["v1.7"],"GO-2024-2948":["v1.7"],...}
# to a list of objects, each on its own line
# {"key":"GO-2024-2947","value":["v1.7"]}
# {"key":"GO-2024-2948","value":["v1.7"]}
# ...
