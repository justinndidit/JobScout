# Job Aggregator Agent

An intelligent A2A (Agent-to-Agent) protocol-compliant agent that searches and aggregates remote job listings from multiple sources including Remotive and Jobicy.

## 🌟 Features

- **Multi-Source Aggregation**: Searches jobs from Remotive and Jobicy APIs simultaneously
- **Natural Language Processing**: Uses Google Gemini AI to parse job search queries
- **A2A Protocol Compliant**: Fully compatible with Telex and other A2A platforms
- **Async Task Processing**: Non-blocking job searches with task status tracking
- **Smart Filtering**: Extracts role, location, experience level, and work type from queries
- **Unified Response Format**: Normalizes job data from different sources

## 🚀 Quick Start

### Prerequisites

- Go 1.21 or higher
- Google Gemini API key

### Installation

1. Clone the repository:
```bash
git clone https://github.com/yourusername/job-aggregator-agent.git
cd job-aggregator-agent
```

2. Install dependencies:
```bash
go mod download
```

3. Set up environment variables:
```bash
cp .env.example .env
# Edit .env and add your GEMINI_API_KEY
```

4. Run the agent:
```bash
go run main.go
```

The agent will start on `http://localhost:8080`

## 📋 Environment Variables

```bash
# Required
GEMINI_API_KEY=your_gemini_api_key_here

# Optional
PORT=8080                                    # Server port (default: 8080)
SERVICE_URL=https://your-domain.com          # Public URL for agent card
```

## 🔧 API Endpoints

### Agent Card Discovery
```bash
GET /.well-known/agent-card
```

Returns the agent's capabilities and configuration in A2A format.

### Health Check
```bash
GET /health
```

Returns the agent's health status.

### A2A JSON-RPC Endpoint
```bash
POST /a2a
Content-Type: application/json
```

Handles A2A protocol requests:
- `message/send` - Submit a job search query
- `tasks/get` - Retrieve task status and results

## 💡 Usage Examples

### Using cURL

**1. Submit a job search:**
```bash
curl -X POST http://localhost:8080/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "message/send",
    "id": "req-123",
    "params": {
      "role": "user",
      "parts": [
        {
          "kind": "text",
          "text": "Find Python remote jobs in Canada"
        }
      ],
      "messageId": "msg-456",
      "kind": "message"
    }
  }'
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": "req-123",
  "result": {
    "id": "task_1234567890",
    "state": "working",
    "timestamp": "2025-10-30T19:06:08Z"
  }
}
```

**2. Check task status:**
```bash
curl -X POST http://localhost:8080/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tasks/get",
    "id": "req-124",
    "params": {
      "taskId": "task_1234567890"
    }
  }'
```

Response (when completed):
```json
{
  "jsonrpc": "2.0",
  "id": "req-124",
  "result": {
    "id": "task_1234567890",
    "state": "completed",
    "message": {
      "role": "agent",
      "parts": [
        {
          "kind": "text",
          "text": "🎯 Found 25 remote jobs for 'Python' in Canada:\n\n..."
        },
        {
          "kind": "data",
          "data": [...array of job objects...]
        }
      ]
    }
  }
}
```

### Natural Language Queries

The agent understands various query formats:

- "Find Python remote jobs in Canada"
- "Show me Go developer positions"
- "JavaScript jobs in Europe"
- "Senior backend engineer roles"
- "Remote DevOps jobs in USA"

## 🏗️ Architecture

```
┌─────────────────┐
│   User Query    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Gemini AI      │ ◄── Parses intent
│  Intent Parser  │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────┐
│   Parallel API Calls        │
├──────────────┬──────────────┤
│  Remotive    │   Jobicy     │
└──────┬───────┴──────┬───────┘
       │              │
       ▼              ▼
┌─────────────────────────────┐
│   Result Aggregation        │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────┐
│  Unified Jobs   │
└─────────────────┘
```



## 🔗 Connecting to Telex

1. Deploy your agent to a public URL
2. Verify agent card is accessible: `https://your-url.com/.well-known/agent-card`
3. Visit [Telex.im](https://telex.im)
4. Navigate to Settings → Agents → Add Agent
5. Enter your agent card URL
6. Telex will automatically discover and register your agent

## 📊 Job Sources

### Remotive
- API: `https://remotive.com/api/remote-jobs`
- Features: Salary info, detailed descriptions, job categories
- Coverage: Global remote positions

### Jobicy
- API: `https://jobicy.com/api/v2/remote-jobs`
- Features: Industry tags, job levels, geographic filtering
- Coverage: Worldwide remote jobs


## 🛠️ Development

### Project Structure

```
.
├── main.go              # Main agent implementation
├── go.mod               # Go dependencies
├── go.sum               # Dependency checksums
├── .env                 # Environment variables (not committed)
└── README.md            # This file
```

### Adding New Job Sources

To add a new job source:

1. Create a struct for the API response
2. Implement a search function (e.g., `searchNewSource()`)
3. Convert results to unified `Job` format
4. Add to `searchJobsFromAllSources()` goroutine pool
5. Update agent card capabilities

## 📝 A2A Protocol

This agent implements the [A2A (Agent-to-Agent) Protocol](https://a2a.ai/):

- **JSON-RPC 2.0**: Standard RPC format
- **Agent Card**: Capability discovery at `/.well-known/agent-card`
- **Task Lifecycle**: submitted → working → completed/failed
- **Message Parts**: Text and structured data responses

## 🐛 Troubleshooting

### Agent returns "Parse error"
- Check that GEMINI_API_KEY is set correctly
- Verify API key has correct permissions

### "Failed to search jobs"
- Check internet connectivity
- Verify job APIs (Remotive/Jobicy) are accessible
- Check logs for specific API errors

### Task stuck in "working" state
- Check agent logs for errors
- Verify Gemini API is responding
- Ensure job APIs are not rate-limiting


## 📄 License

MIT License - feel free to use this agent in your projects!

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Submit a pull request

## 📧 Contact

For questions or issues, please open a GitHub issue or contact [your-email@example.com]

## 🙏 Acknowledgments

- [A2A Protocol](https://a2a.ai/) for the agent-to-agent specification
- [Telex](https://telex.im) for the collaboration platform
- [Remotive](https://remotive.com) and [Jobicy](https://jobicy.com) for job APIs
- [Google Gemini](https://ai.google.dev/) for natural language processing

---

Built with ❤️ using Go and the A2A Protocol
