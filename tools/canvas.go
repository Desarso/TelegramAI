package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// CanvasTool provides a general interface for making HTTP calls to Canvas LMS API
type CanvasTool struct {
	BaseURL    string
	APIToken   string
	HTTPClient *http.Client
}

// NewCanvasTool creates a new Canvas API tool
func NewCanvasTool(apiToken string) *CanvasTool {
	return &CanvasTool{
		BaseURL:    "https://csus.instructure.com/api/v1",
		APIToken:   apiToken,
		HTTPClient: &http.Client{},
	}
}

// CanvasRequest represents a request to the Canvas API
type CanvasRequest struct {
	Method  string            `json:"method"`
	Endpoint string           `json:"endpoint"` // e.g., "/users/self/courses" or "/courses/123/assignments"
	Headers map[string]string `json:"headers,omitempty"`
	Body    interface{}       `json:"body,omitempty"` // Will be JSON encoded
	Query   map[string]string `json:"query,omitempty"` // Query parameters
}

// CanvasResponse represents the response from Canvas API
type CanvasResponse struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
	Error      string              `json:"error,omitempty"`
}

// MakeRequest makes a general HTTP request to the Canvas API
func (c *CanvasTool) MakeRequest(req CanvasRequest) (*CanvasResponse, error) {
	// Build URL
	url := c.BaseURL + req.Endpoint
	log.Printf("Making Canvas API request: %s %s", req.Method, url)

	// Add query parameters
	if len(req.Query) > 0 {
		params := make([]string, 0, len(req.Query))
		for k, v := range req.Query {
			params = append(params, fmt.Sprintf("%s=%s", k, v))
		}
		url += "?" + strings.Join(params, "&")
		log.Printf("Query parameters: %v", req.Query)
	}

	// Create HTTP request
	var bodyReader io.Reader
	if req.Body != nil {
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
		log.Printf("Request body: %s", string(bodyBytes))
	}

	httpReq, err := http.NewRequest(req.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set authorization header
	httpReq.Header.Set("Authorization", "Bearer "+c.APIToken)

	// Set content type for requests with body
	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Add custom headers
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Make the request
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		log.Printf("HTTP request failed: %v", err)
		return &CanvasResponse{
			Error: fmt.Sprintf("HTTP request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	log.Printf("Canvas API response status: %d", resp.StatusCode)

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body: %v", err)
		return &CanvasResponse{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Error:      fmt.Sprintf("failed to read response body: %v", err),
		}, nil
	}

	log.Printf("Canvas API response body length: %d bytes", len(bodyBytes))

	if resp.StatusCode >= 400 {
		log.Printf("Canvas API error response: %s", string(bodyBytes))
	}

	return &CanvasResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       string(bodyBytes),
	}, nil
}

// Convenience methods for common Canvas operations

// GetCourses gets user's favorite courses
func (c *CanvasTool) GetCourses(includeScores bool) (*CanvasResponse, error) {
	endpoint := "/users/self/favorites/courses"
	query := make(map[string]string)

	if includeScores {
		query["include[]"] = "total_scores"
	}

	return c.MakeRequest(CanvasRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Query:    query,
	})
}

// GetAssignments gets assignments for a specific course
func (c *CanvasTool) GetAssignments(courseID int) (*CanvasResponse, error) {
	endpoint := fmt.Sprintf("/courses/%d/assignments", courseID)

	return c.MakeRequest(CanvasRequest{
		Method:   "GET",
		Endpoint: endpoint,
	})
}

// GetUserProfile gets the current user's profile
func (c *CanvasTool) GetUserProfile() (*CanvasResponse, error) {
	return c.MakeRequest(CanvasRequest{
		Method:   "GET",
		Endpoint: "/users/self/profile",
	})
}

// GetAnnouncements gets announcements for a course
func (c *CanvasTool) GetAnnouncements(courseID int, limit int) (*CanvasResponse, error) {
	endpoint := fmt.Sprintf("/courses/%d/discussion_topics", courseID)
	query := map[string]string{
		"only_announcements": "true",
	}

	if limit > 0 {
		query["per_page"] = fmt.Sprintf("%d", limit)
	}

	return c.MakeRequest(CanvasRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Query:    query,
	})
}

// GetGrades gets grades for all enrollments
func (c *CanvasTool) GetGrades() (*CanvasResponse, error) {
	return c.GetCourses(true) // This includes total_scores
}

// SubmitAssignment submits an assignment
func (c *CanvasTool) SubmitAssignment(courseID, assignmentID int, submission interface{}) (*CanvasResponse, error) {
	endpoint := fmt.Sprintf("/courses/%d/assignments/%d/submissions", courseID, assignmentID)

	return c.MakeRequest(CanvasRequest{
		Method:   "POST",
		Endpoint: endpoint,
		Body:     submission,
	})
}
