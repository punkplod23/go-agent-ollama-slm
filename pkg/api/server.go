package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"punkplod23/go-agent-ollama-slm/config"
	"punkplod23/go-agent-ollama-slm/pkg/tools"
	"punkplod23/go-agent-ollama-slm/pkg/webui"

	"github.com/gorilla/mux"
)

type CreateChatRequest struct {
	Prompt string `json:"prompt"`
}

func StartServer(cfg *config.Config) {
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/chat", createChatHandler(cfg)).Methods("POST")
	r.HandleFunc("/api/v1/files", addFileHandler(cfg)).Methods("POST")
	r.HandleFunc("/api/v1/process-base64-image", processBase64ImageHandler(cfg)).Methods("POST")

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

		err, resp := webui.CreateMainChat(cfg, req.Prompt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(resp)
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
