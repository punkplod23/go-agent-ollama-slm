package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"punkplod23/go-agent-ollama-slm/config"
	"strings"
	"time"
)

// --- STRUCTS: Tool-related API Models ---

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

// In a real system, this would be a database lookup or another API call.
func mapRegistrationToOwnerID(regID string) string {
	// Dummy mapping: For any valid plate, return a dummy owner ID
	if len(regID) > 3 {
		return "OWNER-" + strings.ToUpper(regID)
	}
	return ""
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
func GetOwnerID(registrationID string, cfg *config.Config) (string, error) {
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
func ProcessBase64Image(imageBase64 string, cfg *config.Config) (string, error) {
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
