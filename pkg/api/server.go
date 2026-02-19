package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"punkplod23/go-agent-ollama-slm/config"
	"punkplod23/go-agent-ollama-slm/pkg/tools"
	"punkplod23/go-agent-ollama-slm/pkg/webui"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type CreateChatRequest struct {
	Prompt      string `json:"prompt"`
	Content     string `json:"content,omitempty"`
	KnowledgeID string `json:"knowledge_id,omitempty"`
	DocumentID  string `json:"document_id,omitempty"`
}

func StartServer(cfg *config.Config) {
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/chat", createChatHandler(cfg)).Methods("POST")
	r.HandleFunc("/api/v1/files", addFileHandler(cfg)).Methods("POST")
	r.HandleFunc("/api/v1/process-base64-image", processBase64ImageHandler(cfg)).Methods("POST")
	r.HandleFunc("/api/v1/vehicle-lookup", vehicleLookupHandler(cfg)).Methods("POST")

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("could not start server: %v", err)
	}
}

func createChatHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		knowledgeID := req.KnowledgeID

		// If content is provided, add it to the knowledge base
		if req.Content != "" {
			if knowledgeID == "" {
				http.Error(w, "knowledge_id is required when providing content", http.StatusBadRequest)
				return
			}
			// Use a unique name for the file to avoid conflicts.
			filename := fmt.Sprintf("chat-content-%s.md", uuid.New().String())
			DocumentID, err := webui.AddFileToKnowledgeCollection(req.Content, filename, knowledgeID, cfg)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to add content to knowledge collection: %v", err), http.StatusInternalServerError)
				return
			}
			req.DocumentID = DocumentID
		}

		chatID, err := webui.CreateMainChat(cfg, req.Prompt, knowledgeID, req.DocumentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"chat_id": chatID,
			"status":  "chat process initiated",
		})
	}
}

func addFileHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		file, handler, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Error Retrieving the File", http.StatusBadRequest)
			return
		}
		defer file.Close()

		knowledgeID := r.FormValue("knowledgeID")
		if knowledgeID == "" {
			http.Error(w, "knowledgeID is required", http.StatusBadRequest)
			return
		}

		// Create a temporary file
		tempFile, err := os.CreateTemp(cfg.TempDirPath, "upload-*.md")
		if err != nil {
			http.Error(w, "Could not create temporary file", http.StatusInternalServerError)
			return
		}
		defer os.Remove(tempFile.Name())

		// Read the content of the uploaded file
		fileBytes, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Could not read file content", http.StatusInternalServerError)
			return
		}

		// Write the content to the temporary file
		if _, err := tempFile.Write(fileBytes); err != nil {
			http.Error(w, "Could not write to temporary file", http.StatusInternalServerError)
			return
		}

		fileID, err := webui.AddFileToKnowledgeCollection(string(fileBytes), handler.Filename, knowledgeID, cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"fileID": fileID})
	}
}

func processBase64ImageHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ImageBase64 string `json:"image_base64"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		regID, err := tools.ProcessBase64Image(req.ImageBase64, cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"registration_id": regID})
	}
}

func vehicleLookupHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RegistrationID string `json:"registration_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("vehicleLookupHandler: error decoding request body: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("vehicleLookupHandler: looking up registration ID: %s", req.RegistrationID)
		ownerID, err := tools.GetOwnerID(req.RegistrationID, cfg)
		if err != nil {
			log.Printf("vehicleLookupHandler: error getting owner ID: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("vehicleLookupHandler: successfully retrieved owner ID: %s", ownerID)
		json.NewEncoder(w).Encode(map[string]string{"owner_id": ownerID})
	}
}
