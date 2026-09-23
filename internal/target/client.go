package target

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jbonadiman/mirror-to-org/internal/httperr"
	"github.com/jbonadiman/mirror-to-org/internal/profile"
	"github.com/jbonadiman/mirror-to-org/internal/reference"
)

// Migration clones the repo before responding, unlike the web UI which queues it.
const migrationTimeout = 30 * time.Minute
const maxAvatarBytes = 10 * 1024 * 1024

type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "token "+t.token)
	return t.base.RoundTrip(req)
}

// refuseForeignRedirect keeps the target token on the target host. Go's
// default client drops Authorization when a redirect changes host, but the
// authTransport sets it on every hop, so a redirect would otherwise hand the
// token to whatever host the target names. Refuse loudly instead.
func refuseForeignRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("refusing redirect to %s", req.URL)
	}
	return nil
}

func NewClient(token string) *http.Client {
	return &http.Client{
		Transport:     &authTransport{token: token, base: http.DefaultTransport},
		CheckRedirect: refuseForeignRedirect,
	}
}

func doRequest(client *http.Client, method, url string, body []byte, headers map[string]string) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return respBody, resp.StatusCode, nil
}

// Probe decides whether to create, update, or skip this name on the target.
func Probe(client *http.Client, targetURL, name string) (string, error) {
	body, status, err := doRequest(client, http.MethodGet, targetURL+"/api/v1/orgs/"+name, nil, nil)
	if err != nil {
		return "", err
	}
	if status != 200 && status != 404 {
		if err := httperr.CheckStatus(status, body, fmt.Sprintf("probing org '%s'", name), true); err != nil {
			return "", err
		}
	}
	if status == 200 {
		return DecideAction(200, 0), nil
	}

	userBody, userStatus, err := doRequest(client, http.MethodGet, targetURL+"/api/v1/users/"+name, nil, nil)
	if err != nil {
		return "", err
	}
	if userStatus != 200 && userStatus != 404 {
		if err := httperr.CheckStatus(userStatus, userBody, fmt.Sprintf("probing user '%s'", name), true); err != nil {
			return "", err
		}
	}
	return DecideAction(404, userStatus), nil
}

func CreateOrg(client *http.Client, targetURL string, body OrgCreatePayload) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	respBody, status, err := doRequest(client, http.MethodPost, targetURL+"/api/v1/orgs", payload, nil)
	if err != nil {
		return err
	}
	return httperr.CheckStatus(status, respBody, "creating org", true)
}

func RepoExists(client *http.Client, targetURL, owner, repo string) (bool, error) {
	body, status, err := doRequest(client, http.MethodGet, fmt.Sprintf("%s/api/v1/repos/%s/%s", targetURL, owner, repo), nil, nil)
	if err != nil {
		return false, err
	}
	if status == 200 || status == 404 {
		return status == 200, nil
	}
	if err := httperr.CheckStatus(status, body, fmt.Sprintf("probing repo '%s/%s'", owner, repo), true); err != nil {
		return false, err
	}
	return false, nil
}

// 409 is the backstop for Probe: races, and anything the probe missed.
func MigrateRepo(client *http.Client, targetURL string, body MigrationPayload) (string, error) {
	name := body.RepoOwner + "/" + body.RepoName
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	// Shallow copy: same Transport (auth still applies), own Timeout.
	longTimeout := *client
	longTimeout.Timeout = migrationTimeout

	respBody, status, err := doRequest(&longTimeout, http.MethodPost, targetURL+"/api/v1/repos/migrate", payload, nil)
	if err != nil {
		return "", err
	}
	if status >= 200 && status < 300 {
		return "ok", nil
	}
	if status == 409 {
		if strings.Contains(strings.ToLower(string(respBody)), "files already exist") {
			return "", fmt.Errorf(
				"migrating '%s' failed (409): files from a failed migration remain on the target; remove them there before retrying", name)
		}
		return "skip", nil
	}
	if err := httperr.CheckStatus(status, respBody, fmt.Sprintf("migrating '%s'", name), true); err != nil {
		return "", err
	}
	return "ok", nil
}

func UploadAvatar(client, source *http.Client, targetURL, name, avatarURL string) error {
	u, err := url.Parse(avatarURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("refusing non-https avatar URL: %s", avatarURL)
	}
	if err := reference.ValidateHost(strings.ToLower(u.Host)); err != nil {
		return fmt.Errorf("refusing avatar URL host: %w", err)
	}

	// The avatar is fetched from the source, so it follows the same
	// redirect rule as a profile fetch: a source cannot bounce it onto an
	// internal address.
	fetch := *source
	fetch.CheckRedirect = profile.RefuseUnsafeRedirect
	image, status, err := doRequest(&fetch, http.MethodGet, avatarURL, nil, map[string]string{"User-Agent": "mirror-to-org"})
	if err != nil {
		return err
	}
	if err := httperr.CheckStatus(status, image, "downloading avatar", false); err != nil {
		return err
	}
	if len(image) > maxAvatarBytes {
		return fmt.Errorf("avatar exceeds %d bytes", maxAvatarBytes)
	}

	encoded := base64.StdEncoding.EncodeToString(image)
	payload, err := json.Marshal(map[string]string{"image": encoded})
	if err != nil {
		return err
	}
	respBody, status, err := doRequest(client, http.MethodPost, fmt.Sprintf("%s/api/v1/orgs/%s/avatar", targetURL, name), payload, nil)
	if err != nil {
		return err
	}
	return httperr.CheckStatus(status, respBody, fmt.Sprintf("uploading avatar for '%s'", name), true)
}
