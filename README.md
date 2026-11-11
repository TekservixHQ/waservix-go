# WAservix Go client

[![Go Reference](https://pkg.go.dev/badge/github.com/tekservixhq/waservix-go.svg)](https://pkg.go.dev/github.com/tekservixhq/waservix-go)

**WAservix WhatsApp API Gateway** - official Go client: send WhatsApp messages from your own number. By Tekservix.

Go client for the [WAservix WhatsApp API Gateway](https://tekservix.com/waservix/).

WAservix links your own WhatsApp number by QR code (the same way WhatsApp Web does) and gives you a REST API to send text, images, video, audio, documents, locations, polls and more, with webhooks for incoming messages and status updates.

- Standard library only, Go 1.21+
- `context.Context` on every call, typed message struct, typed errors
- Webhook check built in

## Install

```bash
go get github.com/tekservixhq/waservix-go
```

## Quick start

Copy your device's API key from the WAservix dashboard (Devices -> API key) and keep it on your server.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	waservix "github.com/tekservixhq/waservix-go"
)

func main() {
	wa := waservix.New(os.Getenv("WASERVIX_KEY"))

	res, err := wa.SendText(context.Background(), "+15551234567", "Hello Aline, your order is ready")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.MsgID, res.Status) // 58213 in_progress
}
```

## Sending

Messages are queued and sent one at a time per number, so each call returns right away with `Status: "in_progress"` and a `MsgID`.

```go
ctx := context.Background()

// Image with caption
wa.SendImage(ctx, "+15551234567", "https://example.com/product.jpg", "Back in stock")

// Document
wa.SendDocument(ctx, "+15551234567", "https://example.com/inv-1042.pdf", "Invoice-1042.pdf", "Your invoice")

// Anything the API supports, including groups (JID ending in @g.us)
wa.SendMessage(ctx, waservix.Message{To: "120363040000000000@g.us", Text: "Team meeting at 3pm"})
wa.SendMessage(ctx, waservix.Message{
	To:       "+15551234567",
	Location: map[string]any{"latitude": -1.9441, "longitude": 30.0619, "name": "Our shop"},
})
```

## Status, contacts and groups

```go
info, _ := wa.GetMessageInfo(ctx, 58213)            // queued, sent, delivered, read, played or failed
exists, jid, _ := wa.OnWhatsApp(ctx, "+15551234567")
status, _ := wa.GetStatus(ctx)                      // "connected"

wa.EditMessage(ctx, 58213, "Corrected text")        // own text messages, about 15 minutes
groups, _ := wa.ListGroups(ctx)
```

Anything else in the [API reference](https://tekservix.com/waservix/docs/) is one call away:

```go
err := wa.Request(ctx, "POST", "/messages/read", map[string]string{"msgId": "3EB0X123456789"}, nil)
```

## Webhooks

Set a webhook URL and events for the number in the dashboard. Each request carries `X-Webhook-Signature`, which equals the device's webhook secret:

```go
http.HandleFunc("/wa/webhook", func(w http.ResponseWriter, r *http.Request) {
	if !waservix.VerifyWebhook(r.Header.Get("X-Webhook-Signature"), os.Getenv("WASERVIX_WEBHOOK_SECRET")) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	var payload map[string]any
	json.NewDecoder(r.Body).Decode(&payload)
	w.WriteHeader(http.StatusOK) // acknowledge fast; queue slow work
	if payload["event"] == "messages.received" {
		log.Println(payload["data"])
	}
})
```

## Errors

API errors are `*waservix.Error` with `Status`, `Message`, field `Errors` and, on 429, `RetryAfter`. A 503 means the device is reconnecting: retry shortly.

Never ship the key inside a mobile or desktop app: anyone could read it. Call the API from your server.

## Migrating from WaSender

WAservix accepts the same `/send-message` body and webhook fields as WaSender, so most integrations switch by changing the base URL and key. See the [compatibility notes](https://tekservix.com/waservix/docs/).

## Use it responsibly

WAservix is not Meta's official WhatsApp Business Platform. There's no business verification and no per-conversation fee, but WhatsApp's spam rules apply to your number directly. Message people who expect to hear from you, warm up new numbers slowly and keep the built-in account protection on.

## More from Tekservix

- [SMS Gora SMS Gateway](https://tekservix.com/smsgora/): send SMS through your own Android phones (`go get github.com/tekservixhq/smsgora-go`)
- [AI Sales Agent](https://tekservix.com/ai-sales-agent): answers customers on WhatsApp and SMS using your website's content
- [Website to App](https://tekservix.com/mobile-apps): turn your website into Android and iOS apps
- WAservix is also available for Node.js (`npm install waservix`), Python (`pip install waservix`) and PHP (`composer require tekservix/waservix`)

## License

MIT
