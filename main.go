package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	llm "github.com/desarso/go_llm_functions/helpers"
	"github.com/joho/godotenv"
)

type Assignment struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	DueAt        time.Time `json:"due_at"`
	HasSubmitted bool      `json:"has_submitted_submissions"`
	HTMLURL      string    `json:"html_url"`
}

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

var reminderTracker = make(map[int]map[time.Duration]bool)

// keep track of scores here to detect changes
var scoreTracker = make(map[int]float64)

func main() {
	if _, err := os.Stat(".env"); err == nil {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	llm.GROQ_API_KEY = os.Getenv("GROQ_API_KEY")

	// Get Canvas API key from environment
	apiToken := os.Getenv("CANVAS_API_KEY")
	if apiToken == "" {
		log.Fatal("CANVAS_API_KEY not found in environment")
	}

	sendMessage("Server Updated")

	courseIDs, err := fetchCourseIDs(apiToken)
	if err != nil {
		log.Fatal("Error fetching course IDs:", err)
	}

	go startHourlyGradeReminderBot(apiToken)

	// Start reminder bot for each course
	for _, courseID := range courseIDs {
		go startDailyReminderBot(apiToken, courseID)

	}

	// Keep main thread alive
	select {}
}

func startDailyReminderBot(apiToken string, courseID int) {

	// Run the first check immediately
	fmt.Println("Starting initial assignment fetch...")
	assignments, err := fetchAssignments(apiToken, courseID)
	if err != nil {
		fmt.Printf("Error fetching assignments: %v\n", err)
	} else {
		processAssignments(assignments)
	}
	for {
		now := time.Now()
		nextRun := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
		if now.After(nextRun) {
			nextRun = nextRun.Add(24 * time.Hour)
		}
		fmt.Printf("Next run scheduled for: %s\n", nextRun.Format(time.RFC1123))
		time.Sleep(time.Until(nextRun))
		assignments, err := fetchAssignments(apiToken, courseID)
		if err != nil {
			fmt.Printf("Error fetching assignments: %v\n", err)
			continue
		}
		processAssignments(assignments)
	}
}

func startHourlyGradeReminderBot(apiToken string) {
	// Initialize global score tracker
	globalScoreTracker := make(map[int]float64)

	for {
		// Fetch courses
		courses, err := fetchCourses(apiToken)
		if err != nil {
			fmt.Printf("Error fetching courses: %v\n", err)
			continue
		}

		// Check if ComputedCurrentScore has changed
		for _, course := range courses {
			for _, enrollment := range course.Enrollments {
				if globalScoreTracker[course.ID] != enrollment.ComputedCurrentScore {
					fmt.Printf("Course %d score has changed: %f -> %f\n", course.ID, globalScoreTracker[course.ID], enrollment.ComputedCurrentScore)
					sendMessage("Score change detected for Course: " + course.CourseCode + ". New score: " + strconv.FormatFloat(enrollment.ComputedCurrentScore, 'f', 2, 64))
					globalScoreTracker[course.ID] = enrollment.ComputedCurrentScore
				}
				//print the score anyways along with class name
				fmt.Printf("Course %d: %s - Score: %f\n", course.ID, course.Name, enrollment.ComputedCurrentScore)
			}
		}

		// Sleep for an hour
		time.Sleep(time.Hour)
	}
}

func fetchCourseIDs(apiToken string) ([]int, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://canvas.instructure.com/api/v1/users/self/favorites/courses", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch courses: %s", resp.Status)
	}

	var courses []struct {
		ID int `json:"id"`
	}
	// bodyBytes, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	return nil, err
	// }
	// fmt.Println("Response body:", string(bodyBytes))

	// // Reset the response body for subsequent json.Decode
	// resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	err = json.NewDecoder(resp.Body).Decode(&courses)
	if err != nil {
		return nil, err
	}

	courseIDs := make([]int, len(courses))
	for i, course := range courses {
		courseIDs[i] = course.ID
	}

	fmt.Println("course ids", courseIDs)

	return courseIDs, nil
}

// fetch courses function
func fetchCourses(apiToken string) ([]Course, error) {
	client := &http.Client{}
	courses := []Course{}

	// Assuming courseIDs is a global variable or passed as a parameter
	// If courseIDs is not defined, this function will not work as intended
	// for i, courseID := range courseIDs {
	req, err := http.NewRequest("GET", "https://canvas.instructure.com/api/v1/users/self/favorites/courses?include[]=total_scores", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch course: %s", resp.Status)
	}

	err = json.NewDecoder(resp.Body).Decode(&courses)
	if err != nil {
		return nil, err
	}
	return courses, nil
}

func fetchAssignments(apiToken string, courseID int) ([]Assignment, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://canvas.instructure.com/api/v1/courses/%d/assignments", courseID), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch assignments: %s", resp.Status)
	}

	var assignments []Assignment
	err = json.NewDecoder(resp.Body).Decode(&assignments)
	if err != nil {
		return nil, err
	}

	// fmt.Println("Assignments", assignments)

	return assignments, nil
}

func processAssignments(assignments []Assignment) {
	now := time.Now()
	for _, assignment := range assignments {
		if assignment.DueAt.IsZero() || now.After(assignment.DueAt) {
			continue
		}

		timeUntilDue := assignment.DueAt.Sub(now)
		if timeUntilDue > 0 && timeUntilDue <= 48*time.Hour {
			if assignment.HasSubmitted {
				fmt.Printf("Assignment '%s' is already submitted.\n", assignment.Name)
			} else {
				scheduleReminder(assignment)
			}
		}
	}
}

func scheduleReminder(assignment Assignment) {
	now := time.Now()
	dueTime := assignment.DueAt

	if _, exists := reminderTracker[assignment.ID]; !exists {
		reminderTracker[assignment.ID] = make(map[time.Duration]bool)
	}

	reminderIntervals := []time.Duration{
		12 * time.Hour,
		6 * time.Hour,
		3 * time.Hour,
		1 * time.Hour,
	}

	for _, interval := range reminderIntervals {
		reminderTime := dueTime.Add(-interval)
		if now.After(reminderTime) || reminderTracker[assignment.ID][interval] {
			continue
		}

		go func(reminderTime time.Time, interval time.Duration) {
			fmt.Printf("Reminder for '%s' scheduled at %s\n", assignment.Name, reminderTime.Format(time.RFC1123))
			time.Sleep(time.Until(reminderTime))
			sendNotification(assignment)
			reminderTracker[assignment.ID][interval] = true
		}(reminderTime, interval)
	}
}

func sendNotification(assignment Assignment) {
	fmt.Printf("🚨 Reminder: The assignment '%s' is due at %s.\n",
		assignment.Name, assignment.DueAt.Format(time.RFC1123))
	//make a prompt that states how many hours until the assignment is due
	timeUntilDue := assignment.DueAt.Sub(time.Now())
	hoursUntilDue := timeUntilDue.Hours()
	htmlUrl := assignment.HTMLURL
	htmlUrl = strings.Replace(htmlUrl, "canvas", "csus", 1)
	prompt := fmt.Sprintf("Create a motivating message that will get the user to do his homework, the close it is to the due date, the more urgent the message should be .The assignment '%s' is due in %.1f hours. Here is the link to the assignment: %s", assignment.Name, hoursUntilDue, htmlUrl)
	message := llm.Ell(createMessage)(prompt)
	sendMessage(message)

}

func sendMessage(message string) error {
	// Load environment variables
	if _, err := os.Stat(".env"); err == nil {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	// Retrieve environment variables
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN not set in environment")
		return fmt.Errorf("BOT_TOKEN not set")
	}
	// Define chat IDs
	chatIDs := []string{"6995936214"}
	// URL to send the message
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	// Iterate over chat IDs and send messages
	for _, chatID := range chatIDs {
		// Create the payload
		payload := map[string]string{
			"chat_id": chatID,
			"text":    message,
		}

		// Convert payload to JSON
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to marshal payload for chat ID %s: %v", chatID, err)
			continue
		}

		// Send the POST request
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
		if err != nil {
			log.Printf("Failed to send request to chat ID %s: %v", chatID, err)
			continue
		}
		defer resp.Body.Close()

		// Check if the message was sent successfully
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("Message sent successfully to chat ID %s!\n", chatID)
		} else {
			var respBody map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
				log.Printf("Failed to decode response body for chat ID %s: %v", chatID, err)
			}
			//fmt.Printf("Failed to send message to chat ID %s. Status code: %d\n", chatID, resp.StatusCode)
			//fmt.Println("Response:", respBody)
		}
	}
	return nil
}

func getAllChatIds() ([]string, error) {
	if _, err := os.Stat(".env"); err == nil {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	// Retrieve environment variables
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN not set in environment")
	}

	// URL to get updates
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", botToken)

	// Send the GET request
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to send request: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get updates: status code %d", resp.StatusCode)
	}

	// Parse the response body
	var respBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		log.Printf("Failed to decode response body: %v", err)
		return nil, err
	}

	// Extract chat IDs
	chatIDSet := make(map[string]struct{})
	result, ok := respBody["result"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}

	for _, v := range result {
		message, ok := v.(map[string]interface{})["message"].(map[string]interface{})
		if !ok {
			continue
		}
		chat, ok := message["chat"].(map[string]interface{})
		if !ok {
			continue
		}
		chatID, ok := chat["id"].(float64)
		if !ok {
			continue
		}
		// Add chat ID to the set to remove duplicates
		chatIDSet[fmt.Sprintf("%.0f", chatID)] = struct{}{}

	}
	var chatIds []string
	for id := range chatIDSet {
		chatIds = append(chatIds, id)
	}

	return chatIds, nil
}

func createMessage(prompt string) string {
	return prompt
}
