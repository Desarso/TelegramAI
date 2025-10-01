# Telegram AI Agent Makefile

# Build the application
build:
	go build -o telegram-agent .

# Run the CLI mode for testing
chat: build
	./telegram-agent -cli

run: build
	./telegram-agent

# List all chat IDs in the database
list-chats:
	./telegram-agent -list-chats

# Clean build artifacts
clean:
	rm -f telegram-agent
	rm -f chats.db

# Run tests
test:
	go test ./...

# Install dependencies
deps:
	go mod tidy
	go mod download

# Build and run in one command
dev: build chat

# Help target
help:
	@echo "Available targets:"
	@echo "  build      - Build the application"
	@echo "  chat       - Run CLI mode for testing"
	@echo "  list-chats - List all chat IDs in database"
	@echo "  clean      - Remove build artifacts and database"
	@echo "  test       - Run tests"
	@echo "  deps       - Install dependencies"
	@echo "  dev        - Build and run CLI mode"
	@echo "  help       - Show this help message"

.PHONY: build chat list-chats clean test deps dev help
