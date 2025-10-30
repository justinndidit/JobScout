package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"google.golang.org/genai"
)

// A2A Protocol Structures (JSON-RPC 2.0)
type A2ARequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      string      `json:"id"`
}

type A2AResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *A2AError   `json:"error,omitempty"`
}

type A2AError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// A2A Message Structure
type Message struct {
	Role             string                 `json:"role"`
	Parts            []Part                 `json:"parts"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Extensions       []string               `json:"extensions,omitempty"`
	ReferenceTaskIds []string               `json:"referenceTaskIds,omitempty"`
	MessageID        string                 `json:"messageId"`
	TaskID           string                 `json:"taskId,omitempty"`
	ContextID        string                 `json:"contextId,omitempty"`
	Kind             string                 `json:"kind"`
}

type Part struct {
	Kind     string                 `json:"kind"`
	Text     string                 `json:"text,omitempty"`
	Data     interface{}            `json:"data,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// A2A Task Structure
type Task struct {
	ID        string                 `json:"id"`
	State     string                 `json:"state"`
	Message   *Message               `json:"message,omitempty"`
	Timestamp string                 `json:"timestamp,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Agent Card (for A2A discovery)
type AgentCard struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	Capabilities []string          `json:"capabilities"`
	Skills       []Skill           `json:"skills"`
	ServiceURL   string            `json:"serviceUrl"`
	Auth         map[string]string `json:"auth,omitempty"`
}

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// Job Structures
type JobIntent struct {
	Role            string   `json:"role"`
	Location        []string `json:"location"`
	WorkType        *string  `json:"work_type"`
	ExperienceLevel *string  `json:"experience_level"`
}

// Unified Job structure
type Job struct {
	ID            interface{} `json:"id"`
	URL           string      `json:"url"`
	Title         string      `json:"title"`
	CompanyName   string      `json:"company_name"`
	CompanyLogo   string      `json:"company_logo,omitempty"`
	Category      string      `json:"category,omitempty"`
	Tags          []string    `json:"tags,omitempty"`
	JobType       string      `json:"job_type,omitempty"`
	Location      string      `json:"location"`
	Level         string      `json:"level,omitempty"`
	Salary        string      `json:"salary,omitempty"`
	Excerpt       string      `json:"excerpt,omitempty"`
	Description   string      `json:"description,omitempty"`
	PublishedDate string      `json:"published_date"`
	Source        string      `json:"source"`
}

// Jobicy Response
type JobicyResponse struct {
	JobCount int         `json:"job_count"`
	Jobs     []JobicyJob `json:"jobs"`
}

type JobicyJob struct {
	ID             int      `json:"id"`
	URL            string   `json:"url"`
	JobTitle       string   `json:"jobTitle"`
	CompanyName    string   `json:"companyName"`
	CompanyLogo    string   `json:"companyLogo"`
	JobIndustry    []string `json:"jobIndustry"`
	JobType        []string `json:"jobType"`
	JobGeo         string   `json:"jobGeo"`
	JobLevel       string   `json:"jobLevel"`
	JobExcerpt     string   `json:"jobExcerpt"`
	JobDescription string   `json:"jobDescription"`
	PubDate        string   `json:"pubDate"`
}

// Remotive Response
type RemotiveResponse struct {
	JobCount      int           `json:"job-count"`
	TotalJobCount int           `json:"total-job-count"`
	Jobs          []RemotiveJob `json:"jobs"`
}

type RemotiveJob struct {
	ID                        int      `json:"id"`
	URL                       string   `json:"url"`
	Title                     string   `json:"title"`
	CompanyName               string   `json:"company_name"`
	CompanyLogo               string   `json:"company_logo"`
	Category                  string   `json:"category"`
	Tags                      []string `json:"tags"`
	JobType                   string   `json:"job_type"`
	PublicationDate           string   `json:"publication_date"`
	CandidateRequiredLocation string   `json:"candidate_required_location"`
	Salary                    string   `json:"salary"`
	Description               string   `json:"description"`
}

// Task storage for tracking async tasks
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewTaskStore() *TaskStore {
	return &TaskStore{
		tasks: make(map[string]*Task),
	}
}

func (ts *TaskStore) Set(task *Task) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tasks[task.ID] = task
}

func (ts *TaskStore) Get(id string) (*Task, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	task, ok := ts.tasks[id]
	return task, ok
}

var taskStore = NewTaskStore()

// Global agent configuration
var agentCard = AgentCard{
	Name:        "Job Aggregator Agent",
	Description: "AI agent that searches and aggregates remote job listings from multiple sources (Jobicy, Remotive)",
	Version:     "1.1.0",
	Capabilities: []string{
		"job_search",
		"intent_parsing",
		"multi_source_aggregation",
	},
	Skills: []Skill{
		{
			Name:        "parse_job_intent",
			Description: "Parse natural language job queries into structured search parameters",
			Type:        "llm_powered",
		},
		{
			Name:        "search_jobicy",
			Description: "Search remote jobs from Jobicy API",
			Type:        "api_integration",
		},
		{
			Name:        "search_remotive",
			Description: "Search remote jobs from Remotive API",
			Type:        "api_integration",
		},
		{
			Name:        "aggregate_results",
			Description: "Combine and deduplicate results from multiple job sources",
			Type:        "data_processing",
		},
	},
	ServiceURL: getServiceURL(),
}

func getServiceURL() string {
	baseURL := os.Getenv("SERVICE_URL")
	if baseURL == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		baseURL = fmt.Sprintf("http://localhost:%s", port)
	}
	return strings.TrimRight(baseURL, "/") + "/a2a"
}

func main() {
	// Set up HTTP server for A2A protocol
	http.HandleFunc("/.well-known/agent-card", handleAgentCard)
	http.HandleFunc("/a2a", handleA2ARequest)
	http.HandleFunc("/health", handleHealth)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Job Aggregator Agent starting on port %s...", port)
	log.Printf("Agent Card: http://localhost:%s/.well-known/agent-card", port)
	log.Printf("Health Check: http://localhost:%s/health", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// Health check endpoint
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"version": agentCard.Version,
		"time":    time.Now().Format(time.RFC3339),
		"sources": []string{"jobicy", "remotive"},
	})
}

// Handle Agent Card discovery (A2A standard)
func handleAgentCard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(agentCard); err != nil {
		log.Printf("Error encoding agent card: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Handle A2A JSON-RPC requests
func handleA2ARequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req A2ARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding request: %v", err)
		respondWithError(w, "", -32700, "Parse error", err.Error())
		return
	}

	log.Printf("Received A2A request: method=%s, id=%s", req.Method, req.ID)

	// Route to appropriate handler based on method
	switch req.Method {
	case "message/send":
		handleMessageSend(w, req)
	case "tasks/get":
		handleTasksGet(w, req)
	default:
		respondWithError(w, req.ID, -32601, "Method not found", fmt.Sprintf("Unknown method: %s", req.Method))
	}
}

// Handle message/send method (main task execution)
func handleMessageSend(w http.ResponseWriter, req A2ARequest) {
	// Parse message from params
	paramsBytes, _ := json.Marshal(req.Params)
	var msg Message
	if err := json.Unmarshal(paramsBytes, &msg); err != nil {
		respondWithError(w, req.ID, -32602, "Invalid params", err.Error())
		return
	}

	// Extract user query from message parts
	userQuery := ""
	for _, part := range msg.Parts {
		if part.Kind == "text" {
			userQuery = part.Text
			break
		}
	}

	if userQuery == "" {
		respondWithError(w, req.ID, -32602, "Invalid params", "No text content in message")
		return
	}

	// Generate task ID
	taskID := msg.TaskID
	if taskID == "" {
		taskID = fmt.Sprintf("task_%d", time.Now().UnixNano())
	}

	// Create initial task in "working" state
	task := &Task{
		ID:        taskID,
		State:     "working",
		Timestamp: time.Now().Format(time.RFC3339),
		Metadata: map[string]interface{}{
			"query": userQuery,
		},
	}
	taskStore.Set(task)

	// Process asynchronously
	go processJobSearch(taskID, userQuery, msg, req.ID)

	// Return immediate response with working state
	respondWithSuccess(w, req.ID, task)
}

// Process job search asynchronously
func processJobSearch(taskID, userQuery string, originalMsg Message, requestID string) {
	ctx := context.Background()

	// Step 1: Parse intent using LLM
	intent, err := parseJobIntent(ctx, userQuery)
	if err != nil {
		updateTaskError(taskID, fmt.Sprintf("Failed to parse intent: %v", err))
		sendWebhookIfConfigured(taskID, originalMsg.Metadata)
		return
	}

	// Step 2: Search jobs from multiple sources
	jobs, err := searchJobsFromAllSources(intent)
	if err != nil {
		updateTaskError(taskID, fmt.Sprintf("Failed to search jobs: %v", err))
		sendWebhookIfConfigured(taskID, originalMsg.Metadata)
		return
	}

	// Step 3: Format response
	responseText := formatJobResults(jobs, intent)

	// Update task with completed state
	task := &Task{
		ID:    taskID,
		State: "completed",
		Message: &Message{
			Role: "agent",
			Parts: []Part{
				{
					Kind: "text",
					Text: responseText,
				},
				{
					Kind: "data",
					Data: jobs,
					Metadata: map[string]interface{}{
						"job_count": len(jobs),
						"intent":    intent,
						"sources":   []string{"jobicy", "remotive"},
					},
				},
			},
			MessageID: fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			TaskID:    taskID,
			ContextID: originalMsg.ContextID,
			Kind:      "message",
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Metadata: map[string]interface{}{
			"completed_at": time.Now().Format(time.RFC3339),
		},
	}
	taskStore.Set(task)
	log.Printf("Task %s completed with %d jobs", taskID, len(jobs))

	// Send webhook notification if configured
	sendWebhookIfConfigured(taskID, originalMsg.Metadata)
}

// Send webhook notification when task is complete
func sendWebhookIfConfigured(taskID string, metadata map[string]interface{}) {
	if metadata == nil {
		return
	}

	webhookURL, ok := metadata["webhookUrl"].(string)
	if !ok || webhookURL == "" {
		return
	}

	// Get the completed task
	task, exists := taskStore.Get(taskID)
	if !exists {
		log.Printf("Task %s not found for webhook", taskID)
		return
	}

	// Send webhook in background (don't block)
	go func() {
		log.Printf("Sending webhook to %s for task %s", webhookURL, taskID)

		payload := map[string]interface{}{
			"taskId":    task.ID,
			"state":     task.State,
			"message":   task.Message,
			"timestamp": task.Timestamp,
			"metadata":  task.Metadata,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to marshal webhook payload: %v", err)
			return
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Failed to send webhook: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("Webhook sent successfully to %s", webhookURL)
		} else {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("Webhook failed with status %d: %s", resp.StatusCode, string(body))
		}
	}()
}

func updateTaskError(taskID, errorMsg string) {
	task := &Task{
		ID:        taskID,
		State:     "failed",
		Timestamp: time.Now().Format(time.RFC3339),
		Message: &Message{
			Role: "agent",
			Parts: []Part{
				{
					Kind: "text",
					Text: errorMsg,
				},
			},
			MessageID: fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			TaskID:    taskID,
			Kind:      "message",
		},
		Metadata: map[string]interface{}{
			"error":     errorMsg,
			"failed_at": time.Now().Format(time.RFC3339),
		},
	}
	taskStore.Set(task)
	log.Printf("Task %s failed: %s", taskID, errorMsg)
}

// Handle tasks/get method
func handleTasksGet(w http.ResponseWriter, req A2ARequest) {
	paramsBytes, _ := json.Marshal(req.Params)
	var params map[string]interface{}
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		respondWithError(w, req.ID, -32602, "Invalid params", err.Error())
		return
	}

	taskID, ok := params["taskId"].(string)
	if !ok || taskID == "" {
		respondWithError(w, req.ID, -32602, "Invalid params", "taskId is required")
		return
	}

	task, exists := taskStore.Get(taskID)
	if !exists {
		respondWithError(w, req.ID, -32000, "Task not found", fmt.Sprintf("Task %s does not exist", taskID))
		return
	}

	respondWithSuccess(w, req.ID, task)
}

// Parse job intent using LLM
func parseJobIntent(ctx context.Context, query string) (*JobIntent, error) {
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	prompt := fmt.Sprintf(`
You are a job intent parser. Extract the job type, location, and level from the user's message.
Return only valid JSON like this:
{"role": "...", "location": ["..."], "work_type": "...", "experience_level": "..."}.
If a field is not mentioned, use null for work_type and experience_level, or an empty array for location.
User message is: "%s"
`, query)

	result, err := client.Models.GenerateContent(ctx, "gemini-2.0-flash-exp", genai.Text(prompt), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	// Clean and parse response
	value := result.Text()
	value = strings.TrimPrefix(value, "```json\n")
	value = strings.TrimPrefix(value, "```json")
	value = strings.TrimSuffix(value, "\n```")
	value = strings.TrimSuffix(value, "```")
	value = strings.TrimSpace(value)

	var intent JobIntent
	if err := json.Unmarshal([]byte(value), &intent); err != nil {
		return nil, fmt.Errorf("failed to parse intent JSON: %w", err)
	}

	log.Printf("Parsed intent: %+v", intent)
	return &intent, nil
}

// Search jobs from all sources
func searchJobsFromAllSources(intent *JobIntent) ([]Job, error) {
	var allJobs []Job
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// Search Remotive (primary source now)
	wg.Add(1)
	go func() {
		defer wg.Done()
		jobs, err := searchRemotive(intent)
		if err != nil {
			log.Printf("Remotive search error: %v", err)
			errChan <- fmt.Errorf("remotive: %w", err)
			return
		}
		mu.Lock()
		allJobs = append(allJobs, jobs...)
		mu.Unlock()
		log.Printf("Found %d jobs from Remotive", len(jobs))
	}()

	// Search Jobicy (backup source)
	wg.Add(1)
	go func() {
		defer wg.Done()
		jobs, err := searchJobicy(intent)
		if err != nil {
			log.Printf("Jobicy search error: %v", err)
			errChan <- fmt.Errorf("jobicy: %w", err)
			return
		}
		mu.Lock()
		allJobs = append(allJobs, jobs...)
		mu.Unlock()
		log.Printf("Found %d jobs from Jobicy", len(jobs))
	}()

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	// If both sources failed, return error
	if len(errs) == 2 {
		return nil, fmt.Errorf("all sources failed: %v", errs)
	}

	log.Printf("Total jobs found: %d", len(allJobs))
	return allJobs, nil
}

// Search Remotive API
func searchRemotive(intent *JobIntent) ([]Job, error) {
	apiURL := "https://remotive.com/api/remote-jobs"

	// Add query parameters if needed
	params := url.Values{}
	if intent.Role != "" {
		// Remotive supports search parameter
		params.Add("search", intent.Role)
	}
	if len(intent.Location) > 0 && intent.Location[0] != "" {
		params.Add("location", intent.Location[0])
	}

	if len(params) > 0 {
		apiURL += "?" + params.Encode()
	}

	log.Printf("Querying Remotive API: %s", apiURL)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "TelexJobAgent/1.1")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var remotiveResp RemotiveResponse
	if err := json.Unmarshal(body, &remotiveResp); err != nil {
		return nil, err
	}

	// Convert to unified Job format
	jobs := make([]Job, 0, len(remotiveResp.Jobs))
	for _, rJob := range remotiveResp.Jobs {
		// Filter by tags if role is specified
		if intent.Role != "" && !matchesRole(rJob.Tags, intent.Role) {
			continue
		}

		job := Job{
			ID:            rJob.ID,
			URL:           rJob.URL,
			Title:         rJob.Title,
			CompanyName:   rJob.CompanyName,
			CompanyLogo:   rJob.CompanyLogo,
			Category:      rJob.Category,
			Tags:          rJob.Tags,
			JobType:       rJob.JobType,
			Location:      rJob.CandidateRequiredLocation,
			Salary:        rJob.Salary,
			Description:   stripHTML(rJob.Description),
			Excerpt:       extractExcerpt(rJob.Description),
			PublishedDate: rJob.PublicationDate,
			Source:        "Remotive",
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// Search Jobicy API
func searchJobicy(intent *JobIntent) ([]Job, error) {
	queryString := toJobicyQueryString(intent)
	apiURL := fmt.Sprintf("https://jobicy.com/api/v2/remote-jobs?%s", queryString)
	log.Printf("Querying Jobicy API: %s", apiURL)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "TelexJobAgent/1.1")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jobicyResp JobicyResponse
	if err := json.Unmarshal(body, &jobicyResp); err != nil {
		return nil, err
	}

	// Convert to unified Job format
	jobs := make([]Job, 0, len(jobicyResp.Jobs))
	for _, jJob := range jobicyResp.Jobs {
		jobType := ""
		if len(jJob.JobType) > 0 {
			jobType = jJob.JobType[0]
		}

		job := Job{
			ID:            jJob.ID,
			URL:           jJob.URL,
			Title:         jJob.JobTitle,
			CompanyName:   jJob.CompanyName,
			CompanyLogo:   jJob.CompanyLogo,
			Tags:          jJob.JobIndustry,
			JobType:       jobType,
			Location:      jJob.JobGeo,
			Level:         jJob.JobLevel,
			Excerpt:       jJob.JobExcerpt,
			Description:   jJob.JobDescription,
			PublishedDate: jJob.PubDate,
			Source:        "Jobicy",
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// Convert intent to Jobicy query string
func toJobicyQueryString(intent *JobIntent) string {
	params := url.Values{}
	params.Add("count", "10")

	if intent.Role != "" {
		tag := extractTag(intent.Role)
		if tag != "" {
			params.Add("tag", tag)
		}
	}

	if len(intent.Location) > 0 && intent.Location[0] != "" {
		geo := normalizeGeo(intent.Location[0])
		if geo != "" && geo != "remote" {
			params.Add("geo", geo)
		}
	}

	if intent.WorkType != nil && *intent.WorkType != "" {
		industry := normalizeIndustry(*intent.WorkType)
		if industry != "" {
			params.Add("industry", industry)
		}
	}

	return params.Encode()
}

// Helper: Match role against tags
func matchesRole(tags []string, role string) bool {
	role = strings.ToLower(role)
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), role) {
			return true
		}
	}
	return false
}

// Helper: Strip HTML tags
func stripHTML(html string) string {
	// Simple HTML stripping (for production, use a proper HTML parser)
	result := html
	result = strings.ReplaceAll(result, "<br>", "\n")
	result = strings.ReplaceAll(result, "<br/>", "\n")
	result = strings.ReplaceAll(result, "<br />", "\n")
	result = strings.ReplaceAll(result, "</p>", "\n")

	// Remove all tags
	for strings.Contains(result, "<") && strings.Contains(result, ">") {
		start := strings.Index(result, "<")
		end := strings.Index(result, ">")
		if start < end {
			result = result[:start] + result[end+1:]
		} else {
			break
		}
	}
	return strings.TrimSpace(result)
}

// Helper: Extract excerpt from description
func extractExcerpt(description string) string {
	plain := stripHTML(description)
	if len(plain) > 200 {
		return plain[:200] + "..."
	}
	return plain
}

// Format job results for response
func formatJobResults(jobs []Job, intent *JobIntent) string {
	if len(jobs) == 0 {
		return "No jobs found matching your criteria."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎯 Found %d remote jobs", len(jobs)))

	if intent.Role != "" {
		sb.WriteString(fmt.Sprintf(" for '%s'", intent.Role))
	}

	if len(intent.Location) > 0 && intent.Location[0] != "" && intent.Location[0] != "remote" {
		sb.WriteString(fmt.Sprintf(" in %s", intent.Location[0]))
	}

	sb.WriteString(":\n\n")

	displayCount := len(jobs)
	if displayCount > 15 {
		displayCount = 15
	}

	for i := 0; i < displayCount; i++ {
		job := jobs[i]
		sb.WriteString(fmt.Sprintf("%d. **%s** at %s\n", i+1, job.Title, job.CompanyName))
		sb.WriteString(fmt.Sprintf("   📍 %s", job.Location))

		if job.Level != "" {
			sb.WriteString(fmt.Sprintf(" | 📊 %s", job.Level))
		}

		if job.Salary != "" {
			sb.WriteString(fmt.Sprintf(" | 💰 %s", job.Salary))
		}

		if job.JobType != "" {
			sb.WriteString(fmt.Sprintf(" | 💼 %s", job.JobType))
		}

		sb.WriteString(fmt.Sprintf(" | 🔖 %s", job.Source))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("   🔗 %s\n", job.URL))

		if job.Excerpt != "" {
			excerpt := strings.TrimSpace(job.Excerpt)
			if len(excerpt) > 150 {
				excerpt = excerpt[:150] + "..."
			}
			sb.WriteString(fmt.Sprintf("   💡 %s\n", excerpt))
		}
		sb.WriteString("\n")
	}

	if len(jobs) > 15 {
		sb.WriteString(fmt.Sprintf("... and %d more jobs (see full data for complete list)\n", len(jobs)-15))
	}

	return sb.String()
}

// Helper functions
func extractTag(role string) string {
	role = strings.ToLower(role)
	keywords := []string{
		"python", "javascript", "java", "go", "golang", "rust", "ruby",
		"php", "typescript", "react", "node", "nodejs", "angular", "vue",
		"ios", "android", "swift", "kotlin", "c++", "cpp", "c#", "csharp",
		"devops", "backend", "frontend", "fullstack", "full-stack",
	}
	for _, keyword := range keywords {
		if strings.Contains(role, keyword) {
			if keyword == "golang" {
				return "go"
			}
			return keyword
		}
	}
	return role
}

func normalizeGeo(location string) string {
	location = strings.ToLower(strings.TrimSpace(location))
	geoMap := map[string]string{
		"remote": "", "usa": "usa", "us": "usa", "united states": "usa",
		"canada": "canada", "uk": "uk", "united kingdom": "uk",
		"europe": "europe", "asia": "asia", "australia": "australia",
	}
	if geo, ok := geoMap[location]; ok {
		return geo
	}
	return location
}

func normalizeIndustry(workType string) string {
	workType = strings.ToLower(strings.TrimSpace(workType))
	industryMap := map[string]string{
		"copywriting": "copywriting", "support": "supporting",
		"customer support": "supporting", "design": "design",
		"devops": "devops", "marketing": "marketing",
	}
	if industry, ok := industryMap[workType]; ok {
		return industry
	}
	return workType
}

// Response helpers
func respondWithSuccess(w http.ResponseWriter, id string, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(A2AResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}); err != nil {
		log.Printf("Error encoding success response: %v", err)
	}
}

func respondWithError(w http.ResponseWriter, id string, code int, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // JSON-RPC errors still return 200
	if err := json.NewEncoder(w).Encode(A2AResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &A2AError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}); err != nil {
		log.Printf("Error encoding error response: %v", err)
	}
}
