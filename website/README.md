# OpenTF Documentation

This directory contains the portions of [the OpenTF website](https://placeholderplaceholderplaceholder.io/) that pertain to the core functionality, excluding providers and the overall configuration.

## Suggesting Changes

You can [submit an issue](https://github.com/opentffoundation/opentf/issues/new/choose) with documentation requests or submit a pull request with suggested changes.

Click **Edit this page** at the bottom of any OpenTF website page to go directly to the associated markdown file in GitHub.

## Validating Content

Content changes are automatically validated against a set of rules as part of the pull request process. If you want to run these checks locally to validate your content before committing your changes, you can run the following command:

```
npm run content-check
```

If the validation fails, actionable error messages will be displayed to help you address detected issues.

## Modifying Sidebar Navigation

You must update the sidebar navigation when you add or delete documentation .mdx files. If you do not update the navigation, the website deploy preview fails.

To update the sidebar navigation, you must edit the appropriate `nav-data.json` file. This repository contains the sidebar navigation files for the following documentation sets:

- OpenTF Language: [`language-nav-data.json`](https://github.com/opentffoundation/opentf/blob/main/website/data/language-nav-data.json)
- OpenTF CLI: [`cli-nav-data.json`](https://github.com/opentffoundation/opentf/blob/main/website/data/cli-nav-data.json)
- Introduction to OpenTF: [`intro-nav-data.json`](https://github.com/opentffoundation/opentf/blob/main/website/data/intro-nav-data.json)

## Previewing Changes

Coming soon: Documenting the development process for the documentation website repo.

## Deploying Changes

Coming soon: Documenting the deployment process for the documentation website repo.
