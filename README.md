# Telegram Canvas AI Agent

A Google Gemini-powered AI agent that helps you manage your Canvas LMS assignments and grades through Telegram.

## Features

- **Interactive AI Chat**: Ask questions about your Canvas courses, assignments, and grades in natural language
- **Canvas API Integration**: Direct access to Canvas LMS data through AI tools
- **General API Access**: The agent can make arbitrary HTTP calls to the Canvas API for advanced operations
- **Conversation Memory**: Maintains context across messages for natural conversations
- **Telegram Integration**: Easy-to-use Telegram bot interface

## Architecture

The agent is built using Google's Gemini AI with function calling capabilities and consists of:

- **Tools System**: Modular tools that provide different capabilities
  - `CanvasAPITool`: General-purpose Canvas API access for any endpoint
  - `CanvasDataTool`: High-level operations for common Canvas tasks
- **Agent Orchestrator**: Manages conversation flow and tool execution
- **Telegram Interface**: Handles bot interactions and message routing

## Setup

1. **Clone and navigate to the project**:
   ```bash
   cd /path/to/TelegramAI
   ```

2. **Install dependencies**:
   ```bash
   go mod tidy
   ```

3. **Create environment file** (`.env`):
   ```env
   BOT_TOKEN=your_telegram_bot_token_here
   CANVAS_API_KEY=your_canvas_api_token_here
   GEMINI_API_KEY=your_gemini_api_key_here
   ```

4. **Build the application**:
   ```bash
   go build -o telegram-agent .
   ```

5. **Run the agent**:
   ```bash
   ./telegram-agent
   ```

## Usage

Once running, you can interact with the agent through Telegram. Here are some example queries:

### Basic Queries
- "What are my current grades?"
- "Show me my courses"
- "What assignments are due soon?"
- "Get my user profile"

### Advanced Queries
- "Get announcements from course 123"
- "Make a GET request to /api/v1/users/self/calendar_events"
- "Submit assignment 456 with comment 'Done!'"

### General Canvas API Access
The agent can make any HTTP request to the Canvas API using the `canvas_api_call` function:
- Method: GET, POST, PUT, DELETE, PATCH
- Endpoint: Any Canvas API path (e.g., `/users/self/courses`, `/courses/123/assignments`)
- Query parameters, headers, and body support

## Tools

### CanvasAPITool (`canvas_api`)
Provides general-purpose Canvas API access for any operation.

**Function**: `canvas_api_call`
- **Parameters**:
  - `method` (required): HTTP method
  - `endpoint` (required): API endpoint path
  - `query` (optional): Query parameters
  - `body` (optional): Request body for POST/PUT/PATCH
  - `headers` (optional): Additional headers

### CanvasDataTool (`canvas_data`)
Provides high-level operations for common Canvas tasks.

**Functions**:
- `get_courses`: Get all favorite courses
- `get_assignments`: Get assignments for a specific course
- `get_upcoming_assignments`: Get unsubmitted assignments with due dates
- `get_grades`: Get current grades for all courses
- `get_user_profile`: Get user profile information
- `get_announcements`: Get course announcements

## Environment Variables

- `BOT_TOKEN`: Telegram bot token from @BotFather
- `CANVAS_API_KEY`: Canvas LMS API token from your Canvas account settings
- `OPEN_ROUTER_KEY`: OpenRouter API key (https://openrouter.ai/)

## API Keys Setup

### Telegram Bot Token
1. Message @BotFather on Telegram
2. Use `/newbot` command
3. Follow the setup instructions
4. Copy the token to your `.env` file

### Canvas API Token
1. Go to your Canvas account settings
2. Generate a new access token
3. Copy the token to your `.env` file

### OpenRouter API Key
1. Visit [OpenRouter](https://openrouter.ai/)
2. Create an account and get your API key
3. Copy the key to your `.env` file as `OPEN_ROUTER_KEY`

## Development

The codebase is organized as follows:

- `main.go`: Main application entry point and agent orchestration
- `tools/`: Tool implementations
  - `canvas.go`: Canvas API client utilities
  - `tool.go`: Tool interfaces and implementations

### Adding New Tools

1. Implement the `Tool` interface
2. Define function declarations for Gemini
3. Implement function execution logic
4. Register the tool in the agent's tool list

## License

This project is open source. Feel free to modify and distribute.
