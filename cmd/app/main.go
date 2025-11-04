package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
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

type VehicleResponse struct {
	RegistrationNumber       string `json:"registrationNumber"`
	TaxStatus                string `json:"taxStatus"`
	MotStatus                string `json:"motStatus"`
	Make                     string `json:"make"`
	YearOfManufacture        int    `json:"yearOfManufacture"`
	EngineCapacity           int    `json:"engineCapacity"`
	CO2Emissions             int    `json:"co2Emissions"`
	FuelType                 string `json:"fuelType"`
	MarkedForExport          bool   `json:"markedForExport"`
	Colour                   string `json:"colour"`
	TypeApproval             string `json:"typeApproval"`
	EuroStatus               int    `json:"euroStatus"`
	DateOfLastV5CIssued      string `json:"dateOfLastV5CIssued"`
	MotExpiryDate            string `json:"motExpiryDate"`
	Wheelplan                string `json:"wheelplan"`
	MonthOfFirstRegistration string `json:"monthOfFirstRegistration"`
	// ... other fields not needed for the owner_id lookup
}

type BoundingBox struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

type Detection struct {
	Label       string      `json:"label"`
	Confidence  float64     `json:"confidence"`
	BoundingBox BoundingBox `json:"bounding_box"`
}

// OCR struct holds the registration ID (text)
type OCR struct {
	Text       string  `json:"text"` // This is the registration ID
	Confidence float64 `json:"confidence"`
}

// ALPRResult is an item in the alpr_results array
type ALPRResult struct {
	Detection Detection `json:"detection"`
	OCR       OCR       `json:"ocr"`
}

// ProcessImageResponse is the top-level response struct
type ProcessImageResponse struct {
	Message          string       `json:"message"`
	SizeBytes        int          `json:"size_bytes"`
	InferredMimeType string       `json:"inferred_mime_type"`
	ALPRResults      []ALPRResult `json:"alpr_results"` // The array we need to process
}

// ProcessImageRequest maps directly to the expected JSON body for your API
type ProcessImageRequest struct {
	// The key MUST be "image_base64" to match your API's requirement.
	// The value will contain the "data:image/png;base64,..." string.
	ImageBase64 string `json:"image_base64"`
}

// ----------------------------------------------------------------------
// --- API HELPER FUNCTION (WITH DUMPING) ---
// ----------------------------------------------------------------------

// In a real system, this would be a database lookup or another API call.
func mapRegistrationToOwnerID(regID string) string {
	// Dummy mapping: For any valid plate, return a dummy owner ID
	if len(regID) > 3 {
		return "OWNER-" + strings.ToUpper(regID)
	}
	return ""
}

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

// and immediately fails the connection attempt.
func noHostLookupDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	// The 'addr' typically contains "hostname:port".
	// By returning an error here, we stop the network stack before
	// it can initiate a DNS lookup for the 'hostname' part of 'addr'.

	return nil, fmt.Errorf("host lookup disabled: network connection to %s is prohibited", addr)
}

func safeDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// If SplitHostPort fails, treat the whole address as the host
		host = addr
	}

	// 1. Check if the host is a valid IP address.
	// If net.ParseIP returns a non-nil IP, it's an IP address.
	if net.ParseIP(host) != nil {
		// It's a direct IP address (e.g., "127.0.0.1" or "192.168.1.1").
		// Use the default Go dialer to attempt the connection directly.
		return (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 5 * time.Second,
		}).DialContext(ctx, network, addr)
	}

	// 2. If it's not a direct IP, it must be a hostname (like "google.com" or "alpnr.localhost").
	// We block all hostnames to prevent external DNS lookups.
	return nil, fmt.Errorf("hostname lookup blocked: connection attempt to '%s' is prohibited", host)
}

func GetClientWithHostnamesBlocked() *http.Client {
	transport := &http.Transport{
		DialContext: safeDialer,
		// ... standard settings
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

// Tool B: DVSA Vehicle Enquiry API
func getOwnerID(registrationID string, cfg *config.Config) (string, error) {
	if registrationID == "" {
		return "", fmt.Errorf("Tool B received empty registration ID")
	}

	// 1. Construct the URL using direct IP address to avoid lookup issues
	// We assume DVSA service is listening at the root of 127.0.0.1 (via Traefik/Ingress)
	// NOTE: Replace 127.0.0.1 with the correct K8s-exposed IP/Port if needed.
	toolBURL := fmt.Sprintf(cfg.DVSAAPIURL+"vehicle-enquiry/v1/vehicles/%s", registrationID)

	// Use the client that allows IP connections but blocks hostnames
	client := GetClientWithHostnamesBlocked()

	// 2. Create the GET request
	req, err := http.NewRequest("GET", toolBURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create Tool B request: %w", err)
	}

	// 3. Execute Request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute Tool B request: %w", err)
	}
	defer resp.Body.Close()

	// 4. Check Status Code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Tool B call failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	// 5. Decode Response
	var apiResponse VehicleResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return "", fmt.Errorf("failed to decode Tool B response: %w", err)
	}

	// 6. Map to Owner ID
	// Since the actual API doesn't return owner_id, we map the registration ID to a dummy owner ID.
	ownerID := mapRegistrationToOwnerID(apiResponse.RegistrationNumber)

	if ownerID == "" {
		return "", fmt.Errorf("Tool B: Failed to map registration %s to an owner ID", apiResponse.RegistrationNumber)
	}

	fmt.Printf("✅ Tool B: Vehicle details retrieved. Mapped to Owner ID: %s\n", ownerID)
	return ownerID, nil
}

// Tool A: External ALPR API
func processBase64Image(imageBase64 string, cfg *config.Config) (string, error) {
	// ... [Input validation and Request Payload building remain the same] ...

	fmt.Println(imageBase64)
	if imageBase64 == "" {
		return "", fmt.Errorf("invalid image data provided to Tool A")
	}

	requestPayload := ProcessImageRequest{
		ImageBase64: imageBase64,
	}

	var apiResponse ProcessImageResponse
	toolAURL := cfg.OpenALPRAPIURL + "/process-base64-image/"

	// --- Network Request Logic (same as before) ---
	reqData, err := json.Marshal(requestPayload)

	if err != nil {
		return "", fmt.Errorf("failed to marshal Tool A request: %w", err)
	}

	req, err := http.NewRequest("POST", toolAURL, bytes.NewBuffer(reqData))

	if err != nil {
		return "", fmt.Errorf("failed to create Tool A request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute Tool A request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Tool A call failed with status %d: %s", resp.StatusCode, string(responseBody))
	}
	// --- End Network Request Logic ---

	// 1. Decode Response
	if err := json.Unmarshal(responseBody, &apiResponse); err != nil {
		return "", fmt.Errorf("failed to decode Tool A response: %w", err)
	}

	// 2. Extract the Registration ID
	if len(apiResponse.ALPRResults) == 0 {
		// No license plates were detected
		return "", fmt.Errorf("Tool A failed to find any license plate results")
	}

	// Assume the first result is the best/only one
	registrationID := apiResponse.ALPRResults[0].OCR.Text

	if registrationID == "" {
		return "", fmt.Errorf("Tool A result was empty: ALPR found no readable text")
	}

	fmt.Printf("✅ Tool A: Image processed successfully. Registration ID: %s\n", registrationID)
	return strings.TrimSpace(registrationID), nil
}

// ----------------------------------------------------------------------
// --- MAIN EXECUTION ---
// ----------------------------------------------------------------------

func main() {

	cfg, err := config.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	err, _ = createMainChat(cfg, "What is the capital of France?")
	if err != nil {
		fmt.Println("Error in main chat flow:", err)
		os.Exit(1)
	}

}

func createMainChat(cfg *config.Config, prompt string) (error, string) {

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
