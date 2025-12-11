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
		// gh pr listはTTYモードでのみメッセージを出力するため、自前で出力
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

	// --view: ブラウザのみ（checkout しない）
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

	// --web: checkout 後にブラウザも開く
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
	// カラム幅を計算（表示幅ベース）
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

	// 最大幅を制限
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

	// ヘッダーを構築
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
	// 各ラベルにアンダーライン、余白にはアンダーラインなし
	idLabel := underlineStyle.Render(runewidth.FillRight("ID", idWidth))
	titleLabel := underlineStyle.Render(runewidth.FillRight("TITLE", titleWidth))
	branchLabel := underlineStyle.Render(runewidth.FillRight("BRANCH", branchWidth))
	createdLabel := underlineStyle.Render(runewidth.FillRight("CREATED AT", createdWidth))

	// 先頭2スペース（カーソル分）+ 各ラベルを余白で連結
	return fmt.Sprintf("  %s  %s  %s  %s", idLabel, titleLabel, branchLabel, createdLabel)
}

func styleID(pr PullRequest) string {
	if pr.IsDraft {
		return yellowStyle.Render(fmt.Sprintf("#%d", pr.Number))
	}
	return greenStyle.Render(fmt.Sprintf("#%d", pr.Number))
}

func formatPR(pr PullRequest, idWidth, titleWidth, branchWidth, createdWidth int) string {
	// ID (色付き)
	idStr := fmt.Sprintf("#%d", pr.Number)
	paddedID := runewidth.FillRight(idStr, idWidth)
	var styledID string
	if pr.IsDraft {
		styledID = yellowStyle.Render(paddedID)
	} else {
		styledID = greenStyle.Render(paddedID)
	}

	// タイトル（切り詰め & パディング）
	title := pr.Title
	if runewidth.StringWidth(title) > titleWidth {
		title = runewidth.Truncate(title, titleWidth-1, "…")
	}
	paddedTitle := runewidth.FillRight(title, titleWidth)

	// ブランチ（色付き、切り詰め & パディング）
	branch := pr.HeadRefName
	if runewidth.StringWidth(branch) > branchWidth {
		branch = runewidth.Truncate(branch, branchWidth-1, "…")
	}
	paddedBranch := runewidth.FillRight(branch, branchWidth)
	styledBranch := cyanStyle.Render(paddedBranch)

	// 相対時間（about を追加 & パディング）
	created := "about " + humanize.Time(pr.CreatedAt)
	paddedCreated := runewidth.FillRight(created, createdWidth)

	return fmt.Sprintf("%s  %s  %s  %s", styledID, paddedTitle, styledBranch, grayStyle.Render(paddedCreated))
}

func checkoutPR(pr PullRequest) error {
	// 選択されたPR情報を表示
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
	var f flags
	flag.BoolVar(&f.web, "web", false, "Open the PR in browser after checkout")
	flag.BoolVar(&f.web, "w", false, "Open the PR in browser after checkout (shorthand)")
	flag.BoolVar(&f.view, "view", false, "Open the PR in browser without checkout")
	flag.BoolVar(&f.view, "v", false, "Open the PR in browser without checkout (shorthand)")
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
