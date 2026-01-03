// Package action handles GitHub Actions-specific functionality.
//
// Purpose: Post PR comments, write Step Summaries, interact with GitHub API.
// Public API: PostComment, WriteStepSummary, GetPRNumber
// Usage: Use PostComment to upsert a PR comment with a marker for idempotency.
package action

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const commentMarker = "<!-- plarix-scan -->"

// PRInfo holds GitHub PR context from environment.
type PRInfo struct {
	Owner  string
	Repo   string
	Number int
	Token  string
	APIURL string
}

// GetPRInfo extracts PR information from GitHub Actions environment.
// Returns nil if not in a PR context.
func GetPRInfo() *PRInfo {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil
	}

	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		return nil
	}

	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil
	}

	// Get PR number from event
	prNumber := getPRNumber()
	if prNumber == 0 {
		return nil
	}

	apiURL := os.Getenv("GITHUB_API_URL")
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}

	return &PRInfo{
		Owner:  parts[0],
		Repo:   parts[1],
		Number: prNumber,
		Token:  token,
		APIURL: apiURL,
	}
}

// getPRNumber extracts PR number from GitHub event context.
func getPRNumber() int {
	// Try GITHUB_REF_NAME for pull_request events
	refName := os.Getenv("GITHUB_REF_NAME")
	if strings.Contains(refName, "/merge") {
		// Format: <pr_number>/merge
		parts := strings.Split(refName, "/")
		if len(parts) > 0 {
			if n, err := strconv.Atoi(parts[0]); err == nil {
				return n
			}
		}
	}

	// Try parsing event payload
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return 0
	}

	data, err := os.ReadFile(eventPath)
	if err != nil {
		return 0
	}

	var event struct {
		PullRequest struct {
			Number int `json:"number"`
		} `json:"pull_request"`
		Issue struct {
			Number int `json:"number"`
		} `json:"issue"`
		Number int `json:"number"`
	}

	if err := json.Unmarshal(data, &event); err != nil {
		return 0
	}

	if event.PullRequest.Number > 0 {
		return event.PullRequest.Number
	}
	if event.Issue.Number > 0 {
		return event.Issue.Number
	}
	return event.Number
}

// PostComment creates or updates a PR comment with the given content.
// Uses the marker to find and update existing comments (idempotent).
func PostComment(pr *PRInfo, content string) error {
	content = commentMarker + "\n" + content

	// Find existing comment with marker
	existingID, err := findExistingComment(pr)
	if err != nil {
		return fmt.Errorf("find existing comment: %w", err)
	}

	if existingID > 0 {
		return updateComment(pr, existingID, content)
	}
	return createComment(pr, content)
}

// findExistingComment looks for a comment with our marker.
func findExistingComment(pr *PRInfo) (int64, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments",
		pr.APIURL, pr.Owner, pr.Repo, pr.Number)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+pr.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("list comments failed: %s - %s", resp.Status, string(body))
	}

	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return 0, err
	}

	markerRegex := regexp.MustCompile(`<!--\s*plarix-scan\s*-->`)
	for _, c := range comments {
		if markerRegex.MatchString(c.Body) {
			return c.ID, nil
		}
	}

	return 0, nil
}

// createComment creates a new PR comment.
func createComment(pr *PRInfo, content string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments",
		pr.APIURL, pr.Owner, pr.Repo, pr.Number)

	body, _ := json.Marshal(map[string]string{"body": content})
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+pr.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create comment failed: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// updateComment updates an existing PR comment.
func updateComment(pr *PRInfo, commentID int64, content string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d",
		pr.APIURL, pr.Owner, pr.Repo, commentID)

	body, _ := json.Marshal(map[string]string{"body": content})
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+pr.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update comment failed: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// WriteStepSummary writes content to GitHub Step Summary.
func WriteStepSummary(content string) error {
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath == "" {
		return nil // Not in GitHub Actions
	}

	f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	_, err = f.WriteString(content)
	return err
}
