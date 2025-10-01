package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

// ChatDB handles chat history storage in SQLite
type ChatDB struct {
	db *sql.DB
}

// ChatMessage represents a message in the chat history (database format)
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// NewChatDB creates a new chat database connection
func NewChatDB(dbPath string) (*ChatDB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create tables if they don't exist
	if err := createTables(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return &ChatDB{db: db}, nil
}

// GetDB returns the underlying database connection (for advanced queries)
func (c *ChatDB) GetDB() *sql.DB {
	return c.db
}

// Close closes the database connection
func (c *ChatDB) Close() error {
	return c.db.Close()
}

// createTables creates the necessary database tables
func createTables(db *sql.DB) error {
	// Create chat history table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS chat_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		chat_id INTEGER NOT NULL,
		role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
		content TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(chat_id, role, timestamp)
	);

	CREATE INDEX IF NOT EXISTS idx_chat_id ON chat_history(chat_id);
	CREATE INDEX IF NOT EXISTS idx_timestamp ON chat_history(timestamp);

	CREATE TABLE IF NOT EXISTS grade_tracking (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		course_id INTEGER NOT NULL UNIQUE,
		course_code TEXT NOT NULL,
		previous_score REAL NOT NULL,
		last_updated DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(course_id)
	);

	CREATE INDEX IF NOT EXISTS idx_grade_course_id ON grade_tracking(course_id);
	CREATE INDEX IF NOT EXISTS idx_grade_last_updated ON grade_tracking(last_updated);

	CREATE TABLE IF NOT EXISTS assignment_reminders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		assignment_id INTEGER NOT NULL,
		reminder_hours INTEGER NOT NULL,
		scheduled_for DATETIME NOT NULL,
		sent BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(assignment_id, reminder_hours)
	);

	CREATE INDEX IF NOT EXISTS idx_reminder_assignment ON assignment_reminders(assignment_id);
	CREATE INDEX IF NOT EXISTS idx_reminder_scheduled ON assignment_reminders(scheduled_for);
	CREATE INDEX IF NOT EXISTS idx_reminder_sent ON assignment_reminders(sent);
	`

	_, err := db.Exec(createTableSQL)
	return err
}

// SaveMessage saves a message to the database
func (c *ChatDB) SaveMessage(chatID int64, role string, content string) error {
	_, err := c.db.Exec(
		"INSERT INTO chat_history (chat_id, role, content, timestamp) VALUES (?, ?, ?, ?)",
		chatID, role, content, time.Now(),
	)
	return err
}

// GetChatHistory retrieves chat history for a specific chat ID (limited to last 4 days)
func (c *ChatDB) GetChatHistory(chatID int64) ([]ChatMessage, error) {
	// Calculate cutoff time (4 days ago)
	cutoffTime := time.Now().AddDate(0, 0, -4)

	rows, err := c.db.Query(
		"SELECT role, content FROM chat_history WHERE chat_id = ? AND timestamp > ? ORDER BY timestamp ASC",
		chatID, cutoffTime,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, err
		}
		history = append(history, msg)
	}

	return history, rows.Err()
}

// GetAllChatIDs returns all unique chat IDs in the database
func (c *ChatDB) GetAllChatIDs() ([]int64, error) {
	rows, err := c.db.Query("SELECT DISTINCT chat_id FROM chat_history ORDER BY chat_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chatIDs []int64
	for rows.Next() {
		var chatID int64
		if err := rows.Scan(&chatID); err != nil {
			return nil, err
		}
		chatIDs = append(chatIDs, chatID)
	}

	return chatIDs, rows.Err()
}

// DeleteChatHistory deletes all messages for a specific chat ID
func (c *ChatDB) DeleteChatHistory(chatID int64) error {
	_, err := c.db.Exec("DELETE FROM chat_history WHERE chat_id = ?", chatID)
	return err
}

// GetChatStats returns statistics about the chat database
func (c *ChatDB) GetChatStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total messages
	var totalMessages int64
	err := c.db.QueryRow("SELECT COUNT(*) FROM chat_history").Scan(&totalMessages)
	if err != nil {
		return nil, err
	}
	stats["total_messages"] = totalMessages

	// Total chats
	var totalChats int64
	err = c.db.QueryRow("SELECT COUNT(DISTINCT chat_id) FROM chat_history").Scan(&totalChats)
	if err != nil {
		return nil, err
	}
	stats["total_chats"] = totalChats

	// Messages by role
	var userMessages, assistantMessages int64
	err = c.db.QueryRow("SELECT COUNT(*) FROM chat_history WHERE role = 'user'").Scan(&userMessages)
	if err != nil {
		return nil, err
	}
	err = c.db.QueryRow("SELECT COUNT(*) FROM chat_history WHERE role = 'assistant'").Scan(&assistantMessages)
	if err != nil {
		return nil, err
	}
	stats["user_messages"] = userMessages
	stats["assistant_messages"] = assistantMessages

	// Oldest and newest message timestamps
	var oldestTime, newestTime time.Time
	err = c.db.QueryRow("SELECT MIN(timestamp), MAX(timestamp) FROM chat_history").Scan(&oldestTime, &newestTime)
	if err != nil {
		return nil, err
	}
	stats["oldest_message"] = oldestTime
	stats["newest_message"] = newestTime

	return stats, nil
}

// GradeData represents a course's grade tracking information
type GradeData struct {
	CourseID     int     `json:"course_id"`
	CourseCode   string  `json:"course_code"`
	PreviousScore float64 `json:"previous_score"`
	LastUpdated  time.Time `json:"last_updated"`
}

// LoadGradeTracking loads all previously tracked grades from the database
func (c *ChatDB) LoadGradeTracking() (map[int]float64, error) {
	rows, err := c.db.Query(
		"SELECT course_id, course_code, previous_score FROM grade_tracking ORDER BY last_updated DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query grade tracking: %w", err)
	}
	defer rows.Close()

	gradeMap := make(map[int]float64)
	for rows.Next() {
		var courseID int
		var courseCode string
		var previousScore float64

		if err := rows.Scan(&courseID, &courseCode, &previousScore); err != nil {
			return nil, fmt.Errorf("failed to scan grade tracking row: %w", err)
		}

		gradeMap[courseID] = previousScore
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating grade tracking rows: %w", err)
	}

	return gradeMap, nil
}

// SaveOrUpdateGrade saves or updates a course's grade in the tracking database
func (c *ChatDB) SaveOrUpdateGrade(courseID int, courseCode string, score float64) error {
	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO grade_tracking (course_id, course_code, previous_score, last_updated)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		courseID, courseCode, score)
	if err != nil {
		return fmt.Errorf("failed to save/update grade tracking: %w", err)
	}
	return nil
}

// DeleteGradeTracking removes a course from grade tracking (useful for cleanup)
func (c *ChatDB) DeleteGradeTracking(courseID int) error {
	_, err := c.db.Exec("DELETE FROM grade_tracking WHERE course_id = ?", courseID)
	if err != nil {
		return fmt.Errorf("failed to delete grade tracking: %w", err)
	}
	return nil
}

// GetGradeTrackingStats returns statistics about grade tracking
func (c *ChatDB) GetGradeTrackingStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var totalTracked int64
	err := c.db.QueryRow("SELECT COUNT(*) FROM grade_tracking").Scan(&totalTracked)
	if err != nil {
		return nil, fmt.Errorf("failed to count grade tracking entries: %w", err)
	}
	stats["total_tracked_courses"] = totalTracked

	// Get oldest and newest tracking timestamps
	var oldestTime, newestTime time.Time
	err = c.db.QueryRow("SELECT MIN(last_updated), MAX(last_updated) FROM grade_tracking").Scan(&oldestTime, &newestTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get grade tracking timestamps: %w", err)
	}
	stats["oldest_tracking"] = oldestTime
	stats["newest_tracking"] = newestTime

	return stats, nil
}

// CleanupOldMessages removes chat messages older than 4 days to prevent database bloat
func (c *ChatDB) CleanupOldMessages() error {
	cutoffTime := time.Now().AddDate(0, 0, -4)

	result, err := c.db.Exec(
		"DELETE FROM chat_history WHERE timestamp < ?",
		cutoffTime,
	)
	if err != nil {
		return fmt.Errorf("failed to cleanup old messages: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Warning: Could not get rows affected for cleanup: %v", err)
	} else {
		log.Printf("Cleaned up %d old chat messages (older than 4 days)", rowsAffected)
	}

	return nil
}

// CleanupOldGradeTracking removes grade tracking entries older than 30 days
func (c *ChatDB) CleanupOldGradeTracking() error {
	cutoffTime := time.Now().AddDate(0, 0, -30)

	result, err := c.db.Exec(
		"DELETE FROM grade_tracking WHERE last_updated < ?",
		cutoffTime,
	)
	if err != nil {
		return fmt.Errorf("failed to cleanup old grade tracking: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Warning: Could not get rows affected for grade cleanup: %v", err)
	} else if rowsAffected > 0 {
		log.Printf("Cleaned up %d old grade tracking entries (older than 30 days)", rowsAffected)
	}

	return nil
}

// SaveAssignmentReminder saves a scheduled reminder to the database
func (c *ChatDB) SaveAssignmentReminder(assignmentID int, reminderHours int, scheduledFor time.Time) error {
	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO assignment_reminders (assignment_id, reminder_hours, scheduled_for, sent)
		VALUES (?, ?, ?, FALSE)`,
		assignmentID, reminderHours, scheduledFor)
	if err != nil {
		return fmt.Errorf("failed to save assignment reminder: %w", err)
	}
	return nil
}

// LoadPendingReminders loads all pending (unsent) reminders from the database
func (c *ChatDB) LoadPendingReminders() (map[int]map[time.Duration]bool, error) {
	rows, err := c.db.Query(
		"SELECT assignment_id, reminder_hours FROM assignment_reminders WHERE sent = FALSE AND scheduled_for > ?",
		time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending reminders: %w", err)
	}
	defer rows.Close()

	reminderMap := make(map[int]map[time.Duration]bool)
	for rows.Next() {
		var assignmentID int
		var reminderHours int

		if err := rows.Scan(&assignmentID, &reminderHours); err != nil {
			return nil, fmt.Errorf("failed to scan reminder row: %w", err)
		}

		if _, exists := reminderMap[assignmentID]; !exists {
			reminderMap[assignmentID] = make(map[time.Duration]bool)
		}
		reminderMap[assignmentID][time.Duration(reminderHours) * time.Hour] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reminder rows: %w", err)
	}

	return reminderMap, nil
}

// MarkReminderAsSent marks a reminder as sent in the database
func (c *ChatDB) MarkReminderAsSent(assignmentID int, reminderHours int) error {
	_, err := c.db.Exec(
		"UPDATE assignment_reminders SET sent = TRUE WHERE assignment_id = ? AND reminder_hours = ?",
		assignmentID, reminderHours,
	)
	if err != nil {
		return fmt.Errorf("failed to mark reminder as sent: %w", err)
	}
	return nil
}

// CleanupOldReminders removes old sent reminders to prevent database bloat
func (c *ChatDB) CleanupOldReminders() error {
	cutoffTime := time.Now().AddDate(0, 0, -7) // Keep reminders for 7 days

	result, err := c.db.Exec(
		"DELETE FROM assignment_reminders WHERE sent = TRUE AND created_at < ?",
		cutoffTime,
	)
	if err != nil {
		return fmt.Errorf("failed to cleanup old reminders: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Warning: Could not get rows affected for reminder cleanup: %v", err)
	} else if rowsAffected > 0 {
		log.Printf("Cleaned up %d old sent reminders (older than 7 days)", rowsAffected)
	}

	return nil
}

// CancelAssignmentReminders cancels all pending reminders for a specific assignment
func (c *ChatDB) CancelAssignmentReminders(assignmentID int) error {
	result, err := c.db.Exec(
		"DELETE FROM assignment_reminders WHERE assignment_id = ? AND sent = FALSE",
		assignmentID,
	)
	if err != nil {
		return fmt.Errorf("failed to cancel assignment reminders: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Warning: Could not get rows affected for reminder cancellation: %v", err)
	} else if rowsAffected > 0 {
		log.Printf("Cancelled %d pending reminders for assignment %d", rowsAffected, assignmentID)
	}

	return nil
}

// GetPendingReminderCount returns the count of pending reminders for an assignment
func (c *ChatDB) GetPendingReminderCount(assignmentID int) (int, error) {
	var count int
	err := c.db.QueryRow(
		"SELECT COUNT(*) FROM assignment_reminders WHERE assignment_id = ? AND sent = FALSE AND scheduled_for > ?",
		assignmentID, time.Now(),
	).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to get pending reminder count: %w", err)
	}

	return count, nil
}
