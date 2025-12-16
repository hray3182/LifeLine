package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sashabaranov/go-openai"
)

type Client struct {
	client *openai.Client
	model  string
}

func New(apiKey, baseURL, model string) *Client {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL

	return &Client{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

func (c *Client) SetModel(model string) {
	c.model = model
}

type Intent struct {
	Action             string            `json:"action"`
	Entity             string            `json:"entity"`
	Parameters         map[string]string `json:"parameters"`
	Confidence         float64           `json:"confidence"`
	NeedsConfirmation  bool              `json:"needs_confirmation"`
	ConfirmationReason string            `json:"confirmation_reason"`
	// Multi-turn conversation fields
	NeedMoreInfo   bool   `json:"need_more_info"`
	FollowUpPrompt string `json:"follow_up_prompt"`
	AIMessage      string `json:"ai_message"`
	RawResponse    string `json:"-"`
}

// Message represents a chat message for multi-turn conversations
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const systemPromptTemplate = `你是 LifeLine 的智慧助理，負責解析用戶的自然語言輸入並轉換為結構化的意圖。

當前時間: %s

可用的 action:
- create_memo: 建立備忘錄
- list_memo: 列出備忘錄 (可帶 keyword 搜尋)
- delete_memo: 刪除備忘錄
- create_todo: 建立待辦事項
- list_todo: 列出待辦事項 (可帶 keyword 搜尋)
- complete_todo: 完成待辦事項
- delete_todo: 刪除待辦事項
- update_todo: 更新待辦事項
- create_reminder: 建立提醒
- list_reminder: 列出提醒 (可帶 keyword 搜尋)
- delete_reminder: 刪除提醒
- create_expense: 記錄支出
- create_income: 記錄收入
- list_transaction: 列出交易記錄 (可帶 keyword 搜尋)
- delete_transaction: 刪除交易記錄
- get_balance: 查看收支統計
- create_event: 建立事件
- list_event: 列出事件 (可帶 keyword 搜尋)
- delete_event: 刪除事件
- update_event: 更新事件
- unknown: 無法識別

根據 action 類型，parameters 可能包含：
- id: 項目編號 (用於刪除、更新、完成操作)
- keyword: 搜尋關鍵字 (用於 list_* 操作，搜尋標題、內容、描述、標籤)
- content: 內容 (memo)
- title: 標題 (todo, event)
- description: 描述
- priority: 優先級 (1-5)
- due_time: 截止時間 (格式: YYYY-MM-DD HH:MM)
- remind_at: 提醒時間 (格式: YYYY-MM-DD HH:MM)
- amount: 金額
- category: 分類
- start_time: 開始時間 (格式: YYYY-MM-DD HH:MM)
- end_time: 結束時間 (格式: YYYY-MM-DD HH:MM)
- tags: 標籤

重要規則：
1. 當用戶使用相對時間（如「明天」、「下週一」、「3 小時後」），請根據當前時間計算出具體的日期時間，並以 YYYY-MM-DD HH:MM 格式輸出。

2. 以下情況必須設定 needs_confirmation = true：
   - 刪除操作 (delete_*)：任何刪除都需要確認
   - 更新操作 (update_*)：任何更新都需要確認
   - 時間模糊：深夜時段 (00:00-06:00) 用戶說「明天」，需確認是指今天還是隔天
   - 金額較大：支出或收入超過 10000 時需要確認

3. confirmation_reason 應簡潔說明需要確認的原因，例如：
   - "確認刪除待辦事項 #3？"
   - "現在是凌晨 2 點，「明天」是指 12/17 還是 12/18？"
   - "確認記錄支出 50000 元？"

4. 多輪對話規則：
   - 當用戶的請求資訊不足以執行操作時，設定 need_more_info = true
   - follow_up_prompt: 向用戶追問的問題（僅在 need_more_info = true 時設定）
   - ai_message: 給用戶的友善回覆訊息，可用於：
     * 追問更多資訊時的問題
     * 操作完成後的確認訊息（系統會附加操作結果）
     * 閒聊回覆（action = unknown 時）

   需要追問的情況範例：
   - 用戶說「刪除那個待辦」但沒有指定 ID → 追問「請問要刪除哪一個待辦事項？可以告訴我編號嗎？」
   - 用戶說「記一筆花費」但沒有說金額 → 追問「請問花了多少錢？」
   - 用戶說「提醒我」但沒說時間和內容 → 追問「請問要提醒什麼？什麼時候提醒？」

5. 當收到工具執行結果時，你需要：
   - 解讀結果並組織成友善的回覆
   - 如果結果需要用戶選擇（如搜尋到多筆記錄），引導用戶選擇
   - 如果操作失敗，解釋原因並建議下一步`

func getSystemPrompt() string {
	now := time.Now()
	return fmt.Sprintf(systemPromptTemplate, now.Format("2006-01-02 15:04 (Monday)"))
}

// JSON Schema for structured output
var intentSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"action": {
			"type": "string",
			"enum": ["create_memo", "list_memo", "delete_memo", "create_todo", "list_todo", "complete_todo", "delete_todo", "update_todo", "create_reminder", "list_reminder", "delete_reminder", "create_expense", "create_income", "list_transaction", "delete_transaction", "get_balance", "create_event", "list_event", "delete_event", "update_event", "unknown"],
			"description": "The action to perform"
		},
		"entity": {
			"type": "string",
			"description": "The entity type related to the action"
		},
		"parameters": {
			"type": "object",
			"additionalProperties": {
				"type": "string"
			},
			"description": "Parameters for the action including id for delete/update operations"
		},
		"confidence": {
			"type": "number",
			"minimum": 0,
			"maximum": 1,
			"description": "Confidence score between 0 and 1"
		},
		"needs_confirmation": {
			"type": "boolean",
			"description": "Whether this action requires user confirmation before execution"
		},
		"confirmation_reason": {
			"type": "string",
			"description": "Human-readable reason for why confirmation is needed"
		},
		"need_more_info": {
			"type": "boolean",
			"description": "Whether more information is needed from user to complete the action"
		},
		"follow_up_prompt": {
			"type": "string",
			"description": "The follow-up question to ask user when need_more_info is true"
		},
		"ai_message": {
			"type": "string",
			"description": "Friendly message to show user (for asking questions, confirming actions, or casual chat)"
		}
	},
	"required": ["action", "confidence", "needs_confirmation", "need_more_info"],
	"additionalProperties": false
}`)

func (c *Client) ParseIntent(ctx context.Context, userMessage string) (*Intent, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: getSystemPrompt(),
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userMessage,
			},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   "intent",
				Schema: intentSchema,
				Strict: true,
			},
		},
		Temperature: 0.1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call AI API: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	content := resp.Choices[0].Message.Content
	intent := &Intent{RawResponse: content}

	if err := json.Unmarshal([]byte(content), intent); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return intent, nil
}

func (c *Client) GenerateResponse(ctx context.Context, systemMsg, userMsg string) (string, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemMsg,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userMsg,
			},
		},
		Temperature: 0.7,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from AI")
	}

	return resp.Choices[0].Message.Content, nil
}

// ParseIntentWithHistory parses intent using conversation history for multi-turn conversations
func (c *Client) ParseIntentWithHistory(ctx context.Context, history []Message) (*Intent, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: getSystemPrompt(),
		},
	}

	for _, msg := range history {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: messages,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   "intent",
				Schema: intentSchema,
				Strict: true,
			},
		},
		Temperature: 0.1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call AI API: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	content := resp.Choices[0].Message.Content
	intent := &Intent{RawResponse: content}

	if err := json.Unmarshal([]byte(content), intent); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return intent, nil
}

// ContinueWithToolResult continues conversation after tool execution
func (c *Client) ContinueWithToolResult(ctx context.Context, history []Message, toolResult string) (*Intent, error) {
	// Add tool result as assistant context
	history = append(history, Message{
		Role:    "assistant",
		Content: fmt.Sprintf("[工具執行結果]\n%s", toolResult),
	})

	return c.ParseIntentWithHistory(ctx, history)
}
