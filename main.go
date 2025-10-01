package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
	"desarso/TelegramAI/database"
	"desarso/TelegramAI/monitor"
	"desarso/TelegramAI/tools"
)

// Agent state
type Agent struct {
	client       *openai.Client
	tools        []tools.Tool
	chatDB       *database.ChatDB
	chatHistory  map[int64][]openai.ChatCompletionMessage
}

// getCourseInformation fetches and formats course information for the system prompt
func (a *Agent) getCourseInformation() string {
	// Use the canvas_data tool to get courses
	for _, tool := range a.tools {
		if tool.GetName() == "canvas_data" {
			// Execute the get_courses function
			call := &tools.FunctionCall{
				Name: "get_courses",
				Args: make(map[string]interface{}),
			}

			result, err := tool.ExecuteFunction(call)
			if err != nil {
				log.Printf("Failed to get course information: %v", err)
				return "COURSE INFORMATION: Unable to load course data at this time."
			}

			// Format the course information
			courses, ok := result.([]interface{})
			if !ok || len(courses) == 0 {
				return "COURSE INFORMATION: No courses found or unable to parse course data."
			}

			var courseList []string
			courseList = append(courseList, "COURSE INFORMATION:")
			courseList = append(courseList, "Here are your current Canvas courses:")

			for i, courseRaw := range courses {
				course, ok := courseRaw.(map[string]interface{})
				if !ok {
					continue
				}

				// Extract course information
				courseIDRaw, hasID := course["id"]
				courseCodeRaw, hasCode := course["course_code"]
				courseNameRaw, hasName := course["name"]

				if !hasID || !hasCode || !hasName {
					continue
				}

				// Convert ID to int
				var courseID int
				if idFloat, ok := courseIDRaw.(float64); ok {
					courseID = int(idFloat)
				}

				courseCode, _ := courseCodeRaw.(string)
				courseName, _ := courseNameRaw.(string)

				// Format: "CSC 139-04: CSC139 Operating System Principles - SECTION 04 (ID: 137456)"
				courseStr := fmt.Sprintf("• %s: %s (ID: %d)", courseCode, courseName, courseID)
				courseList = append(courseList, courseStr)

				// Limit to first 10 courses to avoid overly long prompts
				if i >= 9 {
					courseList = append(courseList, "• ... and more courses")
					break
				}
			}

			return strings.Join(courseList, "\n")
		}
	}

	return "COURSE INFORMATION: Unable to load course data at this time."
}

// createSystemMessage creates the system message with current date/time and course information
func (a *Agent) createSystemMessage() openai.ChatCompletionMessage {
	// Get current time in user's timezone and UTC
	now := time.Now()
	// Sacramento, CA is in Pacific Time (America/Los_Angeles)
	userLocation, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Fallback to UTC if timezone loading fails
		userLocation = time.UTC
	}

	userTime := now.In(userLocation).Format("Monday, January 2, 2006 at 3:04 PM (Pacific Time)")
	utcTime := now.UTC().Format("Monday, January 2, 2006 at 15:04 UTC")

	// Fetch course information
	courseInfo := a.getCourseInformation()

	systemContent := fmt.Sprintf(`You are an AI assistant helping Gabriel Malek, a computer science student, with his Canvas LMS courses and assignments.

Current date and time in Pacific Time (Sacramento, CA): %s
Current date and time in UTC: %s

%s

When interpreting assignment due dates and times from Canvas API responses:
- Use UTC times for accurate deadline calculations
- Convert to Pacific Time only for display purposes
- Always use UTC for time-based comparisons and calculations
- Remember that Sacramento, CA follows Pacific Standard Time (PST) or Pacific Daylight Time (PDT)

For displaying due dates to the user:
- CRITICAL: Canvas API returns ALL times in UTC - you MUST convert them to PDT for display
- If an assignment is due today, show "due in X hours" format (e.g., "due in 3 hours")
- If an assignment is due tomorrow, show "due tomorrow at X:XX PM PDT"
- If an assignment is due in the future, show "due on [Day] at X:XX PM PDT"
- Always display times in PDT (Pacific Daylight Time) for user clarity
- Never show UTC times to the user - convert everything to PDT for display

TIME CONVERSION RULES:
- Take the UTC time from Canvas API
- Convert it to Pacific Time (America/Los_Angeles timezone)
- Display the converted time with "PDT" label
- Use the current UTC time provided to calculate "due in X hours" accurately

You have access to Canvas API tools to help with:
- Getting course information and grades
- Retrieving assignments and due dates
- Accessing announcements and course content
- Making general Canvas API calls

Always be helpful, accurate, and conversational. Use the available tools when users ask about Canvas-related information.`, userTime, utcTime, courseInfo)

	return openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemContent,
	}
}

// Initialize the OpenRouter agent
func NewAgent(openRouterKey, canvasToken string) (*Agent, error) {
	// For OpenRouter, use a custom base URL
	config := openai.DefaultConfig(openRouterKey)
	config.BaseURL = "https://openrouter.ai/api/v1" // Use OpenRouter

	client := openai.NewClientWithConfig(config)

	// Initialize tools
	canvasAPITool := tools.NewCanvasAPITool(canvasToken)
	canvasDataTool := tools.NewCanvasDataTool(canvasToken)

	agentTools := []tools.Tool{canvasAPITool, canvasDataTool}

	// Initialize database
	chatDB, err := database.NewChatDB("chats.db")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat database: %w", err)
	}

	return &Agent{
		client:      client,
		tools:       agentTools,
		chatDB:      chatDB,
		chatHistory: make(map[int64][]openai.ChatCompletionMessage),
	}, nil
}

// Process a user message and return a response
func (a *Agent) ProcessMessage(chatID int64, userMessage string) (string, error) {
	ctx := context.Background()

	// Get or create chat history for this chat ID
	history, exists := a.chatHistory[chatID]
	if !exists {
		// Get chat history from database for new conversations
		dbHistory, err := a.chatDB.GetChatHistory(chatID)
		if err != nil {
			return "", fmt.Errorf("failed to get chat history: %w", err)
		}

		// Convert database history to OpenAI format
		history = []openai.ChatCompletionMessage{a.createSystemMessage()}
		for _, msg := range dbHistory {
			if msg.Role == "user" {
				history = append(history, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleUser,
					Content: msg.Content,
				})
			} else if msg.Role == "assistant" {
				history = append(history, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: msg.Content,
				})
			}
		}

		a.chatHistory[chatID] = history
		log.Printf("Created new chat history for chat ID: %d", chatID)
	} else {
		log.Printf("Using existing chat history for chat ID: %d", chatID)
	}

	// Add the current user message to history
	userMsg := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userMessage,
	}
	a.chatHistory[chatID] = append(a.chatHistory[chatID], userMsg)

	// Save user message to database
	if err := a.chatDB.SaveMessage(chatID, "user", userMessage); err != nil {
		log.Printf("Warning: failed to save user message to database: %v", err)
	}

	// Prepare tools for OpenAI format
	var openaiTools []openai.Tool
	for _, tool := range a.tools {
		// Convert our tool declarations to OpenAI format
		for _, decl := range tool.GetFunctionDeclarations() {
			openaiTool := openai.Tool{
				Type: "function",
				Function: &openai.FunctionDefinition{
					Name:        decl.Name,
					Description: decl.Description,
					Parameters:  decl.Parameters,
				},
			}
			openaiTools = append(openaiTools, openaiTool)
		}
	}

	// Create the request
	req := openai.ChatCompletionRequest{
		Model:    "x-ai/grok-4-fast", // Use Grok-4 Fast via OpenRouter
		Messages: a.chatHistory[chatID],
		Tools:    openaiTools,
	}

	// Send request to OpenAI
	resp, err := a.client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Printf("OpenAI API error: %v", err)
		return "", fmt.Errorf("failed to get response from OpenAI: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned from OpenAI")
	}

	choice := resp.Choices[0]
	message := choice.Message

	// Check if the response contains tool calls
	if len(message.ToolCalls) > 0 {
		log.Printf("Tool calls received: %d", len(message.ToolCalls))

		// Execute each tool call
		for _, toolCall := range message.ToolCalls {
			log.Printf("Executing tool call: %s", toolCall.Function.Name)

			// Find the tool that can handle this function
			var result interface{}
			var toolErr error

			for _, tool := range a.tools {
				if tool.GetName() == "canvas_api" || tool.GetName() == "canvas_data" {
					// Convert OpenAI function call to our generic FunctionCall format
					call := &tools.FunctionCall{
						Name: toolCall.Function.Name,
						Args: make(map[string]interface{}),
					}

					// Parse arguments
					if toolCall.Function.Arguments != "" {
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &call.Args); err != nil {
							log.Printf("Failed to parse tool call arguments: %v", err)
							continue
						}
					}

					result, toolErr = tool.ExecuteFunction(call)
					if toolErr == nil {
						log.Printf("Tool %s executed successfully", tool.GetName())
						break
					}
				}
			}

			if toolErr != nil {
				log.Printf("All tools failed for function %s: %v", toolCall.Function.Name, toolErr)
				return "", toolErr
			}

			// Add tool result to conversation
			toolResult := openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    fmt.Sprintf("%v", result),
				ToolCallID: toolCall.ID,
			}
			a.chatHistory[chatID] = append(a.chatHistory[chatID], toolResult)

			// Get another response that should contain the final answer
			finalReq := openai.ChatCompletionRequest{
				Model:    "x-ai/grok-4-fast",
				Messages: a.chatHistory[chatID],
				Tools:    openaiTools,
			}

			finalResp, err := a.client.CreateChatCompletion(ctx, finalReq)
	if err != nil {
				log.Printf("Failed to get final response: %v", err)
				return "", err
			}

			if len(finalResp.Choices) > 0 {
				finalMessage := finalResp.Choices[0].Message
				response := finalMessage.Content

				// Check if response is empty and handle gracefully
				if strings.TrimSpace(response) == "" {
					log.Println("Received empty response from AI after tool call, providing fallback message")
					response = "I apologize, but I wasn't able to generate a response after processing the tool results. Please try asking your question again."

					// Save fallback response to database
					if err := a.chatDB.SaveMessage(chatID, "assistant", response); err != nil {
						log.Printf("Warning: failed to save fallback assistant message to database: %v", err)
					}

					// Add to chat history
					a.chatHistory[chatID] = append(a.chatHistory[chatID], openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleAssistant,
						Content: response,
					})

					return response, nil
				}

				// Save assistant response to database
				if err := a.chatDB.SaveMessage(chatID, "assistant", response); err != nil {
					log.Printf("Warning: failed to save assistant message to database: %v", err)
				}

				// Add to chat history
				a.chatHistory[chatID] = append(a.chatHistory[chatID], openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: response,
				})

				return response, nil
			}
		}
	}

	// If no tool calls, return the direct response
	response := message.Content

	// Check if response is empty and handle gracefully
	if strings.TrimSpace(response) == "" {
		log.Println("Received empty response from AI, providing fallback message")
		response = "I apologize, but I wasn't able to generate a response. Please try asking your question again."

		// Save fallback response to database
		if err := a.chatDB.SaveMessage(chatID, "assistant", response); err != nil {
			log.Printf("Warning: failed to save fallback assistant message to database: %v", err)
		}

		// Add to chat history
		a.chatHistory[chatID] = append(a.chatHistory[chatID], openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: response,
		})

		return response, nil
	}

	// Save assistant response to database
	if err := a.chatDB.SaveMessage(chatID, "assistant", response); err != nil {
		log.Printf("Warning: failed to save assistant message to database: %v", err)
	}

	// Add to chat history
	a.chatHistory[chatID] = append(a.chatHistory[chatID], openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: response,
	})

	return response, nil
}

// Telegram bot setup
func setupTelegramBot(agent *Agent) error {
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		return fmt.Errorf("BOT_TOKEN not set in environment")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return err
	}

	bot.Debug = false
	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			chatID := update.Message.Chat.ID
			userMessage := update.Message.Text

			log.Printf("Received message from %d: %s", chatID, userMessage)

			// Process message through agent (chat session is cached internally)
			response, err := agent.ProcessMessage(chatID, userMessage)
			if err != nil {
				log.Printf("Error processing message: %v", err)
				response = "Sorry, I encountered an error processing your request."
			}

			// Send response back to Telegram
			msg := tgbotapi.NewMessage(chatID, response)
			msg.ParseMode = "Markdown"
			_, err = bot.Send(msg)
			if err != nil {
				log.Printf("Error sending message: %v", err)
			}
		}
	}

	return nil
}

func main() {
	// Load environment variables
	if _, err := os.Stat(".env"); err == nil {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	// Parse command line flags
	cliMode := flag.Bool("cli", false, "Run in CLI mode for testing")
	listChats := flag.Bool("list-chats", false, "List all chat IDs in the database")
	chatID := flag.String("chat-id", "", "Specific chat ID to use in CLI mode (if not provided, a new one will be created)")
	flag.Parse()

	// Get required environment variables
	openRouterKey := os.Getenv("OPEN_ROUTER_KEY")
	if openRouterKey == "" {
		log.Fatal("OPEN_ROUTER_KEY not set in environment")
	}

	canvasToken := os.Getenv("CANVAS_API_KEY")
	if canvasToken == "" {
		log.Fatal("CANVAS_API_KEY not set in environment")
	}

	// Initialize agent
	agent, err := NewAgent(openRouterKey, canvasToken)
	if err != nil {
		log.Fatal("Failed to initialize agent:", err)
	}
	defer agent.chatDB.Close()

	// Start grade monitoring
	log.Println("Starting grade monitoring...")
	gradeMonitor := monitor.NewGradeMonitor(canvasToken, agent.chatDB, monitor.SendTelegramMessage)
	go gradeMonitor.StartHourlyMonitoring()

	// Start assignment monitoring
	log.Println("Starting assignment monitoring...")
	go gradeMonitor.StartDailyAssignmentMonitoring()

	// Handle different modes
	if *listChats {
		handleListChats(agent.chatDB)
		return
	}

	if *cliMode {
		handleCLIMode(agent, *chatID)
		return
	}

	// Default: Telegram mode
	log.Println("Agent initialized successfully!")
	log.Println("You can now ask me questions about your Canvas courses, assignments, and grades.")
	log.Println("Examples:")
	log.Println("- What are my current grades?")
	log.Println("- What assignments are due soon?")
	log.Println("- Show me my courses")

	// Start Telegram bot
	err = setupTelegramBot(agent)
	if err != nil {
		log.Fatal("Failed to setup Telegram bot:", err)
	}
}

// handleListChats lists all chat IDs in the database
func handleListChats(chatDB *database.ChatDB) {
	chatIDs, err := chatDB.GetAllChatIDs()
	if err != nil {
		log.Fatalf("Failed to get chat IDs: %v", err)
	}

	if len(chatIDs) == 0 {
		fmt.Println("No chats found in database.")
		return
	}

	fmt.Println("Chat IDs in database:")
	for _, id := range chatIDs {
		fmt.Printf("- %d\n", id)
	}
}

// handleCLIMode handles CLI mode for testing
func handleCLIMode(agent *Agent, chatIDStr string) {
	// Determine chat ID
	var chatID int64
	if chatIDStr != "" {
		var err error
		chatID, err = strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			log.Fatalf("Invalid chat ID: %s", chatIDStr)
		}
	} else {
		// Generate a new chat ID for this CLI session
		chatID = time.Now().UnixNano()
		fmt.Printf("Using chat ID: %d\n", chatID)
	}

	log.Println("Agent initialized successfully!")
	log.Println("Starting CLI mode. Type 'quit' or 'exit' to end the conversation.")
	log.Println("Type 'help' for available commands.")
	log.Println("Type 'history' to see conversation history.")

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("You: ")

	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			fmt.Print("You: ")
			continue
		}

		// Handle special commands
		switch strings.ToLower(input) {
		case "quit", "exit":
			fmt.Println("Goodbye!")
			return
		case "help":
			fmt.Println("Available commands:")
			fmt.Println("- quit/exit: End the conversation")
			fmt.Println("- help: Show this help message")
			fmt.Println("- history: Show conversation history")
			fmt.Println("- clear: Clear conversation history")
			fmt.Println("- new: Start a new conversation")
			fmt.Println("- Any other text will be sent as a message to the agent")
			fmt.Print("You: ")
			continue
		case "history":
			showChatHistory(agent.chatDB, chatID)
			fmt.Print("You: ")
			continue
		case "clear":
			err := agent.chatDB.DeleteChatHistory(chatID)
			if err != nil {
				fmt.Printf("Error clearing history: %v\n", err)
		} else {
				fmt.Println("Chat history cleared.")
			}
			fmt.Print("You: ")
			continue
		case "new":
			// Start a new conversation
			chatID = time.Now().UnixNano()
			fmt.Printf("Starting new conversation with chat ID: %d\n", chatID)
			fmt.Print("You: ")
			continue
		}

		// Process message through agent
		fmt.Println("Thinking...")
		response, err := agent.ProcessMessage(chatID, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("Assistant: %s\n", response)
		}

		fmt.Print("You: ")
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading input: %v", err)
	}
}

// showChatHistory displays the conversation history for a chat ID
func showChatHistory(chatDB *database.ChatDB, chatID int64) {
	// Get raw messages from database for proper role display
	rows, err := chatDB.GetDB().Query("SELECT role, content FROM chat_history WHERE chat_id = ? ORDER BY timestamp ASC", chatID)
	if err != nil {
		fmt.Printf("Error getting history: %v\n", err)
		return
	}
	defer rows.Close()

	var messages []struct {
		Role    string
		Content string
	}
	for rows.Next() {
		var msg struct {
			Role    string
			Content string
		}
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			fmt.Printf("Error reading message: %v\n", err)
			return
		}
		messages = append(messages, msg)
	}

	if len(messages) == 0 {
		fmt.Println("No conversation history found.")
		return
	}

	fmt.Println("Conversation History:")
	fmt.Println("====================")

	for _, msg := range messages {
		role := "Assistant"
		if msg.Role == "user" {
			role = "You"
		}
		fmt.Printf("%s: %s\n", role, msg.Content)
	}
	fmt.Println("====================")
}