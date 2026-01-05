Here are two examples of how to test the API using cURL:

**1. Test the chat endpoint:**

This command sends a POST request to the `/api/v1/chat` endpoint with a JSON payload containing the prompt.

```bash
curl -X POST http://localhost:8080/api/v1/chat -H "Content-Type: application/json" -d '{"prompt": "What is the capital of France?"}'
```

**2. Test the file upload endpoint:**

This command sends a POST request to the `/api/v1/files` endpoint with a multipart/form-data payload. It uploads the `test.md` file and sets the `knowledgeID`.

**Important:** Replace `YOUR_KNOWLEDGE_ID` with an actual knowledge base ID from your Open WebUI instance.

```bash
curl -X POST http://localhost:8080/api/v1/files -F "file=@test.md" -F "knowledgeID=YOUR_KNOWLEDGE_ID"
```
