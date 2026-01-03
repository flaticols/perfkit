package capture

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/flaticols/perfkit/internal/models"
)

// ProfileEndpoint maps profile types to pprof endpoints
var ProfileEndpoint = map[models.ProfileType]string{
	models.ProfileTypeCPU:          "/debug/pprof/profile",
	models.ProfileTypeHeap:         "/debug/pprof/heap",
	models.ProfileTypeGoroutine:    "/debug/pprof/goroutine",
	models.ProfileTypeBlock:        "/debug/pprof/block",
	models.ProfileTypeMutex:        "/debug/pprof/mutex",
	models.ProfileTypeAllocs:       "/debug/pprof/allocs",
	models.ProfileTypeThreadCreate: "/debug/pprof/threadcreate",
}

// AllProfiles returns all capturable profile types
var AllProfiles = []models.ProfileType{
	models.ProfileTypeCPU,
	models.ProfileTypeHeap,
	models.ProfileTypeGoroutine,
	models.ProfileTypeBlock,
	models.ProfileTypeMutex,
	models.ProfileTypeAllocs,
	models.ProfileTypeThreadCreate,
}

// CaptureResult holds the result of capturing a single profile
type CaptureResult struct {
	ProfileType models.ProfileType
	Data        []byte
	Size        int
	Duration    time.Duration
	Error       error
}

// Capturer captures pprof profiles from a target and sends to perfkit server
type Capturer struct {
	TargetURL   string
	ServerURL   string
	CPUDuration time.Duration
	Session     string
	Project     string
	Source      string
	client      *http.Client
}

// New creates a new Capturer
func New(targetURL, serverURL string) *Capturer {
	return &Capturer{
		TargetURL:   targetURL,
		ServerURL:   serverURL,
		CPUDuration: 30 * time.Second,
		Source:      "capture",
		client: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for CPU profiles
		},
	}
}

// CaptureProfile fetches a single profile from the target
func (c *Capturer) CaptureProfile(profileType models.ProfileType) CaptureResult {
	result := CaptureResult{ProfileType: profileType}
	start := time.Now()

	endpoint, ok := ProfileEndpoint[profileType]
	if !ok {
		result.Error = fmt.Errorf("unknown profile type: %s", profileType)
		return result
	}

	targetURL := c.TargetURL + endpoint

	// CPU profile needs duration parameter
	if profileType == models.ProfileTypeCPU {
		seconds := int(c.CPUDuration.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		targetURL += fmt.Sprintf("?seconds=%d", seconds)
	}

	resp, err := c.client.Get(targetURL)
	if err != nil {
		result.Error = fmt.Errorf("fetch %s: %w", profileType, err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		result.Error = fmt.Errorf("fetch %s: status %d: %s", profileType, resp.StatusCode, string(body))
		return result
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Errorf("read %s: %w", profileType, err)
		return result
	}

	result.Data = data
	result.Size = len(data)
	result.Duration = time.Since(start)
	return result
}

// SendToServer uploads a captured profile to the perfkit server
func (c *Capturer) SendToServer(result CaptureResult) error {
	if result.Error != nil {
		return result.Error
	}

	// Build ingest URL with query params
	ingestURL, err := url.Parse(c.ServerURL + "/api/pprof/ingest")
	if err != nil {
		return fmt.Errorf("parse server URL: %w", err)
	}

	q := ingestURL.Query()
	q.Set("type", string(result.ProfileType))
	if c.Session != "" {
		q.Set("session", c.Session)
	}
	if c.Project != "" {
		q.Set("project", c.Project)
	}
	if c.Source != "" {
		q.Set("source", c.Source)
	}
	// Mark cumulative profiles
	if result.ProfileType.IsCumulative() {
		q.Set("cumulative", "true")
	}
	// Generate name with timestamp
	q.Set("name", fmt.Sprintf("%s-%s", result.ProfileType, time.Now().Format("20060102-150405")))
	ingestURL.RawQuery = q.Encode()

	// POST the profile data
	resp, err := c.client.Post(ingestURL.String(), "application/octet-stream", bytes.NewReader(result.Data))
	if err != nil {
		return fmt.Errorf("send to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CaptureAndSend captures a profile and sends it to the server
func (c *Capturer) CaptureAndSend(profileType models.ProfileType) CaptureResult {
	result := c.CaptureProfile(profileType)
	if result.Error == nil {
		result.Error = c.SendToServer(result)
	}
	return result
}

// Unused but may be needed for multipart uploads in the future
var _ = multipart.Writer{}
