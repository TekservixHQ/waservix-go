package waservix

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type call struct {
	method, path, auth string
	body               map[string]interface{}
}

func server(t *testing.T, status int, reply string, last *call) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*last = call{method: r.Method, path: r.URL.EscapedPath(), auth: r.Header.Get("Authorization")}
		if len(b) > 0 {
			_ = json.Unmarshal(b, &last.body)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	c := New("wpk_test")
	c.BaseURL = srv.URL
	return c
}

func TestSendText(t *testing.T) {
	var last call
	c := server(t, 200, `{"success":true,"data":{"msgId":58213,"jid":"1555@s.whatsapp.net","status":"in_progress"}}`, &last)
	res, err := c.SendText(context.Background(), "+1555", "hi")
	if err != nil || res.MsgID != 58213 || res.Status != "in_progress" {
		t.Fatalf("got %+v, %v", res, err)
	}
	if last.method != "POST" || last.path != "/send-message" || last.auth != "Bearer wpk_test" {
		t.Fatalf("bad call %+v", last)
	}
	if len(last.body) != 2 || last.body["to"] != "+1555" || last.body["text"] != "hi" {
		t.Fatalf("bad body %+v", last.body)
	}
}

func TestMediaAndPaths(t *testing.T) {
	var last call
	c := server(t, 200, `{"success":true,"data":{}}`, &last)
	ctx := context.Background()
	_, _ = c.SendDocument(ctx, "+1", "https://x/a.pdf", "a.pdf", "c")
	if last.body["documentUrl"] != "https://x/a.pdf" || last.body["fileName"] != "a.pdf" || last.body["text"] != "c" {
		t.Fatalf("document body %+v", last.body)
	}
	_, _ = c.GetMessageInfo(ctx, 5)
	if last.path != "/messages/5/info" {
		t.Fatalf("info %s", last.path)
	}
	_ = c.EditMessage(ctx, 5, "t")
	if last.method != "PUT" || last.body["text"] != "t" {
		t.Fatalf("edit %+v", last)
	}
	_, _, _ = c.OnWhatsApp(ctx, "+1555")
	if last.path != "/on-whatsapp/+1555" {
		t.Fatalf("on-whatsapp %s", last.path)
	}
	_, _ = c.GetGroupMetadata(ctx, "1203@g.us")
	if last.path != "/groups/1203@g.us/metadata" {
		t.Fatalf("group %s", last.path)
	}
	_, _ = c.ConnectSession(ctx, 3, "", "")
	if last.body["method"] != "qr" {
		t.Fatalf("connect %+v", last.body)
	}
}

func TestErrors(t *testing.T) {
	var last call
	c := server(t, 503, `{"success":false,"message":"Device reconnecting"}`, &last)
	_, err := c.GetStatus(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Status != 503 || e.Message != "Device reconnecting" {
		t.Fatalf("got %v", err)
	}
	if _, err := c.SendMessage(context.Background(), Message{Text: "x"}); err == nil {
		t.Fatal("expected error for empty To")
	}
}

func TestVerifyWebhook(t *testing.T) {
	if !VerifyWebhook("abc", "abc") || VerifyWebhook("abd", "abc") || VerifyWebhook("", "abc") {
		t.Fatal("webhook verification wrong")
	}
}
