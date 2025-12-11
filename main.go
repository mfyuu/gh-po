package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/go-gh/v2"
	"github.com/dustin/go-humanize"
	"github.com/mattn/go-runewidth"
)

type PullRequest struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	HeadRefName string    `json:"headRefName"`
	IsDraft     bool      `json:"isDraft"`
	CreatedAt   time.Time `json:"createdAt"`
}

var (
	greenStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellowStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	cyanStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	grayStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	underlineStyle = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("7"))
)

func main() {
	f := parseFlags()

	var prs []PullRequest
	var stderr string
	var listErr error

	_ = spinner.New().
		Title("Loading pull requests...").
		Action(func() {
			prs, stderr, listErr = listPRs()
		}).
		Run()

	if listErr != nil {
		fmt.Fprint(os.Stderr, stderr)
		os.Exit(1)
	}

	if len(prs) == 0 {
		// gh pr list only outputs message in TTY mode, so we print it ourselves
		repo := getRepoName()
		if repo != "" {
			fmt.Printf("no open pull requests in %s\n", repo)
		} else {
			fmt.Println("no open pull requests")
		}
		return
	}

	selected, err := selectPR(prs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// --view: open in browser only (without checkout)
	if f.view {
		if err := browsePR(selected, false); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := checkoutPR(selected); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// --web: open in browser after checkout
	if f.web {
		if err := browsePR(selected, true); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func listPRs() ([]PullRequest, string, error) {
	stdout, stderr, err := gh.Exec("pr", "list", "--json", "number,title,headRefName,isDraft,createdAt")
	if err != nil {
		return nil, stderr.String(), err
	}

	var prs []PullRequest
	if err := json.Unmarshal(stdout.Bytes(), &prs); err != nil {
		return nil, "", fmt.Errorf("failed to parse PR list: %w", err)
	}

	return prs, "", nil
}

func getRepoName() string {
	stdout, _, err := gh.Exec("repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

func selectPR(prs []PullRequest) (PullRequest, error) {
	// Calculate column widths based on display width
	maxIDWidth := 2
	maxTitleWidth := 5
	maxBranchWidth := 6
	maxCreatedWidth := 10 // "CREATED AT"

	for _, pr := range prs {
		idWidth := runewidth.StringWidth(fmt.Sprintf("#%d", pr.Number))
		if idWidth > maxIDWidth {
			maxIDWidth = idWidth
		}
		titleWidth := runewidth.StringWidth(pr.Title)
		if titleWidth > maxTitleWidth {
			maxTitleWidth = titleWidth
		}
		branchWidth := runewidth.StringWidth(pr.HeadRefName)
		if branchWidth > maxBranchWidth {
			maxBranchWidth = branchWidth
		}
		createdWidth := runewidth.StringWidth("about " + humanize.Time(pr.CreatedAt))
		if createdWidth > maxCreatedWidth {
			maxCreatedWidth = createdWidth
		}
	}

	// Limit maximum widths
	if maxTitleWidth > 100 {
		maxTitleWidth = 100
	}
	if maxBranchWidth > 30 {
		maxBranchWidth = 30
	}

	options := make([]huh.Option[PullRequest], len(prs))
	for i, pr := range prs {
		label := formatPR(pr, maxIDWidth, maxTitleWidth, maxBranchWidth, maxCreatedWidth)
		options[i] = huh.NewOption(label, pr)
	}

	// Build header
	header := buildHeader(maxIDWidth, maxTitleWidth, maxBranchWidth, maxCreatedWidth)

	var selected PullRequest
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[PullRequest]().
				Title("Select a PR to checkout").
				Description(header).
				Options(options...).
				Value(&selected),
		),
	)

	if err := form.Run(); err != nil {
		return PullRequest{}, fmt.Errorf("selection cancelled: %w", err)
	}

	return selected, nil
}

func buildHeader(idWidth, titleWidth, branchWidth, createdWidth int) string {
	// Underline each label, no underline for padding
	idLabel := underlineStyle.Render(runewidth.FillRight("ID", idWidth))
	titleLabel := underlineStyle.Render(runewidth.FillRight("TITLE", titleWidth))
	branchLabel := underlineStyle.Render(runewidth.FillRight("BRANCH", branchWidth))
	createdLabel := underlineStyle.Render(runewidth.FillRight("CREATED AT", createdWidth))

	// 2 leading spaces (for cursor) + labels separated by spaces
	return fmt.Sprintf("  %s  %s  %s  %s", idLabel, titleLabel, branchLabel, createdLabel)
}

func styleID(pr PullRequest) string {
	if pr.IsDraft {
		return yellowStyle.Render(fmt.Sprintf("#%d", pr.Number))
	}
	return greenStyle.Render(fmt.Sprintf("#%d", pr.Number))
}

func formatPR(pr PullRequest, idWidth, titleWidth, branchWidth, createdWidth int) string {
	// ID (colored)
	idStr := fmt.Sprintf("#%d", pr.Number)
	paddedID := runewidth.FillRight(idStr, idWidth)
	var styledID string
	if pr.IsDraft {
		styledID = yellowStyle.Render(paddedID)
	} else {
		styledID = greenStyle.Render(paddedID)
	}

	// Title (truncate & pad)
	title := pr.Title
	if runewidth.StringWidth(title) > titleWidth {
		title = runewidth.Truncate(title, titleWidth-1, "…")
	}
	paddedTitle := runewidth.FillRight(title, titleWidth)

	// Branch (colored, truncate & pad)
	branch := pr.HeadRefName
	if runewidth.StringWidth(branch) > branchWidth {
		branch = runewidth.Truncate(branch, branchWidth-1, "…")
	}
	paddedBranch := runewidth.FillRight(branch, branchWidth)
	styledBranch := cyanStyle.Render(paddedBranch)

	// Relative time (add "about" prefix & pad)
	created := "about " + humanize.Time(pr.CreatedAt)
	paddedCreated := runewidth.FillRight(created, createdWidth)

	return fmt.Sprintf("%s  %s  %s  %s", styledID, paddedTitle, styledBranch, grayStyle.Render(paddedCreated))
}

func checkoutPR(pr PullRequest) error {
	// Display selected PR info
	styledBranch := cyanStyle.Render(pr.HeadRefName)
	fmt.Printf("%s  %s  %s\n\n", styleID(pr), pr.Title, styledBranch)

	var stdoutStr, stderrStr string
	var execErr error

	_ = spinner.New().
		Title("Checking out PR...").
		Action(func() {
			stdout, stderr, err := gh.Exec("pr", "checkout", strconv.Itoa(pr.Number))
			stdoutStr = stdout.String()
			stderrStr = stderr.String()
			execErr = err
		}).
		Run()

	if stdoutStr != "" {
		fmt.Print(stdoutStr)
	}
	if stderrStr != "" {
		fmt.Print(stderrStr)
	}
	if execErr != nil {
		return fmt.Errorf("failed to checkout PR #%d: %w", pr.Number, execErr)
	}

	return nil
}

type flags struct {
	web  bool
	view bool
}

func parseFlags() flags {
	flag.Usage = func() {
		fmt.Print(`Interactively select and checkout a pull request.
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
`)
	}

	var f flags
	flag.BoolVar(&f.web, "web", false, "")
	flag.BoolVar(&f.web, "w", false, "")
	flag.BoolVar(&f.view, "view", false, "")
	flag.BoolVar(&f.view, "v", false, "")
	flag.Parse()
	return f
}

func browsePR(pr PullRequest, withNewline bool) error {
	if withNewline {
		fmt.Println()
	}
	cmd := exec.Command("gh", "browse", strconv.Itoa(pr.Number))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open PR #%d in browser: %w", pr.Number, err)
	}
	return nil
}
