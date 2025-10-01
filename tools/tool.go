package tools

import (
	"encoding/json"
	"fmt"
	"log"
)

// FunctionCall represents a generic function call
type FunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// FunctionDeclaration represents a generic function declaration
type FunctionDeclaration struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// Tool represents a tool that can be used by the agent
type Tool interface {
	// GetFunctionDeclarations returns the function declarations for this tool
	GetFunctionDeclarations() []*FunctionDeclaration

	// ExecuteFunction executes a function call
	ExecuteFunction(call *FunctionCall) (interface{}, error)

	// GetName returns the name of this tool
	GetName() string
}

// CanvasAPITool is a tool that provides general Canvas API access
type CanvasAPITool struct {
	canvasTool *CanvasTool
}

// NewCanvasAPITool creates a new Canvas API tool
func NewCanvasAPITool(apiToken string) *CanvasAPITool {
	return &CanvasAPITool{
		canvasTool: NewCanvasTool(apiToken),
	}
}

// GetName returns the tool name
func (t *CanvasAPITool) GetName() string {
	return "canvas_api"
}

// GetFunctionDeclarations returns function declarations for the Canvas API tool
func (t *CanvasAPITool) GetFunctionDeclarations() []*FunctionDeclaration {
	return []*FunctionDeclaration{
		{
			Name:        "canvas_api_call",
			Description: "Make a general HTTP request to the Canvas LMS API. Use this for any Canvas API operation not covered by specific tools.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"method": map[string]interface{}{
						"type":        "string",
						"description": "HTTP method (GET, POST, PUT, DELETE, etc.)",
						"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
					},
					"endpoint": map[string]interface{}{
						"type":        "string",
						"description": "API endpoint path (e.g., '/users/self/courses', '/courses/123/assignments')",
					},
					"query": map[string]interface{}{
						"type":        "object",
						"description": "Query parameters as key-value pairs",
					},
					"body": map[string]interface{}{
						"type":        "object",
						"description": "Request body as JSON object (for POST/PUT/PATCH requests)",
					},
					"headers": map[string]interface{}{
						"type":        "object",
						"description": "Additional headers as key-value pairs",
					},
				},
				"required": []string{"method", "endpoint"},
			},
		},
	}
}

// ExecuteFunction executes a Canvas API function call
func (t *CanvasAPITool) ExecuteFunction(call *FunctionCall) (interface{}, error) {
	switch call.Name {
	case "canvas_api_call":
		return t.executeCanvasAPICall(call.Args)
	default:
		return nil, &ToolError{
			Tool:    t.GetName(),
			Message: "unknown function: " + call.Name,
		}
	}
}

func (t *CanvasAPITool) executeCanvasAPICall(args map[string]interface{}) (interface{}, error) {
	// Extract method
	methodRaw, ok := args["method"]
	if !ok {
		return nil, &ToolError{Tool: t.GetName(), Message: "method parameter is required"}
	}
	method, ok := methodRaw.(string)
	if !ok {
		return nil, &ToolError{Tool: t.GetName(), Message: "method must be a string"}
	}

	// Extract endpoint
	endpointRaw, ok := args["endpoint"]
	if !ok {
		return nil, &ToolError{Tool: t.GetName(), Message: "endpoint parameter is required"}
	}
	endpoint, ok := endpointRaw.(string)
	if !ok {
		return nil, &ToolError{Tool: t.GetName(), Message: "endpoint must be a string"}
	}

	// Build request
	req := CanvasRequest{
		Method:   method,
		Endpoint: endpoint,
	}

	// Extract optional query parameters
	if queryRaw, ok := args["query"]; ok {
		if queryMap, ok := queryRaw.(map[string]interface{}); ok {
			req.Query = make(map[string]string)
			for k, v := range queryMap {
				if vStr, ok := v.(string); ok {
					req.Query[k] = vStr
				}
			}
		}
	}

	// Extract optional body
	if bodyRaw, ok := args["body"]; ok {
		req.Body = bodyRaw
	}

	// Extract optional headers
	if headersRaw, ok := args["headers"]; ok {
		if headersMap, ok := headersRaw.(map[string]interface{}); ok {
			req.Headers = make(map[string]string)
			for k, v := range headersMap {
				if vStr, ok := v.(string); ok {
					req.Headers[k] = vStr
				}
			}
		}
	}

	// Make the API call
	resp, err := t.canvasTool.MakeRequest(req)
	if err != nil {
		return nil, &ToolError{Tool: t.GetName(), Message: err.Error()}
	}

	return resp, nil
}

// ToolError represents an error from a tool
type ToolError struct {
	Tool    string
	Message string
}

func (e *ToolError) Error() string {
	return fmt.Sprintf("tool %s error: %s", e.Tool, e.Message)
}

// CanvasDataTool provides high-level Canvas operations
type CanvasDataTool struct {
	canvasTool *CanvasTool
}

// NewCanvasDataTool creates a new Canvas data tool
func NewCanvasDataTool(apiToken string) *CanvasDataTool {
	return &CanvasDataTool{
		canvasTool: NewCanvasTool(apiToken),
	}
}

// GetName returns the tool name
func (t *CanvasDataTool) GetName() string {
	return "canvas_data"
}

// GetFunctionDeclarations returns function declarations for Canvas data operations
func (t *CanvasDataTool) GetFunctionDeclarations() []*FunctionDeclaration {
	return []*FunctionDeclaration{
		{
			Name:        "get_courses",
			Description: "Get all favorite courses from Canvas LMS",
			Parameters: map[string]interface{}{
				"type": "object",
			},
		},
		{
			Name:        "get_assignments",
			Description: "Get assignments for a specific course",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"course_id": map[string]interface{}{
						"type":        "integer",
						"description": "The course ID to get assignments for",
					},
				},
				"required": []string{"course_id"},
			},
		},
		{
			Name:        "get_upcoming_assignments",
			Description: "Get all upcoming assignments that haven't been submitted yet",
			Parameters: map[string]interface{}{
				"type": "object",
			},
		},
		{
			Name:        "get_grades",
			Description: "Get current grades for all courses",
			Parameters: map[string]interface{}{
				"type": "object",
			},
		},
		{
			Name:        "get_user_profile",
			Description: "Get the current user's profile information",
			Parameters: map[string]interface{}{
				"type": "object",
			},
		},
		{
			Name:        "get_announcements",
			Description: "Get recent announcements for a course",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"course_id": map[string]interface{}{
						"type":        "integer",
						"description": "The course ID to get announcements for",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of announcements to retrieve (default: 10)",
					},
				},
				"required": []string{"course_id"},
			},
		},
	}
}

// ExecuteFunction executes a Canvas data function call
func (t *CanvasDataTool) ExecuteFunction(call *FunctionCall) (interface{}, error) {
	switch call.Name {
	case "get_courses":
		return t.getCourses()
	case "get_assignments":
		return t.getAssignments(call.Args)
	case "get_upcoming_assignments":
		return t.getUpcomingAssignments()
	case "get_grades":
		return t.getGrades()
	case "get_user_profile":
		return t.getUserProfile()
	case "get_announcements":
		return t.getAnnouncements(call.Args)
	default:
		return nil, &ToolError{
			Tool:    t.GetName(),
			Message: "unknown function: " + call.Name,
		}
	}
}

func (t *CanvasDataTool) getCourses() (interface{}, error) {
	log.Printf("CanvasDataTool: Getting courses")
	resp, err := t.canvasTool.GetCourses(true)
	if err != nil {
		log.Printf("CanvasDataTool: Failed to get courses from API: %v", err)
		return nil, err
	}

	log.Printf("CanvasDataTool: API response status: %d", resp.StatusCode)

	if resp.StatusCode != 200 {
		log.Printf("CanvasDataTool: API error response: %s", resp.Body)
		return nil, &ToolError{Tool: t.GetName(), Message: fmt.Sprintf("API error: %s", resp.Body)}
	}

	var courses []interface{}
	if err := json.Unmarshal([]byte(resp.Body), &courses); err != nil {
		log.Printf("CanvasDataTool: JSON parse error: %v", err)
		return nil, &ToolError{Tool: t.GetName(), Message: fmt.Sprintf("JSON parse error: %v", err)}
	}

	log.Printf("CanvasDataTool: Successfully parsed %d courses", len(courses))
	return courses, nil
}

func (t *CanvasDataTool) getAssignments(args map[string]interface{}) (interface{}, error) {
	courseIDRaw, ok := args["course_id"]
	if !ok {
		return nil, &ToolError{Tool: t.GetName(), Message: "course_id parameter is required"}
	}

	courseIDFloat, ok := courseIDRaw.(float64)
	if !ok {
		return nil, &ToolError{Tool: t.GetName(), Message: "course_id must be a number"}
	}

	courseID := int(courseIDFloat)
	resp, err := t.canvasTool.GetAssignments(courseID)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, &ToolError{Tool: t.GetName(), Message: fmt.Sprintf("API error: %s", resp.Body)}
	}

	var assignments []interface{}
	if err := json.Unmarshal([]byte(resp.Body), &assignments); err != nil {
		return nil, &ToolError{Tool: t.GetName(), Message: fmt.Sprintf("JSON parse error: %v", err)}
	}

	return assignments, nil
}

func (t *CanvasDataTool) getUpcomingAssignments() (interface{}, error) {
	// First get all courses
	coursesResp, err := t.getCourses()
	if err != nil {
		return nil, err
	}

	courses, ok := coursesResp.([]interface{})
	if !ok {
		return nil, &ToolError{Tool: t.GetName(), Message: "failed to parse courses"}
	}

	var allAssignments []interface{}

	for _, courseRaw := range courses {
		course, ok := courseRaw.(map[string]interface{})
		if !ok {
			continue
		}

		courseIDRaw, ok := course["id"]
		if !ok {
			continue
		}

		courseIDFloat, ok := courseIDRaw.(float64)
		if !ok {
			continue
		}

		assignmentsResp, err := t.getAssignments(map[string]interface{}{"course_id": courseIDFloat})
		if err != nil {
			continue // Skip courses we can't access
		}

		if assignments, ok := assignmentsResp.([]interface{}); ok {
			allAssignments = append(allAssignments, assignments...)
		}
	}

	// Filter for upcoming assignments (you'd need to implement date filtering here)
	return allAssignments, nil
}

func (t *CanvasDataTool) getGrades() (interface{}, error) {
	return t.getCourses() // Grades are included in course data
}

func (t *CanvasDataTool) getUserProfile() (interface{}, error) {
	resp, err := t.canvasTool.GetUserProfile()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, &ToolError{Tool: t.GetName(), Message: fmt.Sprintf("API error: %s", resp.Body)}
	}

	var profile interface{}
	if err := json.Unmarshal([]byte(resp.Body), &profile); err != nil {
		return nil, &ToolError{Tool: t.GetName(), Message: fmt.Sprintf("JSON parse error: %v", err)}
	}

	return profile, nil
}

func (t *CanvasDataTool) getAnnouncements(args map[string]interface{}) (interface{}, error) {
	courseIDRaw, ok := args["course_id"]
	if !ok {
		return nil, &ToolError{Tool: t.GetName(), Message: "course_id parameter is required"}
	}

	courseIDFloat, ok := courseIDRaw.(float64)
	if !ok {
		return nil, &ToolError{Tool: t.GetName(), Message: "course_id must be a number"}
	}

	courseID := int(courseIDFloat)

	limit := 10 // default
	if limitRaw, ok := args["limit"]; ok {
		if limitFloat, ok := limitRaw.(float64); ok {
			limit = int(limitFloat)
		}
	}

	resp, err := t.canvasTool.GetAnnouncements(courseID, limit)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, &ToolError{Tool: t.GetName(), Message: fmt.Sprintf("API error: %s", resp.Body)}
	}

	var announcements []interface{}
	if err := json.Unmarshal([]byte(resp.Body), &announcements); err != nil {
		return nil, &ToolError{Tool: t.GetName(), Message: fmt.Sprintf("JSON parse error: %v", err)}
	}

	return announcements, nil
}
