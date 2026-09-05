package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jbonadiman/mirror-to-org/internal/httperr"
)

const (
	userAgent = "mirror-to-org"
	githubAPI = "https://api.github.com"
)

var ErrNotFound = errors.New("no such profile")

func SourceURL(host, account string) string {
	return fmt.Sprintf("https://%s/%s", host, account)
}

func doGet(client *http.Client, rawURL string, headers map[string]string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}

func Fetch(source *http.Client, host, account string) ([]byte, error) {
	if GitlabHosts[host] {
		return fetchGitlab(source, host, account)
	}

	var apiURL string
	headers := map[string]string{
		"Accept":     "application/json",
		"User-Agent": userAgent,
	}
	if host == GithubHost {
		apiURL = fmt.Sprintf("%s/users/%s", githubAPI, account)
		headers["Accept"] = "application/vnd.github+json"
		headers["X-GitHub-Api-Version"] = "2022-11-28"
	} else {
		apiURL = fmt.Sprintf("https://%s/api/v1/users/%s", host, account)
	}

	body, status, err := doGet(source, apiURL, headers)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, host, account)
	}
	if err := httperr.CheckStatus(status, body, fmt.Sprintf("fetching %s/%s", host, account), false); err != nil {
		return nil, err
	}
	return body, nil
}

func fetchGitlab(source *http.Client, host, account string) ([]byte, error) {
	headers := map[string]string{
		"Accept":     "application/json",
		"User-Agent": userAgent,
	}

	usersURL := fmt.Sprintf("https://%s/api/v4/users?username=%s", host, url.QueryEscape(account))
	body, status, err := doGet(source, usersURL, headers)
	if err != nil {
		return nil, err
	}
	if err := httperr.CheckStatus(status, body, fmt.Sprintf("fetching %s/%s", host, account), false); err != nil {
		return nil, err
	}

	var matches []json.RawMessage
	if err := json.Unmarshal(body, &matches); err != nil {
		return nil, err
	}
	if len(matches) > 0 {
		return matches[0], nil
	}

	groupURL := fmt.Sprintf("https://%s/api/v4/groups/%s", host, account)
	body, status, err = doGet(source, groupURL, headers)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, host, account)
	}
	if err := httperr.CheckStatus(status, body, fmt.Sprintf("fetching group %s/%s", host, account), false); err != nil {
		return nil, err
	}
	return body, nil
}
