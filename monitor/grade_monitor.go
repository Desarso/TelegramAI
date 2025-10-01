package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// Canvas API base URL - use CSUS-specific URL for compatibility
const CanvasAPIBaseURL = "https://csus.instructure.com/api/v1"

// Course represents a Canvas course with grade information
type Course struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	CourseCode  string `json:"course_code"`
	Enrollments []struct {
		ComputedCurrentGrade       string  `json:"computed_current_grade"`
		ComputedCurrentScore       float64 `json:"computed_current_score"`
		ComputedCurrentLetterGrade string  `json:"computed_current_letter_grade"`
		ComputedFinalGrade         string  `json:"computed_final_grade"`
		ComputedFinalScore         float64 `json:"computed_final_score"`
	} `json:"enrollments"`
}

// Assignment represents a Canvas assignment
type Assignment struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	DueAt        time.Time `json:"due_at"`
	HasSubmitted bool      // This will be populated separately via submissions API
	HTMLURL      string    `json:"html_url"`
	CourseID     int       `json:"course_id"`
}

// GradeDatabase interface for grade tracking operations
type GradeDatabase interface {
	LoadGradeTracking() (map[int]float64, error)
	SaveOrUpdateGrade(courseID int, courseCode string, score float64) error
	GetGradeTrackingStats() (map[string]interface{}, error)
	CleanupOldMessages() error
	CleanupOldGradeTracking() error
	SaveAssignmentReminder(assignmentID int, reminderHours int, scheduledFor time.Time) error
	LoadPendingReminders() (map[int]map[time.Duration]bool, error)
	MarkReminderAsSent(assignmentID int, reminderHours int) error
	CleanupOldReminders() error
	CancelAssignmentReminders(assignmentID int) error
	GetPendingReminderCount(assignmentID int) (int, error)
}

// GradeMonitor handles grade monitoring and notifications
type GradeMonitor struct {
	apiToken       string
	db             GradeDatabase
	scoreTracker   map[int]float64
	reminderTracker map[int]map[time.Duration]bool
	mutex          sync.RWMutex
	notificationFn func(string) error
}

// NewGradeMonitor creates a new grade monitor
func NewGradeMonitor(apiToken string, db GradeDatabase, notificationFunc func(string) error) *GradeMonitor {
	// Load previous grades from database
	scoreTracker, err := db.LoadGradeTracking()
	if err != nil {
		log.Printf("Warning: Failed to load previous grades from database: %v", err)
		log.Println("Starting with empty grade tracking - will send notifications for all current grades on first run")
		scoreTracker = make(map[int]float64)
	} else {
		log.Printf("Loaded grade tracking data for %d courses from database", len(scoreTracker))
	}

	return &GradeMonitor{
		apiToken:        apiToken,
		db:              db,
		scoreTracker:    scoreTracker,
		reminderTracker: make(map[int]map[time.Duration]bool),
		notificationFn:  notificationFunc,
	}
}

// StartHourlyMonitoring begins the hourly grade checking loop
func (gm *GradeMonitor) StartHourlyMonitoring() {
	log.Println("Starting hourly grade monitoring...")

	// Run initial check immediately
	gm.checkGrades()

	// Start the hourly loop
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		gm.checkGrades()
	}
}

// StartDailyAssignmentMonitoring begins the daily assignment reminder system
func (gm *GradeMonitor) StartDailyAssignmentMonitoring() {
	log.Println("Starting daily assignment monitoring...")

	// Run initial check immediately
	gm.checkAssignments()

	// Start the daily loop at 10 AM
	for {
		now := time.Now()
		nextRun := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location())
		if now.After(nextRun) {
			nextRun = nextRun.Add(24 * time.Hour)
		}

		log.Printf("Next assignment check scheduled for: %s", nextRun.Format(time.RFC1123))
		time.Sleep(time.Until(nextRun))

		// Run cleanup before checking assignments
		gm.runCleanup()

		gm.checkAssignments()
	}
}

// runCleanup performs database cleanup operations
func (gm *GradeMonitor) runCleanup() {
	log.Println("Running database cleanup...")

	// Cleanup old chat messages (older than 4 days)
	if err := gm.db.CleanupOldMessages(); err != nil {
		log.Printf("Error during chat message cleanup: %v", err)
	}

	// Cleanup old grade tracking entries (older than 30 days)
	if err := gm.db.CleanupOldGradeTracking(); err != nil {
		log.Printf("Error during grade tracking cleanup: %v", err)
	}

	// Cleanup old sent reminders (older than 7 days)
	if err := gm.db.CleanupOldReminders(); err != nil {
		log.Printf("Error during reminder cleanup: %v", err)
	}

	log.Println("Database cleanup completed")
}

// checkGrades fetches current grades and compares with stored values
func (gm *GradeMonitor) checkGrades() {
	log.Println("Checking grades for changes...")

	courses, err := gm.fetchCourses()
	if err != nil {
		log.Printf("Error fetching courses: %v", err)
		return
	}

	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	var changesDetected []string

	for _, course := range courses {
		for _, enrollment := range course.Enrollments {
			currentScore := enrollment.ComputedCurrentScore
			previousScore, exists := gm.scoreTracker[course.ID]

			if exists && previousScore != currentScore {
				change := fmt.Sprintf("Course %s score has changed: %.2f → %.2f",
					course.CourseCode, previousScore, currentScore)
				log.Println(change)
				changesDetected = append(changesDetected, change)

				// Send notification
				message := fmt.Sprintf("📊 Grade Alert: %s\nNew score: %.2f%% (was %.2f%%)",
					course.CourseCode, currentScore, previousScore)
				if err := gm.notificationFn(message); err != nil {
					log.Printf("Failed to send grade change notification: %v", err)
				}
			}

			// Update stored score in both memory and database
			gm.scoreTracker[course.ID] = currentScore

			// Save to database for persistence across restarts
			if err := gm.db.SaveOrUpdateGrade(course.ID, course.CourseCode, currentScore); err != nil {
				log.Printf("Failed to save grade to database for course %d: %v", course.ID, err)
			}

			log.Printf("Course %d: %s - Score: %.2f%%", course.ID, course.Name, currentScore)
		}
	}

	if len(changesDetected) > 0 {
		log.Printf("Grade changes detected: %v", changesDetected)
	} else {
		log.Println("No grade changes detected")
	}
}

// checkAssignments fetches assignments and schedules reminders for upcoming due dates
func (gm *GradeMonitor) checkAssignments() {
	log.Println("Checking assignments for reminders...")

	courses, err := gm.fetchCourses()
	if err != nil {
		log.Printf("Error fetching courses for assignments: %v", err)
		return
	}

	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	for _, course := range courses {
		assignments, err := gm.fetchAssignments(course.ID)
		if err != nil {
			log.Printf("Error fetching assignments for course %d: %v", course.ID, err)
			continue
		}

		gm.processAssignments(assignments)
	}
}

// processAssignments processes assignments and schedules reminders
func (gm *GradeMonitor) processAssignments(assignments []Assignment) {
	now := time.Now()

	for _, assignment := range assignments {
		if assignment.DueAt.IsZero() || now.After(assignment.DueAt) {
			continue
		}

		timeUntilDue := assignment.DueAt.Sub(now)
		if timeUntilDue > 0 && timeUntilDue <= 7*24*time.Hour { // Only process assignments due within 7 days
			if assignment.HasSubmitted {
				log.Printf("Assignment '%s' is already submitted.", assignment.Name)

				// Cancel all pending reminders for this submitted assignment
				if err := gm.db.CancelAssignmentReminders(assignment.ID); err != nil {
					log.Printf("Failed to cancel reminders for submitted assignment '%s': %v", assignment.Name, err)
				} else {
					log.Printf("Cancelled all pending reminders for submitted assignment '%s'", assignment.Name)
				}

				// Remove from in-memory tracker
				delete(gm.reminderTracker, assignment.ID)
			} else {
				gm.scheduleAssignmentReminders(assignment)
			}
		}
	}
}

// scheduleAssignmentReminders schedules escalating reminders for an assignment
func (gm *GradeMonitor) scheduleAssignmentReminders(assignment Assignment) {
	now := time.Now()
	dueTime := assignment.DueAt

	// Load existing reminders from database to avoid duplicates
	existingReminders, err := gm.db.LoadPendingReminders()
	if err != nil {
		log.Printf("Warning: Failed to load existing reminders: %v", err)
		existingReminders = make(map[int]map[time.Duration]bool)
	}

	// Initialize reminder tracker for this assignment
	if _, exists := gm.reminderTracker[assignment.ID]; !exists {
		gm.reminderTracker[assignment.ID] = make(map[time.Duration]bool)
	}

	// Reminder intervals: 7h, 5h, 3h, 1h before due
	reminderIntervals := []time.Duration{
		7 * time.Hour,
		5 * time.Hour,
		3 * time.Hour,
		1 * time.Hour,
	}

	for _, interval := range reminderIntervals {
		reminderTime := dueTime.Add(-interval)

		// Skip if reminder time has passed or already scheduled/sent
		if now.After(reminderTime) || gm.reminderTracker[assignment.ID][interval] {
			// Check if it exists in database
			if existingReminders[assignment.ID] != nil && existingReminders[assignment.ID][interval] {
				gm.reminderTracker[assignment.ID][interval] = true
			}
			continue
		}

		// Save reminder to database first
		reminderHours := int(interval.Hours())
		if err := gm.db.SaveAssignmentReminder(assignment.ID, reminderHours, reminderTime); err != nil {
			log.Printf("Failed to save reminder to database for assignment %d: %v", assignment.ID, err)
			continue
		}

		// Mark as scheduled in memory
		gm.reminderTracker[assignment.ID][interval] = true

		// Schedule the reminder
		go gm.scheduleReminder(assignment, interval, reminderTime)
	}
}

// scheduleReminder schedules a single reminder for an assignment
func (gm *GradeMonitor) scheduleReminder(assignment Assignment, interval time.Duration, reminderTime time.Time) {
	log.Printf("⏰ Reminder for '%s' scheduled at %s (%.0f hours before due)",
		assignment.Name, reminderTime.Format(time.RFC1123), interval.Hours())

	time.Sleep(time.Until(reminderTime))

	// Double-check if assignment is still not submitted before sending
	submitted, err := gm.checkAssignmentSubmission(assignment.CourseID, assignment.ID)
	if err != nil {
		log.Printf("Warning: Failed to re-check submission status for assignment %d: %v", assignment.ID, err)
		// Default to not submitted if we can't check
		submitted = false
	}

	if submitted {
		log.Printf("Assignment '%s' was submitted, skipping reminder", assignment.Name)
		// Mark reminder as sent (even though we didn't send it) to prevent rescheduling
		reminderHours := int(interval.Hours())
		if err := gm.db.MarkReminderAsSent(assignment.ID, reminderHours); err != nil {
			log.Printf("Failed to mark skipped reminder as sent: %v", err)
		}
		return
	}

	// Send the reminder
	gm.sendAssignmentNotification(assignment, interval)

	// Mark reminder as sent in database
	reminderHours := int(interval.Hours())
	if err := gm.db.MarkReminderAsSent(assignment.ID, reminderHours); err != nil {
		log.Printf("Failed to mark reminder as sent in database: %v", err)
	}
}

// sendAssignmentNotification sends a reminder notification for an assignment
func (gm *GradeMonitor) sendAssignmentNotification(assignment Assignment, interval time.Duration) {
	timeUntilDue := time.Until(assignment.DueAt)
	hoursUntilDue := timeUntilDue.Hours()

	// Convert Canvas URL to CSUS URL
	htmlUrl := assignment.HTMLURL
	htmlUrl = strings.Replace(htmlUrl, "canvas", "csus", 1)

	urgency := "⏰"
	if hoursUntilDue <= 1 {
		urgency = "🚨 URGENT:"
	} else if hoursUntilDue <= 3 {
		urgency = "⚠️"
	}

	message := fmt.Sprintf("%s Assignment Reminder\n\n📚 **%s**\n⏰ Due in %.1f hours\n🔗 %s",
		urgency, assignment.Name, hoursUntilDue, htmlUrl)

	log.Printf("Sending assignment reminder: %s", assignment.Name)
	if err := gm.notificationFn(message); err != nil {
		log.Printf("Failed to send assignment reminder: %v", err)
	}
}

// fetchCourses gets all favorite courses from Canvas API
func (gm *GradeMonitor) fetchCourses() ([]Course, error) {
	client := &http.Client{}

	req, err := http.NewRequest("GET", CanvasAPIBaseURL+"/users/self/favorites/courses?include[]=total_scores", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+gm.apiToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", resp.Status)
	}

	var courses []Course
	if err := json.NewDecoder(resp.Body).Decode(&courses); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return courses, nil
}

// fetchAssignments gets assignments for a specific course and checks submission status
func (gm *GradeMonitor) fetchAssignments(courseID int) ([]Assignment, error) {
	client := &http.Client{}

	// First get the assignments list
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/courses/%d/assignments", CanvasAPIBaseURL, courseID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+gm.apiToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", resp.Status)
	}

	var assignments []Assignment
	if err := json.NewDecoder(resp.Body).Decode(&assignments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check submission status for each assignment
	for i := range assignments {
		assignment := &assignments[i]
		submitted, err := gm.checkAssignmentSubmission(courseID, assignment.ID)
		if err != nil {
			log.Printf("Warning: Failed to check submission status for assignment %d: %v", assignment.ID, err)
			// Default to not submitted if we can't check
			assignment.HasSubmitted = false
		} else {
			assignment.HasSubmitted = submitted
		}
	}

	return assignments, nil
}

// checkAssignmentSubmission checks if the current user has submitted a specific assignment
func (gm *GradeMonitor) checkAssignmentSubmission(courseID, assignmentID int) (bool, error) {
	client := &http.Client{}

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/courses/%d/assignments/%d/submissions/self", CanvasAPIBaseURL,
			courseID, assignmentID), nil)
	if err != nil {
		return false, fmt.Errorf("failed to create submission request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+gm.apiToken)
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to make submission request: %w", err)
	}
	defer resp.Body.Close()

	// If 404, user hasn't submitted (no submission record exists)
	if resp.StatusCode == 404 {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("submission API error: %s", resp.Status)
	}

	var submission struct {
		ID             int       `json:"id"`
		SubmittedAt    time.Time `json:"submitted_at"`
		WorkflowState  string    `json:"workflow_state"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&submission); err != nil {
		return false, fmt.Errorf("failed to decode submission response: %w", err)
	}

	// Consider it submitted if there's a submission record with a submitted_at date
	return !submission.SubmittedAt.IsZero() && submission.WorkflowState != "unsubmitted", nil
}

// GetStoredScores returns a copy of the current score tracker for debugging
func (gm *GradeMonitor) GetStoredScores() map[int]float64 {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	scores := make(map[int]float64)
	for k, v := range gm.scoreTracker {
		scores[k] = v
	}
	return scores
}

// GetGradeTrackingStats returns statistics about grade tracking from the database
func (gm *GradeMonitor) GetGradeTrackingStats() (map[string]interface{}, error) {
	return gm.db.GetGradeTrackingStats()
}

// SendTelegramMessage sends a message via Telegram bot
func SendTelegramMessage(message string) error {
	// Load environment variables
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			return fmt.Errorf("error loading .env file: %w", err)
		}
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		return fmt.Errorf("BOT_TOKEN not set in environment")
	}

	// Use the chat ID from the old code
	chatIDs := []string{"6995936214"}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	for _, chatID := range chatIDs {
		payload := map[string]string{
			"chat_id": chatID,
			"text":    message,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("telegram API error: status %d", resp.StatusCode)
		}

		log.Printf("Grade notification sent successfully to chat ID %s", chatID)
	}

	return nil
}
