A lightweight GitHub CLI extension that provides an interactive PR selector for seamless checkout and browser viewing.

![demo](https://github.com/user-attachments/assets/e7584db8-da33-4be6-b113-8aeff6b9bdcb)

## Overview

`gh-po` = `gh pr list` + `gh pr checkout`, optionally `gh pr view --web`

Streamlines the process of viewing and checking out pull requests. It displays all available pull requests in an interactive selection UI, allowing you to quickly checkout any PR or open it in your browser.

Built with [golang/go](https://github.com/golang/go), this extension uses [charmbracelet/huh](https://github.com/charmbracelet/huh) for interactive selection.

## Motivation

When working with multiple pull requests, developers often need to:

1. Run `gh pr list` to see available PRs
2. Identify the PR number they want to work on
3. Run `gh pr checkout <number>` to switch to that PR
4. (Optionally) Run `gh pr view --web` to open the PR in browser

This extension combines these steps into a single command with an interactive interface, reducing context switching and making PR management more efficient. The arrow-key navigation makes it easy to quickly jump between different pull requests during code reviews or when switching between multiple work streams.

## Installation

### Prerequisites

- [GitHub CLI](https://cli.github.com/) must be installed and authenticated

### Install as a GitHub CLI extension

```bash
gh extensions install mfyuu/gh-po
```

## Usage

```
gh po --help

# Output:
Interactively select and checkout a pull request.
Optionally open the PR in the browser.

USAGE
  gh po [flags]

FLAGS
  -w, --web     Open the PR in browser after checkout
  -v, --view    Open the PR in browser without checkout
  --help        Show help for command

EXAMPLES
  $ gh po              # Checkout only
  $ gh po --web        # Checkout and open in browser
  $ gh po --view       # Open in browser without checkout
```

### Modes

- **Default (`gh po`)**: Interactively select a PR and checkout the branch
- **Web (`gh po --web` or `gh po -w`)**: Checkout the PR and open it in your browser
- **View (`gh po --view` or `gh po -v`)**: Open the PR in your browser without checking out
