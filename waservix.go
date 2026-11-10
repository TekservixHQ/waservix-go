// Package waservix is a Go client for the WAservix WhatsApp API Gateway.
//
// WAservix links your own WhatsApp number by QR code (the same way WhatsApp
// Web does) and gives you a REST API to send text, images, video, audio,
// documents, locations, polls and more, with webhooks for incoming messages.
//
// Docs: https://tekservix.com/waservix/docs/
package waservix

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version of this client.
const Version = "1.0.1"

// DefaultBaseURL is the WAservix API endpoint.
const DefaultBaseURL = "https://tekservix.com/waservix/api"

// Error is returned when the API answers with an error. A 503 means the device is reconnecting.
type Error struct {
	Status     int
	Message    string
	Errors     map[string][]string
	Body       []byte
	RetryAfter time.Duration // set on HTTP 429
}

func (e *Error) Error() string { return fmt.Sprintf("waservix: %s (HTTP %d)", e.Message, e.Status) }

// Client talks to the WAservix API. Create one with New and reuse it.
type Client struct {
	APIKey     string // device API key (wpk_...) or a personal access token for sessions
	BaseURL    string
	HTTPClient *http.Client
}

// New returns a client for the given key (Devices -> API key in the WAservix dashboard).
func New(apiKey string) *Client {
	return &Client{APIKey: apiKey, BaseURL: DefaultBaseURL, HTTPClient: &http.Client{Timeout: 30 * time.Second}}
}

// Request calls any documented endpoint and decodes the `data` field into out (which may be nil).
func (c *Client) Request(ctx context.Context, method, path string, body, out interface{}) error {
	if c.APIKey == "" {
		return errors.New("waservix: APIKey is required (Devices -> API key in the WAservix dashboard)")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "waservix-go/"+Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var env struct {
		Success *bool               `json:"success"`
		Message string              `json:"message"`
		Errors  map[string][]string `json:"errors"`
		Data    json.RawMessage     `json:"data"`
	}
	jsonErr := json.Unmarshal(raw, &env)
	if res.StatusCode >= 400 || (jsonErr == nil && env.Success != nil && !*env.Success) {
		e := &Error{Status: res.StatusCode, Message: env.Message, Errors: env.Errors, Body: raw}
		if e.Message == "" {
			e.Message = http.StatusText(res.StatusCode)
		}
		if s, perr := strconv.ParseFloat(res.Header.Get("Retry-After"), 64); perr == nil {
			e.RetryAfter = time.Duration(s * float64(time.Second))
		}
		return e
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if jsonErr == nil && env.Data != nil {
		return json.Unmarshal(env.Data, out)
	}
	return json.Unmarshal(raw, out)
}

// Message is the body of /send-message. Set To and the fields you need.
type Message struct {
	To          string                 `json:"to"` // international number or group JID (...@g.us)
	Text        string                 `json:"text,omitempty"`
	ImageURL    string                 `json:"imageUrl,omitempty"`
	VideoURL    string                 `json:"videoUrl,omitempty"`
	AudioURL    string                 `json:"audioUrl,omitempty"`
	DocumentURL string                 `json:"documentUrl,omitempty"`
	FileName    string                 `json:"fileName,omitempty"`
	StickerURL  string                 `json:"stickerUrl,omitempty"`
	Contact     map[string]interface{} `json:"contact,omitempty"`
	Location    map[string]interface{} `json:"location,omitempty"`
	Poll        map[string]interface{} `json:"poll,omitempty"`
	ReplyTo     string                 `json:"replyTo,omitempty"`
	Mentions    []string               `json:"mentions,omitempty"`
	ViewOnce    bool                   `json:"viewOnce,omitempty"`
}

// SendResult is returned when a message is queued.
type SendResult struct {
	MsgID  int64  `json:"msgId"`
	JID    string `json:"jid"`
	Status string `json:"status"` // "in_progress"
}

// SendMessage queues any message type.
func (c *Client) SendMessage(ctx context.Context, m Message) (*SendResult, error) {
	if m.To == "" {
		return nil, errors.New("waservix: To is required (international number or group JID)")
	}
	var out SendResult
	if err := c.Request(ctx, http.MethodPost, "/send-message", m, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SendText sends a plain text message.
func (c *Client) SendText(ctx context.Context, to, text string) (*SendResult, error) {
	return c.SendMessage(ctx, Message{To: to, Text: text})
}

// SendImage sends an image with an optional caption.
func (c *Client) SendImage(ctx context.Context, to, imageURL, caption string) (*SendResult, error) {
	return c.SendMessage(ctx, Message{To: to, ImageURL: imageURL, Text: caption})
}

// SendDocument sends a document (PDF, etc.) with optional file name and caption.
func (c *Client) SendDocument(ctx context.Context, to, documentURL, fileName, caption string) (*SendResult, error) {
	return c.SendMessage(ctx, Message{To: to, DocumentURL: documentURL, FileName: fileName, Text: caption})
}

// GetMessageInfo returns message status: queued, sent, delivered, read, played or failed.
func (c *Client) GetMessageInfo(ctx context.Context, msgID int64) (map[string]interface{}, error) {
	return c.getMap(ctx, "/messages/"+strconv.FormatInt(msgID, 10)+"/info")
}

// EditMessage edits one of your own text messages (about 15 minutes after sending).
func (c *Client) EditMessage(ctx context.Context, msgID int64, text string) error {
	return c.Request(ctx, http.MethodPut, "/messages/"+strconv.FormatInt(msgID, 10), map[string]string{"text": text}, nil)
}

// DeleteMessage deletes a message.
func (c *Client) DeleteMessage(ctx context.Context, msgID int64) error {
	return c.Request(ctx, http.MethodDelete, "/messages/"+strconv.FormatInt(msgID, 10), nil, nil)
}

// ResendMessage retries a failed message.
func (c *Client) ResendMessage(ctx context.Context, msgID int64) error {
	return c.Request(ctx, http.MethodPost, "/messages/"+strconv.FormatInt(msgID, 10)+"/resend", nil, nil)
}

// OnWhatsApp reports whether a number has WhatsApp.
func (c *Client) OnWhatsApp(ctx context.Context, phone string) (exists bool, jid string, err error) {
	var out struct {
		Exists bool   `json:"exists"`
		JID    string `json:"jid"`
	}
	err = c.Request(ctx, http.MethodGet, "/on-whatsapp/"+url.PathEscape(phone), nil, &out)
	return out.Exists, out.JID, err
}

// GetStatus returns connected, connecting, need_scan, disconnected, logged_out or expired.
func (c *Client) GetStatus(ctx context.Context) (string, error) {
	var out struct {
		Status string `json:"status"`
	}
	err := c.Request(ctx, http.MethodGet, "/status", nil, &out)
	return out.Status, err
}

// GetUser returns the linked WhatsApp account.
func (c *Client) GetUser(ctx context.Context) (map[string]interface{}, error) {
	return c.getMap(ctx, "/user")
}

// ListContacts lists contacts.
func (c *Client) ListContacts(ctx context.Context) ([]map[string]interface{}, error) {
	return c.getList(ctx, "/contacts")
}

// GetContact returns one contact.
func (c *Client) GetContact(ctx context.Context, phone string) (map[string]interface{}, error) {
	return c.getMap(ctx, "/contacts/"+url.PathEscape(phone))
}

// SaveContact saves a contact name.
func (c *Client) SaveContact(ctx context.Context, phone, name string) error {
	return c.Request(ctx, http.MethodPut, "/contacts", map[string]string{"phone": phone, "name": name}, nil)
}

// ListGroups lists the groups the number is in.
func (c *Client) ListGroups(ctx context.Context) ([]map[string]interface{}, error) {
	return c.getList(ctx, "/groups")
}

// GetGroupMetadata returns a group's details.
func (c *Client) GetGroupMetadata(ctx context.Context, groupJID string) (map[string]interface{}, error) {
	return c.getMap(ctx, "/groups/"+url.PathEscape(groupJID)+"/metadata")
}

// ListSessions lists WhatsApp sessions (needs a personal access token).
func (c *Client) ListSessions(ctx context.Context) ([]map[string]interface{}, error) {
	return c.getList(ctx, "/whatsapp-sessions")
}

// GetSession returns one session.
func (c *Client) GetSession(ctx context.Context, id int64) (map[string]interface{}, error) {
	return c.getMap(ctx, "/whatsapp-sessions/"+strconv.FormatInt(id, 10))
}

// CreateSession creates a session.
func (c *Client) CreateSession(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	var out map[string]interface{}
	err := c.Request(ctx, http.MethodPost, "/whatsapp-sessions", params, &out)
	return out, err
}

// ConnectSession starts linking: method "qr", or "code" with a phone number for pairing codes.
func (c *Client) ConnectSession(ctx context.Context, id int64, method, phone string) (map[string]interface{}, error) {
	if method == "" {
		method = "qr"
	}
	body := map[string]string{"method": method}
	if phone != "" {
		body["phone"] = phone
	}
	var out map[string]interface{}
	err := c.Request(ctx, http.MethodPost, "/whatsapp-sessions/"+strconv.FormatInt(id, 10)+"/connect", body, &out)
	return out, err
}

// GetSessionQRCode returns the QR code to scan.
func (c *Client) GetSessionQRCode(ctx context.Context, id int64) (map[string]interface{}, error) {
	return c.getMap(ctx, "/whatsapp-sessions/"+strconv.FormatInt(id, 10)+"/qrcode")
}

func (c *Client) getMap(ctx context.Context, path string) (map[string]interface{}, error) {
	var out map[string]interface{}
	err := c.Request(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) getList(ctx context.Context, path string) ([]map[string]interface{}, error) {
	var out []map[string]interface{}
	err := c.Request(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// VerifyWebhook compares the X-Webhook-Signature header with your device's webhook secret in constant time.
func VerifyWebhook(signature, secret string) bool {
	if signature == "" || secret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(signature), []byte(secret)) == 1
}
