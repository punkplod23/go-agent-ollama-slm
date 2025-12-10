package webui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"punkplod23/go-agent-ollama-slm/config"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- CONFIGURATION ---
const (
	// Polling configuration for fetching the final result
	PollingInterval    = 1 * time.Second
	MaxPollingAttempts = 15
)

// --- GLOBAL VARIABLE ---
var (
	question         string
	userMessage      Message
	assistantMessage Message // Replaced in Step 2, but used globally
)

// --- STRUCTS: Open WebUI API Models ---

type Chat struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Models   []string  `json:"models"`
	Messages []Message `json:"messages"`
	History  History   `json:"history"`
}

type History struct {
	CurrentID string             `json:"current_id"`
	Messages  map[string]Message `json:"messages"`
}

type Message struct {
	ID        string   `json:"id"`
	Role      string   `json:"role"`
	Content   string   `json:"content"`
	Timestamp int64    `json:"timestamp"`
	ParentID  string   `json:"parentId,omitempty"`
	ModelName string   `json:"modelName,omitempty"`
	ModelIdx  int      `json:"modelIdx,omitempty"`
	Models    []string `json:"models,omitempty"`
}

type BackgroundTasks struct {
	TitleGeneration    bool `json:"title_generation"`
	TagsGeneration     bool `json:"tags_generation"`
	FollowUpGeneration bool `json:"follow_up_generation"`
}

type Features struct {
	CodeInterpreter bool `json:"code_interpreter"`
	WebSearch       bool `json:"web_search"`
	ImageGeneration bool `json:"image_generation"`
	Memory          bool `json:"memory"`
}

type EnvironmentData struct {
	UserName        string `json:"{{USER_NAME}}"`
	UserLanguage    string `json:"{{USER_LANGUAGE}}"`
	CurrentDatetime string `json:"{{CURRENT_DATETIME}}"`
	CurrentTimezone string `json:"{{CURRENT_TIMEZONE}}"`
}

type CompletionRequest struct {
	ChatID          string          `json:"chat_id"`
	MessageID       string          `json:"id"`
	Messages        []Message       `json:"messages"`
	Model           string          `json:"model"`
	Stream          bool            `json:"stream"`
	BackgroundTasks BackgroundTasks `json:"background_tasks"`
	Features        Features        `json:"features"`
	EnvironmentData EnvironmentData `json:"variables"`
	SessionID       string          `json:"session_id,omitempty"`
}

type CompletedRequest struct {
	ChatID    string `json:"chat_id"`
	MessageID string `json:"id"`
	Model     string `json:"model"`
	SessionID string `json:"session_id,omitempty"`
}

// ----------------------------------------------------------------------
// --- API HELPER FUNCTION (WITH DUMPING) ---
// ----------------------------------------------------------------------

func callAPI(method, path string, requestBody interface{}, responseTarget interface{}, cfg *config.Config) error {
	var reqBody io.Reader
	var reqData []byte

	// 1. MARSHAL AND DUMP REQUEST BODY
	if requestBody != nil {
		var err error
		reqData, err = json.MarshalIndent(requestBody, "", "  ") // Use MarshalIndent for readable JSON
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(reqData)
	}

	url := cfg.OpenWebUIHostURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+cfg.OpenWebUIToken)
	req.Header.Set("Content-Type", "application/json")

	// DUMP REQUEST DETAILS
	fmt.Println("\n==============================================")
	fmt.Printf("➡️ DUMPING REQUEST: %s %s\n", method, cfg.OpenWebUIHostURL+path)
	fmt.Println("----------------------------------------------")
	fmt.Printf("Headers:\n")
	for key, values := range req.Header {
		fmt.Printf("  %s: %s\n", key, strings.Join(values, ", "))
	}
	if len(reqData) > 0 {
		fmt.Printf("Body:\n%s\n", string(reqData))
	} else {
		fmt.Println("Body: (None)")
	}
	fmt.Println("==============================================")

	// 2. EXECUTE REQUEST
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed to %s: %w", url, err)
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(resp.Body)

	// DUMP RESPONSE DETAILS
	fmt.Printf("⬅️ DUMPING RESPONSE: %s\n", url)
	fmt.Println("----------------------------------------------")
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response Body (Truncated):\n%s\n", string(responseBody))
	fmt.Println("----------------------------------------------")

	// 3. CHECK STATUS AND DECODE
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API call failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	if responseTarget != nil {
		err = json.Unmarshal(responseBody, responseTarget)
		if err != nil {
			return fmt.Errorf("failed to decode response: %w (Response Body: %s)", err, string(responseBody))
		}
	}
	return nil
}

// ----------------------------------------------------------------------
// --- CHAT FLOW FUNCTIONS ---
// ----------------------------------------------------------------------

// 1. Create a new chat with the user message
func createChat(userQuestion string, cfg *config.Config) (string, string, error) {
	userMsgID := uuid.New().String()
	timestamp := time.Now().UnixMilli()

	userMessage = Message{ // Save to global for reuse in subsequent steps
		ID:        userMsgID,
		Role:      "user",
		Content:   userQuestion,
		Timestamp: timestamp,
		Models:    []string{cfg.OpenWebUIModelName},
	}

	requestPayload := struct {
		Chat Chat `json:"chat"`
	}{
		Chat: Chat{
			Title:    userQuestion,
			Models:   []string{cfg.OpenWebUIModelName},
			Messages: []Message{userMessage},
			History: History{
				CurrentID: userMsgID,
				Messages:  map[string]Message{userMsgID: userMessage},
			},
		},
	}

	var rawResponse map[string]interface{}

	err := callAPI("POST", "/api/v1/chats/new", requestPayload, &rawResponse, cfg)
	if err != nil {
		return "", "", fmt.Errorf("failed to create chat: %w", err)
	}

	chatID, ok := rawResponse["id"].(string)
	if !ok || chatID == "" {
		return "", "", fmt.Errorf("failed to extract top-level ChatID from API response")
	}

	fmt.Printf("✅ Step 1: Chat created. ID: %s\n", chatID)
	return chatID, userMsgID, nil
}

// 2 & 4. Centralized function to update the existing chat state
func updateChat(chatID string, step int, userMsgID string, assistantMsgID string, cfg *config.Config) error {

	// 1. Re-initialize assistantMessage for the update payload
	// This is done to ensure the timestamp is fresh for the update request
	// and the content is empty, ready for streaming.
	currentAssistantMessage := Message{
		ID:        assistantMsgID,
		Role:      "assistant",
		Content:   "",
		ParentID:  userMsgID,
		Timestamp: time.Now().UnixMilli(),
		ModelName: cfg.OpenWebUIModelName,
		ModelIdx:  0,
		Models:    []string{cfg.OpenWebUIModelName},
	}

	// Update global assistantMessage with the fresh payload for the next step
	assistantMessage = currentAssistantMessage

	// Construct the payload
	chatPayload := struct {
		Chat Chat `json:"chat"`
	}{
		Chat: Chat{
			ID:     chatID,
			Title:  question,
			Models: []string{cfg.OpenWebUIModelName},
			Messages: []Message{
				userMessage,
				currentAssistantMessage,
			},
			History: History{
				CurrentID: currentAssistantMessage.ID,
				Messages: map[string]Message{
					userMessage.ID:             userMessage,
					currentAssistantMessage.ID: currentAssistantMessage,
				},
			},
		},
	}

	description := ""
	if step == 2 {
		description = "Inject empty assistant message"
	} else if step == 4 {
		description = "Update chat with model details"
	}

	err := callAPI("POST", fmt.Sprintf("/api/v1/chats/%s", chatID), chatPayload, nil, cfg)
	if err != nil {
		return fmt.Errorf("failed to %s: %w", description, err)
	}

	fmt.Printf("✅ Step %d: %s done.\n", step, description)
	return nil
}

// 3. Trigger the completion (POST /api/chat/completions)
func triggerCompletion(chatID, assistantMsgID string, cfg *config.Config) error {

	requestPayload := CompletionRequest{
		ChatID:    chatID,
		MessageID: assistantMsgID,
		Messages:  []Message{userMessage},
		Model:     cfg.OpenWebUIModelName,
		Stream:    true,
		BackgroundTasks: BackgroundTasks{
			TitleGeneration:    true,
			TagsGeneration:     false,
			FollowUpGeneration: false,
		},
		Features: Features{
			CodeInterpreter: false,
			WebSearch:       false,
			ImageGeneration: false,
			Memory:          false,
		},
		EnvironmentData: EnvironmentData{
			UserName:        "",
			UserLanguage:    "en-US",
			CurrentDatetime: time.Now().Format("2006-01-02 15:04:05"),
			CurrentTimezone: "Europe",
		},
		SessionID: chatID,
	}

	err := callAPI("POST", "/api/chat/completions", requestPayload, nil, cfg)
	if err != nil {
		return fmt.Errorf("failed to trigger completion: %w", err)
	}

	fmt.Println("✅ Step 3: Completion triggered successfully.")
	return nil
}

// 5. Mark the completion as done (POST /api/chat/completed)
func markCompletion(chatID, assistantMsgID string, cfg *config.Config) error {

	requestPayload := CompletedRequest{
		ChatID:    chatID,
		MessageID: assistantMsgID,
		Model:     cfg.OpenWebUIModelName,
		SessionID: chatID,
	}

	err := callAPI("POST", "/api/chat/completed", requestPayload, nil, cfg)
	if err != nil {
		return fmt.Errorf("failed to mark completion: %w", err)
	}

	fmt.Println("✅ Step 5: Completion marked as done.")
	return nil
}

// 6. Polling to fetch the final chat and wait for content (GET /api/v1/chats/{chatId})
func fetchFinalChatWithPolling(chatID, assistantMsgID string, cfg *config.Config) (string, error) {

	var chatArray []Chat
	path := fmt.Sprintf("/api/v1/chats/%s", chatID)

	// Call API: Request the current chat state
	err := callAPI("GET", path, nil, &chatArray, cfg)
	if err != nil {
		// Log the underlying API error but do not treat it as a poll failure yet (for now)
		return "", fmt.Errorf("failed to fetch chat state: %w", err)
	}

	// Check 1: Ensure we got a response
	if len(chatArray) == 0 {
		return "", fmt.Errorf("chat response array was empty, continuing poll")
	}

	resp := chatArray[0]

	// Check 2: Find the specific assistant message by ID
	latestMsg, ok := resp.History.Messages[assistantMsgID]
	if !ok {
		return "", fmt.Errorf("assistant message ID not yet present in history, continuing poll")
	}

	// Check 3: Check message role (should always be assistant)
	if latestMsg.Role != "assistant" {
		return "", fmt.Errorf("found message, but role is incorrect, continuing poll")
	}

	// Check 4 (The goal): Check if content has been populated
	if latestMsg.Content == "" {
		return "", fmt.Errorf("assistant message content is empty, continuing poll")
	}

	// SUCCESS: Content is found.
	fmt.Printf("\nAssistant's Final Content:")
	fmt.Println("---")
	fmt.Println(latestMsg.Content)
	fmt.Println("---")

	return latestMsg.Content, nil
}

func CreateMainChat(cfg *config.Config, prompt string) (error, string) {

	// 1. Get the user's question
	fmt.Println("--- Open WebUI Backend-Controlled Flow (Go) ---")
	fmt.Print("Please enter your question for the LLM: ")

	reader := bufio.NewReader(os.Stdin)

	question = prompt
	if question == "" {
		question, _ = reader.ReadString('\n')
		question = strings.TrimSpace(question)
	}

	fmt.Printf("Question: %s\n\n", question)

	// --- EXECUTE FLOW ---

	chatID, userMsgID, err := createChat(question, cfg)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// Generate assistant ID here, as it's used in all subsequent steps
	assistantMsgID := uuid.New().String()

	// Step 2: Use the unified updateChat function to inject the empty message
	err = updateChat(chatID, 2, userMsgID, assistantMsgID, cfg)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	err = triggerCompletion(chatID, assistantMsgID, cfg)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	return err, ""

}
