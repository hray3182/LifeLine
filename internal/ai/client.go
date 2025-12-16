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

// ActionItem represents a single action in multi-action requests
type ActionItem struct {
	Action     string            `json:"action"`
	Entity     string            `json:"entity"`
	Parameters map[string]string `json:"parameters"`
}

// ConfirmationOption represents a choice for confirmation
type ConfirmationOption struct {
	Label      string            `json:"label"`      // Button text (e.g., "12/17", "12/18")
	Parameters map[string]string `json:"parameters"` // Parameters to override when this option is chosen
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
	// Multi-action support
	Actions []ActionItem `json:"actions,omitempty"`
	// Confirmation options (for ambiguous cases like date confirmation)
	ConfirmationOptions []ConfirmationOption `json:"confirmation_options,omitempty"`
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
- content: 內容 (memo, reminder)
- title: 標題 (todo, event)
- description: 描述
- priority: 優先級 (1-5)
- due_time: 截止時間 (格式: YYYY-MM-DD HH:MM)
- dtstart: 第一次發生時間 (格式: YYYY-MM-DD HH:MM)，用於 reminder 和 event
- rrule: RFC 5545 重複規則 (用於 reminder 和 event 的重複設定)
- amount: 金額
- category: 分類
- tags: 標籤

重要規則：
1. 時間處理：
   - 當用戶使用相對時間（如「明天」、「下週一」、「3 小時後」），請根據當前時間計算出具體的日期時間
   - 輸出格式: YYYY-MM-DD HH:MM
   - 重要：「明天」= 當前日期 + 1 天，「今天」= 當前日期
   - 深夜特別規則 (00:00-05:59)：如果當前時間在凌晨，用戶說「明天晚上」很可能是指「今晚」（同一個日曆日），此時必須設定 needs_confirmation = true 並詢問確認具體日期

2. RFC 5545 RRULE 重複規則（用於 create_reminder 和 create_event）：
   - 格式: FREQ=頻率;其他參數
   - 頻率 (FREQ): HOURLY, DAILY, WEEKLY, MONTHLY, YEARLY
   - 間隔 (INTERVAL): 數字，如 INTERVAL=2 表示每 2 個週期
   - 指定時間 (BYHOUR): 指定在哪些小時執行，如 BYHOUR=9,10,11,12
   - 指定分鐘 (BYMINUTE): 指定在哪些分鐘執行
   - 指定星期 (BYDAY): MO,TU,WE,TH,FR,SA,SU
   - 指定日期 (BYMONTHDAY): 1-31
   - 次數限制 (COUNT): 總共執行幾次
   - 結束日期 (UNTIL): 格式 YYYYMMDDTHHMMSSZ

   範例：
   - 每天早上 9 點: dtstart="2024-01-01 09:00", rrule="FREQ=DAILY"
   - 每小時（9點到22點）: dtstart="2024-01-01 09:00", rrule="FREQ=DAILY;BYHOUR=9,10,11,12,13,14,15,16,17,18,19,20,21,22"
   - 每週一三五: dtstart="2024-01-01 09:00", rrule="FREQ=WEEKLY;BYDAY=MO,WE,FR"
   - 每月 15 號: dtstart="2024-01-15 09:00", rrule="FREQ=MONTHLY;BYMONTHDAY=15"
   - 每 2 小時: dtstart="2024-01-01 09:00", rrule="FREQ=HOURLY;INTERVAL=2"
   - 一次性（不重複）: 只設定 dtstart，不設定 rrule

   注意：
   - 對於「每天從 X 點到 Y 點每小時」這類請求，使用 FREQ=DAILY;BYHOUR=X,X+1,...,Y
   - 不要使用 end_time 來表示每天的結束時間，end_time 只用於事件的持續時間
   - dtstart 是第一次發生的時間，也決定了每次發生的分鐘數

3. 以下情況必須設定 needs_confirmation = true：
   - 刪除操作 (delete_*)：任何刪除都需要確認
   - 更新操作 (update_*)：任何更新都需要確認
   - 深夜時間模糊：當前時間在 00:00-05:59 之間，且用戶提到「明天」、「今晚」、「晚上」等詞時，必須確認具體日期
   - 金額較大：支出或收入超過 10000 時需要確認

4. 確認選項 (confirmation_options)：
   - 當需要用戶從多個選項中選擇時，使用 confirmation_options 提供按鈕選項
   - 每個選項包含 label（按鈕文字）和 parameters（選擇後使用的參數）
   - 範例：深夜時間模糊時
     {
       "ai_message": "現在是凌晨，「明天下午4點」是指哪一天？",
       "needs_confirmation": true,
       "confirmation_options": [
         {"label": "12/17 (今天)", "parameters": {"dtstart": "2025-12-17 16:00"}},
         {"label": "12/18 (明天)", "parameters": {"dtstart": "2025-12-18 16:00"}}
       ]
     }
   - 刪除/更新操作的簡單確認不需要 confirmation_options，只需 ai_message 即可

5. 多輪對話規則：
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

6. 當收到工具執行結果時，你需要：
   - 解讀結果並組織成友善的回覆
   - 如果結果需要用戶選擇（如搜尋到多筆記錄），引導用戶選擇
   - 如果操作失敗，解釋原因並建議下一步

7. 多操作規則：
   - 當用戶請求需要多個操作時（如「把待辦改成事件」、「刪除這個然後建立那個」），使用 actions 陣列
   - actions 中的操作會依序執行，每個操作的結果會回傳給你
   - 使用 actions 時，action 欄位應設為 "multi_action"
   - 範例：用戶說「把待辦 #5 改成明天下午3點的事件」
     {
       "action": "multi_action",
       "actions": [
         {"action": "delete_todo", "entity": "todo", "parameters": {"id": "5"}},
         {"action": "create_event", "entity": "event", "parameters": {"title": "原待辦標題", "start_time": "2024-01-01 15:00"}}
       ],
       "confidence": 0.9,
       "needs_confirmation": true,
       "confirmation_reason": "這將刪除待辦事項 #5 並創建新事件"
     }`

func getSystemPrompt() string {
	now := time.Now()
	zone, offset := now.Zone()
	offsetHours := offset / 3600
	timeStr := fmt.Sprintf("%s (星期%s) [時區: %s, UTC%+d]",
		now.Format("2006-01-02 15:04"),
		[]string{"日", "一", "二", "三", "四", "五", "六"}[now.Weekday()],
		zone, offsetHours)
	return fmt.Sprintf(systemPromptTemplate, timeStr)
}

// JSON Schema for structured output
var intentSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"action": {
			"type": "string",
			"enum": ["create_memo", "list_memo", "delete_memo", "create_todo", "list_todo", "complete_todo", "delete_todo", "update_todo", "create_reminder", "list_reminder", "delete_reminder", "create_expense", "create_income", "list_transaction", "delete_transaction", "get_balance", "create_event", "list_event", "delete_event", "update_event", "multi_action", "unknown"],
			"description": "The action to perform. Use multi_action when multiple operations are needed."
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
		},
		"confirmation_options": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"label": {
						"type": "string",
						"description": "Button text to display (e.g., '12/17', '12/18')"
					},
					"parameters": {
						"type": "object",
						"additionalProperties": {
							"type": "string"
						},
						"description": "Parameters to use when this option is selected"
					}
				},
				"required": ["label", "parameters"],
				"additionalProperties": false
			},
			"description": "Options for user to choose from when confirmation is needed (e.g., date options). Always include a cancel option."
		},
		"actions": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"action": {
						"type": "string",
						"description": "The action to perform"
					},
					"entity": {
						"type": "string",
						"description": "The entity type"
					},
					"parameters": {
						"type": "object",
						"additionalProperties": {
							"type": "string"
						},
						"description": "Parameters for this action"
					}
				},
				"required": ["action"],
				"additionalProperties": false
			},
			"description": "Array of actions to execute sequentially when action is multi_action"
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
